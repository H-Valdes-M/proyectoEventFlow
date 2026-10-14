package config

import (
	"fmt"
	"os"
	"time"

	"eventflow/pkg/env"
)

// APIConfig reune la configuracion del proceso API de este hito: entorno,
// puerto, PostgreSQL, logs y tiempos. Deliberadamente no incluye AMQP_URL ni
// PUBSUB_PROJECT_ID: la API solo necesita PostgreSQL y en este proyecto no
// existe GCP (el broker es responsabilidad del worker). DBPassword se
// consulta por configuracion pero nunca se imprime ni se registra.
type APIConfig struct {
	Environment     string
	Port            int
	DBHost          string
	DBPort          int
	DBUser          string
	DBPassword      string
	DBName          string
	DBSSLMode       string
	LogLevel        string
	LogFormat       string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

const (
	envEnvironment      = "ENVIRONMENT"
	envPort             = "PORT"
	envDatabaseHost     = "DB_HOST"
	envDatabasePort     = "DB_PORT"
	envDatabaseUser     = "DB_USER"
	envDatabasePassword = "DB_PASSWORD"
	envDatabaseName     = "DB_NAME"
	envDatabaseSSLMode  = "DB_SSL_MODE"
	envLogLevel         = "LOG_LEVEL"
	envLogFormat        = "LOG_FORMAT"
	envRequestTimeout   = "REQUEST_TIMEOUT"
	envShutdownTimeout  = "SHUTDOWN_TIMEOUT"
)

var validEnvironments = map[string]bool{"dev": true, "qa": true, "prod": true}
var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
var validLogFormats = map[string]bool{"json": true, "console": true}
var validSSLModes = map[string]bool{"disable": true, "require": true, "verify-ca": true, "verify-full": true}

// LoadAPIConfig carga y valida la configuracion de la API con la precedencia
// de la base: sistema > archivo opcional > defaults. El archivo solo se
// busca en dev/qa; en prod la configuracion llega solo por entorno. Una
// configuracion invalida impide arrancar y no abre nada: esta funcion no
// crea conexiones ni servidores.
func LoadAPIConfig() (*APIConfig, error) {
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

	port, err := readPort(envPort, 8080, file)
	if err != nil {
		return nil, err
	}
	dbPort, err := readPort(envDatabasePort, 5432, file)
	if err != nil {
		return nil, err
	}
	requestTimeout, err := readDuration(envRequestTimeout, 10*time.Second, file)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := readDuration(envShutdownTimeout, 15*time.Second, file)
	if err != nil {
		return nil, err
	}

	cfg := &APIConfig{
		Environment:     environment,
		Port:            port,
		DBHost:          readString(envDatabaseHost, "localhost", file),
		DBPort:          dbPort,
		DBUser:          readString(envDatabaseUser, "eventflow", file),
		DBPassword:      readString(envDatabasePassword, "eventflow", file),
		DBName:          readString(envDatabaseName, "eventflow", file),
		DBSSLMode:       readString(envDatabaseSSLMode, "disable", file),
		LogLevel:        readString(envLogLevel, "info", file),
		LogFormat:       readString(envLogFormat, "json", file),
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate comprueba con listas cerradas los valores de texto de la
// configuracion antes de considerarla lista para el arranque.
func (c *APIConfig) validate() error {
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%s: %q no es un nivel valido (debug, info, warn o error)", envLogLevel, c.LogLevel)
	}
	if !validLogFormats[c.LogFormat] {
		return fmt.Errorf("%s: %q no es un formato valido (json o console)", envLogFormat, c.LogFormat)
	}
	if !validSSLModes[c.DBSSLMode] {
		return fmt.Errorf("%s: %q no es un modo admitido por PostgreSQL (disable, require, verify-ca o verify-full)", envDatabaseSSLMode, c.DBSSLMode)
	}
	return nil
}
