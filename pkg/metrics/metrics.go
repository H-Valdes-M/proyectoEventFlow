// Package metrics mantiene métricas locales sin una dependencia de monitorización.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

const maxRequestSeries = 32

type requestKey struct {
	method, route string
	status        int
}
type requestValue struct {
	count        uint64
	latencyNanos uint64
}

// DurableStats son los agregados persistidos que pertenecen al worker.
type DurableStats struct{ OutboxPending, ProcessingAttempts, Processed, TerminalFailures int64 }

// Registry protege las series locales y limita su cardinalidad.
type Registry struct {
	mu       sync.Mutex
	requests map[requestKey]requestValue
}

func NewRegistry() *Registry { return &Registry{requests: make(map[requestKey]requestValue)} }

func (r *Registry) ObserveRequest(method, route string, status int, latency time.Duration) {
	if r == nil {
		return
	}
	if route == "" {
		route = "unmatched"
	}
	key := requestKey{method: method, route: route, status: status}
	r.mu.Lock()
	if _, exists := r.requests[key]; !exists && len(r.requests) >= maxRequestSeries {
		key = requestKey{method: "other", route: "other", status: 0}
	}
	value := r.requests[key]
	value.count++
	value.latencyNanos += uint64(latency)
	r.requests[key] = value
	r.mu.Unlock()
}

// Handler expone texto local; la función durable consulta PostgreSQL al scrape.
func (r *Registry) Handler(durable func(*http.Request) (DurableStats, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		r.mu.Lock()
		keys := make([]requestKey, 0, len(r.requests))
		for key := range r.requests {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		values := make(map[requestKey]requestValue, len(keys))
		for _, key := range keys {
			values[key] = r.requests[key]
		}
		r.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		for _, key := range keys {
			value := values[key]
			labels := fmt.Sprintf(`method=%q,route=%q,status=%q`, key.method, key.route, fmt.Sprint(key.status))
			fmt.Fprintf(w, "eventflow_http_requests_total{%s} %d\n", labels, value.count)
			fmt.Fprintf(w, "eventflow_http_request_latency_seconds_sum{%s} %.9f\n", labels, float64(value.latencyNanos)/float64(time.Second))
		}
		if durable == nil {
			return
		}
		stats, err := durable(request)
		if err != nil {
			http.Error(w, "metricas durables no disponibles", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "eventflow_outbox_pending %d\neventflow_processing_attempts %d\neventflow_processed_total %d\neventflow_terminal_failures_total %d\n", stats.OutboxPending, stats.ProcessingAttempts, stats.Processed, stats.TerminalFailures)
	}
}
