package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistryEsConcurrenteYLimitaSeries(t *testing.T) {
	registry := NewRegistry()
	var group sync.WaitGroup
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			registry.ObserveRequest(http.MethodGet, "/orders/:id", 200+index%2, time.Millisecond)
		}(index)
	}
	group.Wait()
	recorder := httptest.NewRecorder()
	registry.Handler(func(*http.Request) (DurableStats, error) {
		return DurableStats{OutboxPending: 2, ProcessingAttempts: 3, Processed: 1, TerminalFailures: 1}, nil
	}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	for _, name := range []string{"eventflow_http_requests_total", "eventflow_outbox_pending 2", "eventflow_terminal_failures_total 1"} {
		if !strings.Contains(body, name) {
			t.Fatalf("falta %q en %s", name, body)
		}
	}
}
