package env

import (
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv lee un archivo opcional de entorno (.env.dev/.env.qa) y
// devuelve sus pares clave=valor. Si el archivo no existe devuelve un mapa
// vacío sin error, porque la carga es opcional; un error de lectura o una
// línea malformada sí es un fallo. Los errores no reproducen el contenido de
// la línea, para no filtrar valores sensibles a los registros.
func LoadDotEnv(path string) (map[string]string, error) {
	values := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, fmt.Errorf("leer el archivo %q: %w", path, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%q: la linea %d no tiene la forma CLAVE=VALOR", path, i+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%q: la linea %d no define un nombre de variable", path, i+1)
		}
		values[key] = stripQuotes(strings.TrimSpace(value))
	}
	return values, nil
}

// stripQuotes elimina comillas dobles o simples cuando el valor está
// completamente entrecomillado.
func stripQuotes(value string) string {
	if length := len(value); length >= 2 {
		if (value[0] == '"' && value[length-1] == '"') || (value[0] == '\'' && value[length-1] == '\'') {
			return value[1 : length-1]
		}
	}
	return value
}
