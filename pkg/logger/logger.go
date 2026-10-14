package logger

import (
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New crea el logger del proceso con salida estándar. JSON es el formato
// recomendado para evidencias y operaciones; console se reserva para el
// desarrollo local.
func New(level, format string) (*zap.Logger, error) {
	return NewWithWriter(level, format, os.Stdout)
}

// NewWithWriter crea un logger con el destino indicado. Permite comprobar la
// salida en pruebas sin redirigir la salida global del proceso.
func NewWithWriter(level, format string, output io.Writer) (*zap.Logger, error) {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	encoder, err := newEncoder(format)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(output), parsedLevel)
	return zap.New(core), nil
}

// NewNop devuelve un logger sin salida para pruebas o rutas que no necesitan
// observabilidad todavía.
func NewNop() *zap.Logger {
	return zap.NewNop()
}

func parseLevel(value string) (zapcore.Level, error) {
	switch value {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL: %q no es valido (debug, info, warn o error)", value)
	}
}

func newEncoder(format string) (zapcore.Encoder, error) {
	switch format {
	case "json":
		config := zap.NewProductionEncoderConfig()
		config.EncodeTime = zapcore.RFC3339TimeEncoder
		return zapcore.NewJSONEncoder(config), nil
	case "console":
		config := zap.NewDevelopmentEncoderConfig()
		config.EncodeTime = zapcore.RFC3339TimeEncoder
		return zapcore.NewConsoleEncoder(config), nil
	default:
		return nil, fmt.Errorf("LOG_FORMAT: %q no es valido (json o console)", format)
	}
}
