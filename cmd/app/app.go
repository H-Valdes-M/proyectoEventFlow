package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"eventflow/cmd/config"
	"eventflow/cmd/container"
	"eventflow/cmd/routes"
	_ "eventflow/docs"
	"eventflow/internal/health/application"
	healthpostgres "eventflow/internal/health/infrastructure/postgres"
	"eventflow/pkg/database"
	"eventflow/pkg/logger"
	"eventflow/pkg/metrics"
	"github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

// Resources reúne los recursos globales ya creados para la API.
type Resources struct {
	Config    *config.APIConfig
	Logger    *zap.Logger
	Database  database.Database
	Server    *http.Server
	readiness *readinessGate
}

// Init resuelve configuración, crea logger y pool PostgreSQL, y deja un
// registro de arranque con campos seguros. El pool no comprueba conectividad
// hasta que se invoque Ping; tampoco inicia el servidor.
func Init() (*Resources, error) {
	return initWithWriter(os.Stdout)
}

func initWithWriter(output io.Writer) (*Resources, error) {
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		return nil, err
	}
	log, err := logger.NewWithWriter(cfg.LogLevel, cfg.LogFormat, output)
	if err != nil {
		return nil, err
	}
	log.Info("aplicacion inicializada",
		zap.String("environment", cfg.Environment),
		zap.Int("port", cfg.Port),
		zap.String("log_level", cfg.LogLevel),
		zap.String("log_format", cfg.LogFormat),
	)
	db, err := database.NewPostgresDatabase(context.Background(), database.PostgresConfig{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Database: cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		_ = log.Sync()
		return nil, err
	}
	orderHandler, err := container.NewOrderHandler(db)
	if err != nil {
		db.Close()
		_ = log.Sync()
		return nil, err
	}
	readinessChecker, err := healthpostgres.NewReadinessChecker(db)
	if err != nil {
		db.Close()
		_ = log.Sync()
		return nil, err
	}
	readiness := newReadinessGate(application.NewReadinessUseCase(readinessChecker))
	metricRegistry := metrics.NewRegistry()
	operationalMetrics, err := container.NewOperationalMetrics(db)
	if err != nil {
		db.Close()
		_ = log.Sync()
		return nil, err
	}
	router := routes.New(log,
		routes.WithOrderHandler(orderHandler),
		routes.WithReadiness(readiness),
		routes.WithMetrics(metricRegistry, metricRegistry.Handler(func(request *http.Request) (metrics.DurableStats, error) {
			return operationalMetrics.Read(request.Context())
		})),
	)
	if cfg.Environment == "dev" || cfg.Environment == "qa" {
		router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
	return &Resources{
		Config:    cfg,
		Logger:    log,
		Database:  db,
		Server:    newHTTPServer(cfg, router),
		readiness: readiness,
	}, nil
}

// Run sirve las rutas ya ensambladas y devuelve los errores de arranque para
// que el proceso no quede esperando señales si el puerto está ocupado.
func (r *Resources) Run() error {
	if r == nil || r.Server == nil {
		return errors.New("servidor HTTP no inicializado")
	}
	err := r.Server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown deja de aceptar tráfico, drena el servidor con el contexto recibido
// y recién entonces cierra las dependencias.
func (r *Resources) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.readiness != nil {
		r.readiness.markNotReady()
	}
	if r.Server != nil {
		if err := r.Server.Shutdown(ctx); err != nil {
			errs = append(errs, err)
			// Shutdown no cancela conexiones activas cuando vence el plazo.
			errs = append(errs, r.Server.Close())
		}
	}
	if r.Database != nil {
		r.Database.Close()
	}
	if r.Logger != nil {
		errs = append(errs, r.Logger.Sync())
	}
	return errors.Join(errs...)
}

func newHTTPServer(cfg *config.APIConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.RequestTimeout,
		WriteTimeout:      cfg.RequestTimeout,
		IdleTimeout:       60 * time.Second,
	}
}
