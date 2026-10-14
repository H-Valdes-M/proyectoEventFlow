package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"eventflow/pkg/env"
)

const (
	envAMQPURL               = "AMQP_URL"
	envOutboxPollInterval    = "OUTBOX_POLL_INTERVAL"
	envOutboxBatchSize       = "OUTBOX_BATCH_SIZE"
	envPublishConfirmTimeout = "PUBLISH_CONFIRM_TIMEOUT"
	envConsumerPrefetch      = "CONSUMER_PREFETCH"
)

// WorkerConfig contiene los parámetros AMQP y relay propios del worker.
type WorkerConfig struct {
	Environment           string
	AMQPURL               string
	OutboxPollInterval    time.Duration
	OutboxBatchSize       int
	PublishConfirmTimeout time.Duration
	ConsumerPrefetch      int
}

// LoadWorkerConfig carga una configuración separada para no exigir AMQP_URL
// al proceso API.
func LoadWorkerConfig() (*WorkerConfig, error) {
	environment := env.Normalize(os.Getenv(envEnvironment))
	if !validEnvironments[environment] {
		return nil, fmt.Errorf("%s: %q no es un entorno valido (dev, qa o prod)", envEnvironment, environment)
	}
	file := make(map[string]string)
	if environment != "prod" {
		var err error
		file, err = env.LoadDotEnv(".env." + environment)
		if err != nil {
			return nil, fmt.Errorf("cargar el archivo opcional de configuracion: %w", err)
		}
	}
	batchSize, err := readPositiveInt(envOutboxBatchSize, 20, file)
	if err != nil {
		return nil, err
	}
	pollInterval, err := readDuration(envOutboxPollInterval, 500*time.Millisecond, file)
	if err != nil {
		return nil, err
	}
	publishTimeout, err := readDuration(envPublishConfirmTimeout, 5*time.Second, file)
	if err != nil {
		return nil, err
	}
	prefetch, err := readPositiveInt(envConsumerPrefetch, 1, file)
	if err != nil {
		return nil, err
	}
	amqpURL := readString(envAMQPURL, "amqp://guest:guest@localhost:5672/", file)
	parsed, err := url.Parse(amqpURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") {
		return nil, fmt.Errorf("%s: URL AMQP invalida", envAMQPURL)
	}
	return &WorkerConfig{
		Environment: environment, AMQPURL: amqpURL, OutboxPollInterval: pollInterval, OutboxBatchSize: batchSize, PublishConfirmTimeout: publishTimeout, ConsumerPrefetch: prefetch,
	}, nil
}

func readPositiveInt(key string, defaultValue int, file map[string]string) (int, error) {
	raw := readString(key, strconv.Itoa(defaultValue), file)
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s: se espera un entero positivo", key)
	}
	return value, nil
}
