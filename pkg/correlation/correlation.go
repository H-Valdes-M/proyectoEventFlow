// Package correlation transporta el identificador de correlación entre capas.
package correlation

import "context"

type key struct{}

// WithID adjunta un identificador ya validado al contexto.
func WithID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, key{}, id) }

// ID obtiene la correlación actual sin inventarla fuera del borde HTTP.
func ID(ctx context.Context) string { value, _ := ctx.Value(key{}).(string); return value }
