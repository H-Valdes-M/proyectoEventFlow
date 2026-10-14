package config

import (
	"testing"
	"time"
)

func TestLoadWorkerConfigDefaultsYValidacionAMQP(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envEnvironment, "dev")
	config, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("cargar configuración worker: %v", err)
	}
	if config.AMQPURL != "amqp://guest:guest@localhost:5672/" || config.OutboxPollInterval != 500*time.Millisecond || config.OutboxBatchSize != 20 || config.PublishConfirmTimeout != 5*time.Second || config.ConsumerPrefetch != 1 {
		t.Fatalf("defaults worker inesperados: %+v", config)
	}
	t.Setenv(envAMQPURL, "http://localhost:5672")
	if _, err := LoadWorkerConfig(); err == nil {
		t.Fatal("una URL AMQP inválida debe impedir el arranque del worker")
	}
}
