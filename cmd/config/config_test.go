package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAPIConfigConDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envEnvironment, "")
	for _, key := range []string{
		envPort, envDatabaseHost, envDatabasePort, envDatabaseUser,
		envDatabasePassword, envDatabaseName, envDatabaseSSLMode,
		envLogLevel, envLogFormat, envRequestTimeout, envShutdownTimeout,
	} {
		unsetEnv(t, key)
	}
	cfg, err := LoadAPIConfig()
	if err != nil {
		t.Fatalf("la carga con defaults no debio fallar: %v", err)
	}
	expected := APIConfig{
		Environment:     "dev",
		Port:            8080,
		DBHost:          "localhost",
		DBPort:          5432,
		DBUser:          "eventflow",
		DBPassword:      "eventflow",
		DBName:          "eventflow",
		DBSSLMode:       "disable",
		LogLevel:        "info",
		LogFormat:       "json",
		RequestTimeout:  10 * time.Second,
		ShutdownTimeout: 15 * time.Second,
	}
	if *cfg != expected {
		t.Fatalf("configuracion por defecto incorrecta:\n obtuve %+v\n esperaba %+v", *cfg, expected)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("limpiar %s para la prueba: %v", key, err)
	}
	t.Cleanup(func() {
		var err error
		if existed {
			err = os.Setenv(key, previous)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Errorf("restaurar %s después de la prueba: %v", key, err)
		}
	})
}

func TestLoadAPIConfigArchivoOpcionalAusente(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envEnvironment, "qa")
	if _, err := LoadAPIConfig(); err != nil {
		t.Fatalf("la ausencia de .env.qa no debe impedir la carga: %v", err)
	}
}

func TestLoadAPIConfigLeeArchivoYElSistemaPrecede(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(envEnvironment, "dev")
	content := "PORT=9090\nDB_NAME=otra_base\nREQUEST_TIMEOUT=30s\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.dev"), []byte(content), 0o600); err != nil {
		t.Fatalf("crear archivo de prueba: %v", err)
	}

	cfg, err := LoadAPIConfig()
	if err != nil {
		t.Fatalf("la carga con archivo no debio fallar: %v", err)
	}
	if cfg.Port != 9090 || cfg.DBName != "otra_base" || cfg.RequestTimeout != 30*time.Second {
		t.Fatalf("el archivo debe aportar los valores declarados: %+v", cfg)
	}

	t.Setenv(envPort, "7007")
	fromSystem, err := LoadAPIConfig()
	if err != nil {
		t.Fatalf("la carga con variable del sistema no debio fallar: %v", err)
	}
	if fromSystem.Port != 7007 {
		t.Fatalf("el sistema debe preceder al archivo: se esperaba 7007 y llego %d", fromSystem.Port)
	}
	if fromSystem.DBName != "otra_base" {
		t.Fatalf("el valor del archivo debe sobrevivir si no hay variable del sistema: %q", fromSystem.DBName)
	}
}

func TestLoadAPIConfigRechazaValoresInvalidos(t *testing.T) {
	cases := []struct {
		name        string
		environment string
		key         string
		value       string
		fileContent string
	}{
		{name: "entorno invalido", environment: "staging"},
		{name: "puerto fuera de rango", environment: "dev", key: envPort, value: "99999"},
		{name: "puerto no numerico", environment: "dev", key: envPort, value: "ochenta"},
		{name: "duracion invalida", environment: "dev", key: envRequestTimeout, value: "10"},
		{name: "duracion no positiva", environment: "dev", key: envShutdownTimeout, value: "-1s"},
		{name: "nivel de log invalido", environment: "dev", key: envLogLevel, value: "verbose"},
		{name: "formato de log invalido", environment: "dev", key: envLogFormat, value: "texto"},
		{name: "modo ssl invalido", environment: "dev", key: envDatabaseSSLMode, value: "rigido"},
		{name: "linea malformada en el archivo", environment: "dev", fileContent: "DB_NAME sin igual\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv(envEnvironment, c.environment)
			if c.key != "" {
				t.Setenv(c.key, c.value)
			}
			if c.fileContent != "" {
				if err := os.WriteFile(".env."+c.environment, []byte(c.fileContent), 0o600); err != nil {
					t.Fatalf("crear archivo de prueba: %v", err)
				}
			}
			if _, err := LoadAPIConfig(); err == nil {
				t.Fatal("una configuracion invalida debe impedir la carga")
			}
		})
	}
}

func TestLoadAPIConfigNoDependeDelBrokerNiDeGCP(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envEnvironment, "dev")
	t.Setenv("AMQP_URL", "amqp://usuario:contrasena@fantasma:5672/")
	t.Setenv("PUBSUB_PROJECT_ID", "proyecto-fantasma")
	cfg, err := LoadAPIConfig()
	if err != nil {
		t.Fatalf("la API debe cargar sin AMQP_URL ni PUBSUB_PROJECT_ID: %v", err)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("el default de apagado no debio alterarse: %s", cfg.ShutdownTimeout)
	}
}

func TestLoadAPIConfigEnProdNoLeeArchivo(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(envEnvironment, "prod")
	if err := os.WriteFile(".env.prod", []byte("PORT=99999\n"), 0o600); err != nil {
		t.Fatalf("crear archivo de prueba: %v", err)
	}
	cfg, err := LoadAPIConfig()
	if err != nil {
		t.Fatalf("la carga en prod no debio fallar: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("en prod no se debe leer archivo opcional: se esperaba 8080 y llego %d", cfg.Port)
	}
}
