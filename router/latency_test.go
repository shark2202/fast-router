package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// hot-path latency: inherit-routed requests must add ~µs, not ms
func TestHotPathLatency(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`))
	}))
	defer up.Close()
	gw := NewGateway(nil, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: up.URL, Protocol: "openai"},
	})
	// warm
	body := `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
	for i := 0; i < 5; i++ {
		postChat(t, gw, body)
	}
	n := 200
	start := time.Now()
	for i := 0; i < n; i++ {
		postChat(t, gw, body)
	}
	per := time.Since(start) / time.Duration(n)
	t.Logf("gateway added latency per request (incl. httptest upstream RTT): %v", per)
	if per > 5*time.Millisecond {
		t.Fatalf("hot path too slow: %v per request", per)
	}
	_ = strings.Repeat("x", 0)
}
