// Package pubsub contiene el transporte AMQP independiente del negocio.
package pubsub

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	Exchange   = "eventflow.events"
	Queue      = "eventflow.orders.process"
	RoutingKey = "order.created"
)

var (
	ErrNotConfirmed = errors.New("publicacion no confirmada por RabbitMQ")
	ErrUnroutable   = errors.New("publicacion sin ruta RabbitMQ")
)

// Message conserva el identificador estable del evento que se publica.
type Message struct {
	MessageID string
	EventID   string
	Type      string
	Attempt   int
	Payload   []byte
}

// Delivery es una entrega manual cuyo canal propietario confirma su resultado.
type Delivery struct {
	MessageID string
	EventID   string
	Type      string
	Attempt   int
	Payload   []byte
}

// Disposition define la única disposición aplicable a una entrega manual.
type Disposition int

const (
	KeepUnconfirmed Disposition = iota
	Ack
	NackDrop
)

// DeliveryHandler decide la disposición solo después de completar la operación durable.
type DeliveryHandler func(context.Context, Delivery) (Disposition, error)

// Publisher es el contrato mínimo que necesita el relay.
type Publisher interface {
	PublishConfirmed(context.Context, Message) error
}

// RabbitMQProvider publica en la topología durable de EventFlow.
type RabbitMQProvider struct {
	connection *amqp091.Connection
	channel    *amqp091.Channel
	returns    <-chan amqp091.Return
	mu         sync.Mutex
}

var testBeforeAck func() error

// NewRabbitMQProvider conecta, declara la topología y habilita confirms antes
// de aceptar publicaciones.
func NewRabbitMQProvider(ctx context.Context, url string) (*RabbitMQProvider, error) {
	dialer := &net.Dialer{}
	connection, err := amqp091.DialConfig(url, amqp091.Config{
		Dial: func(network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	})
	if err != nil {
		return nil, errors.New("no se pudo conectar a RabbitMQ")
	}
	channel, err := connection.Channel()
	if err != nil {
		connection.Close()
		return nil, errors.New("no se pudo abrir el canal RabbitMQ")
	}
	if err := declareTopology(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, errors.New("no se pudo declarar la topologia RabbitMQ")
	}
	if err := channel.Confirm(false); err != nil {
		channel.Close()
		connection.Close()
		return nil, errors.New("no se pudo habilitar confirmaciones RabbitMQ")
	}
	return &RabbitMQProvider{
		connection: connection,
		channel:    channel,
		returns:    channel.NotifyReturn(make(chan amqp091.Return, 1)),
	}, nil
}

// PublishConfirmed retorna nil solo después del confirm positivo y de
// comprobar que mandatory no devolvió el mensaje por falta de ruta.
func (p *RabbitMQProvider) PublishConfirmed(ctx context.Context, message Message) error {
	if message.MessageID == "" || message.EventID == "" || message.Type == "" || message.Attempt < 1 {
		return errors.New("mensaje RabbitMQ invalido")
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, Exchange, message.Type, true, false, amqp091.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp091.Persistent,
		MessageId:    message.MessageID,
		Timestamp:    time.Now().UTC(),
		Type:         message.Type,
		Headers:      amqp091.Table{"attempt": int32(message.Attempt), "event_id": message.EventID},
		Body:         message.Payload,
	})
	if err != nil {
		return errors.New("no se pudo publicar en RabbitMQ")
	}
	acknowledged, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotConfirmed, err)
	}
	if !acknowledged {
		return ErrNotConfirmed
	}
	select {
	case returned := <-p.returns:
		if returned.MessageId == message.MessageID {
			return ErrUnroutable
		}
		return ErrUnroutable
	default:
		return nil
	}
}

// Consume detiene nuevas entregas al cancelar intakeCtx, pero permite que la
// entrega ya iniciada termine con processingCtx antes de cerrar su canal.
// Un resultado sin disposición durable cierra el canal para que RabbitMQ
// recupere la entrega.
func (p *RabbitMQProvider) Consume(intakeCtx, processingCtx context.Context, prefetch int, handler DeliveryHandler) error {
	if prefetch < 1 || handler == nil {
		return errors.New("consumidor RabbitMQ invalido")
	}
	if processingCtx == nil {
		return errors.New("contexto de procesamiento RabbitMQ invalido")
	}
	p.mu.Lock()
	if p.connection == nil {
		p.mu.Unlock()
		return errors.New("RabbitMQ cerrado")
	}
	channel, err := p.connection.Channel()
	p.mu.Unlock()
	if err != nil {
		return errors.New("no se pudo abrir el canal de consumo RabbitMQ")
	}
	defer channel.Close()
	if err := channel.Qos(prefetch, 0, false); err != nil {
		return errors.New("no se pudo configurar prefetch RabbitMQ")
	}
	deliveries, err := channel.Consume(Queue, "", false, false, false, false, nil)
	if err != nil {
		return errors.New("no se pudo iniciar consumo RabbitMQ")
	}
	for {
		select {
		case <-intakeCtx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("consumo RabbitMQ interrumpido")
			}
			attempt, _ := attemptFromHeaders(delivery.Headers)
			eventID, _ := delivery.Headers["event_id"].(string)
			disposition, handlerErr := handler(processingCtx, Delivery{
				MessageID: delivery.MessageId, EventID: eventID, Type: delivery.Type,
				Attempt: attempt, Payload: delivery.Body,
			})
			if handlerErr != nil || disposition == KeepUnconfirmed {
				return fmt.Errorf("entrega sin disposicion durable: %w", handlerErr)
			}
			if disposition == Ack {
				if testBeforeAck != nil {
					if err := testBeforeAck(); err != nil {
						return err
					}
				}
				if err := delivery.Ack(false); err != nil {
					return errors.New("no se pudo confirmar entrega RabbitMQ")
				}
				continue
			}
			if disposition == NackDrop {
				if err := delivery.Nack(false, false); err != nil {
					return errors.New("no se pudo rechazar entrega RabbitMQ")
				}
				continue
			}
			return errors.New("disposicion de entrega RabbitMQ invalida")
		}
	}
}

func attemptFromHeaders(headers amqp091.Table) (int, bool) {
	value, ok := headers["attempt"]
	if !ok {
		return 0, false
	}
	switch attempt := value.(type) {
	case int32:
		return int(attempt), attempt > 0
	case int64:
		return int(attempt), attempt > 0
	default:
		return 0, false
	}
}

// Close libera el canal y la conexión. Es seguro llamar más de una vez.
func (p *RabbitMQProvider) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.channel != nil {
		_ = p.channel.Close()
		p.channel = nil
	}
	if p.connection != nil {
		_ = p.connection.Close()
		p.connection = nil
	}
}

func declareTopology(channel *amqp091.Channel) error {
	if err := channel.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare(Queue, true, false, false, false, nil); err != nil {
		return err
	}
	return channel.QueueBind(Queue, RoutingKey, Exchange, false, nil)
}
