package env

import "strings"

// Normalize devuelve el entorno canónico de EventFlow: un valor vacío se
// interpreta como dev y el resto se recorta para descartar espacios
// accidentales de la configuración. La validación de los valores admitidos
// corresponde al consumidor (cmd/config).
func Normalize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "dev"
	}
	return value
}
