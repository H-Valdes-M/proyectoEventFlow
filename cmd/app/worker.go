package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"eventflow/cmd/config"
	"eventflow/cmd/container"
	"eventflow/pkg/database"
	"eventflow/pkg/logger"
	"eventflow/pkg/pubsub"
	"go.uber.org/zap"
)

// WorkerResources reúne los recursos globales del proceso de relay.
type WorkerResources struct {
	Config      *config.WorkerConfig
	APIConfig   *config.APIConfig
	Logger      *zap.Logger
	Database    database.Database
	publisher   *pubsub.RabbitMQProvider
	publisherMu sync.Mutex
	readyMu     sync.RWMutex
	ready       bool
	lifecycleMu sync.Mutex
	runDone     chan struct{}
	cancelWork  context.CancelFunc
	runWorker   func(context.Context, context.Context) error
}

// InitWorker crea los recursos del worker sin abrir una conexión AMQP. La
// conexión se intenta durante Run para poder recuperar un broker que arranca
// después del proceso.
func InitWorker() (*WorkerResources, error) {
	return initWorkerWithWriter(os.Stdout)
}

func initWorkerWithWriter(output io.Writer) (*WorkerResources, error) {
	workerConfig, err := config.LoadWorkerConfig()
	if err != nil {
		return nil, err
	}
	apiConfig, err := config.LoadAPIConfig()
	if err != nil {
		return nil, err
	}
	log, err := logger.NewWithWriter(apiConfig.LogLevel, apiConfig.LogFormat, output)
	if err != nil {
		return nil, err
	}
	db, err := database.NewPostgresDatabase(context.Background(), database.PostgresConfig{
		Host: apiConfig.DBHost, Port: apiConfig.DBPort, User: apiConfig.DBUser,
		Password: apiConfig.DBPassword, Database: apiConfig.DBName, SSLMode: apiConfig.DBSSLMode,
	})
	if err != nil {
		_ = log.Sync()
		return nil, err
	}
	log.Info("worker inicializado", zap.String("environment", workerConfig.Environment), zap.Int("outbox_batch_size", workerConfig.OutboxBatchSize))
	return &WorkerResources{Config: workerConfig, APIConfig: apiConfig, Logger: log, Database: db}, nil
}

// Ready comprueba las dependencias requeridas por el worker. La API no usa
// esta sonda ni requiere RabbitMQ para aceptar pedidos mediante outbox.
func (r *WorkerResources) Ready(ctx context.Context) error {
	if r == nil || r.Database == nil {
		return errors.New("worker no inicializado")
	}
	if err := r.Database.Ping(ctx); err != nil {
		return fmt.Errorf("PostgreSQL no disponible: %w", err)
	}
	r.readyMu.RLock()
	ready := r.ready
	r.readyMu.RUnlock()
	if !ready {
		return errors.New("RabbitMQ no disponible")
	}
	return nil
}

// Run alterna lotes de outbox y consumo serial. Un fallo de consumo cierra su
// conexión y deja las entregas sin disposición durable para su recuperación.
func (r *WorkerResources) Run(ctx context.Context) error {
	if r == nil || r.Config == nil || r.Database == nil {
		return errors.New("worker no inicializado")
	}
	workCtx, done, err := r.startRun()
	if err != nil {
		return err
	}
	defer close(done)
	defer r.finishRun()
	if r.runWorker != nil {
		return r.runWorker(ctx, workCtx)
	}
	return runRelayLoop(ctx, r.Config.OutboxPollInterval, func(ctx context.Context) error {
		if err := r.Database.Ping(ctx); err != nil {
			r.setReady(false)
			return fmt.Errorf("comprobar PostgreSQL: %w", err)
		}
		publisher, err := r.getPublisher(ctx)
		if err != nil {
			r.setReady(false)
			return err
		}
		relay, err := container.NewOutboxRelay(r.Database, publisher)
		if err != nil {
			r.setReady(false)
			return err
		}
		relay.WithLogger(r.Logger)
		publishCtx, cancel := context.WithTimeout(ctx, r.Config.PublishConfirmTimeout)
		if err := relay.PublishPendingBatch(publishCtx, r.Config.OutboxBatchSize); err != nil {
			cancel()
			r.closePublisher()
			r.setReady(false)
			return fmt.Errorf("publicar lote outbox: %w", err)
		}
		cancel()
		consumer, err := container.NewOrderConsumer(r.Database)
		if err != nil {
			r.setReady(false)
			return err
		}
		consumer.WithLogger(r.Logger)
		r.setReady(true)
		consumeCtx, cancelConsume := context.WithTimeout(ctx, r.Config.OutboxPollInterval)
		err = publisher.Consume(consumeCtx, workCtx, r.Config.ConsumerPrefetch, consumer.HandleDelivery)
		cancelConsume()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			r.closePublisher()
			r.setReady(false)
			return fmt.Errorf("consumir pedidos: %w", err)
		}
		return nil
	})
}

func (r *WorkerResources) startRun() (context.Context, chan struct{}, error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.runDone != nil {
		return nil, nil, errors.New("worker ya esta en ejecucion")
	}
	workCtx, cancelWork := context.WithCancel(context.Background())
	r.runDone = make(chan struct{})
	r.cancelWork = cancelWork
	return workCtx, r.runDone, nil
}

func (r *WorkerResources) finishRun() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.cancelWork != nil {
		r.cancelWork()
	}
}

func (r *WorkerResources) activeRun() (chan struct{}, context.CancelFunc) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	return r.runDone, r.cancelWork
}

func runRelayLoop(ctx context.Context, interval time.Duration, run func(context.Context) error) error {
	if interval <= 0 {
		return errors.New("intervalo de sondeo invalido")
	}
	for {
		if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			// El siguiente intervalo controla el reintento y evita un bucle agresivo.
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (r *WorkerResources) getPublisher(ctx context.Context) (*pubsub.RabbitMQProvider, error) {
	r.publisherMu.Lock()
	defer r.publisherMu.Unlock()
	if r.publisher != nil {
		return r.publisher, nil
	}
	publisher, err := pubsub.NewRabbitMQProvider(ctx, r.Config.AMQPURL)
	if err != nil {
		return nil, err
	}
	r.publisher = publisher
	return publisher, nil
}

func (r *WorkerResources) closePublisher() {
	r.publisherMu.Lock()
	defer r.publisherMu.Unlock()
	if r.publisher != nil {
		r.publisher.Close()
		r.publisher = nil
	}
}

func (r *WorkerResources) setReady(ready bool) {
	r.readyMu.Lock()
	r.ready = ready
	r.readyMu.Unlock()
}

// Shutdown marca el worker no listo, impide nuevas entregas y espera el
// drenaje. Si vence el presupuesto, cancela el procesamiento en curso antes
// de cerrar el canal para que RabbitMQ recupere cualquier entrega sin ACK.
func (r *WorkerResources) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.setReady(false)
	var shutdownErr error
	if done, cancelWork := r.activeRun(); done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			shutdownErr = ctx.Err()
			if cancelWork != nil {
				cancelWork()
			}
			// Consume cierra su canal al terminar el handler cancelado, con lo que
			// RabbitMQ recupera la entrega manual que no alcanzó ACK.
			<-done
		}
	}
	r.closePublisher()
	if r.Database != nil {
		r.Database.Close()
	}
	if r.Logger != nil {
		return errors.Join(shutdownErr, r.Logger.Sync())
	}
	return shutdownErr
}
