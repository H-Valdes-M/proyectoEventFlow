package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// lookup resuelve el valor efectivo de una variable: primero el entorno del
// proceso (mayor precedencia) y despues el archivo opcional cargado; si no
// aparece en ninguno, el getter aplica su default.
func lookup(key string, file map[string]string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}
	if value, ok := file[key]; ok {
		return value, true
	}
	return "", false
}

// readString devuelve el valor efectivo de la variable o su default.
func readString(key, defaultValue string, file map[string]string) string {
	if value, ok := lookup(key, file); ok {
		return value
	}
	return defaultValue
}

// readPort valida un puerto TCP del rango 1..65535.
func readPort(key string, defaultValue int, file map[string]string) (int, error) {
	raw := readString(key, strconv.Itoa(defaultValue), file)
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q no es un puerto valido (debe ser un entero)", key, raw)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s: %d esta fuera del rango admisible (1..65535)", key, port)
	}
	return port, nil
}

// readDuration valida una duracion positiva en el formato canonico de la
// base (por ejemplo 15s); no interpreta numeros sueltos como segundos.
func readDuration(key string, defaultValue time.Duration, file map[string]string) (time.Duration, error) {
	raw := readString(key, defaultValue.String(), file)
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q no es una duracion valida (se espera el formato de la base, por ejemplo %s)", key, raw, defaultValue)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s: la duracion debe ser positiva y llego %s", key, duration)
	}
	return duration, nil
}
