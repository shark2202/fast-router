package router

// Meta + upstream-aware routing: (a) unhealthy upstreams lose traffic while
// healthy alternatives exist; (b) requests exceeding a model's context window
// are gated before forwarding.
import (
	"context"
	"path/filepath"
	"testing"
)

func metaRegistry() []ModelEntry {
	return []ModelEntry{
		{"test/cheap", "cheap", "Cheap", 128_000, 0.1, 0.2,
			map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
		{"test/bigctx", "bigctx", "BigCtx", 200_000, 5.0, 10.0,
			map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
		{"test/tinyctx", "tinyctx", "TinyCtx", 4_096, 0.05, 0.1,
			map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
	}
}

func metaUpstreams() map[string]Upstream {
	return map[string]Upstream{
		"cheap":   {Name: "cheap", BaseURL: "https://c.example", Protocol: "openai"},
		"bigctx":  {Name: "bigctx", BaseURL: "https://b.example", Protocol: "openai"},
		"tinyctx": {Name: "tinyctx", BaseURL: "https://t.example", Protocol: "openai"},
	}
}

func TestUnhealthyUpstreamSkipped(t *testing.T) {
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	// cheap upstream melting down (429s), bigctx healthy
	for i := 0; i < 5; i++ {
		store.Record(VerdictEvent{Upstream: "cheap", Outcome: "upstream_error"})
	}
	for i := 0; i < 5; i++ {
		store.Record(VerdictEvent{Upstream: "bigctx", Outcome: "ok"})
	}
	h := store.UpstreamHealths()
	if h["cheap"].ErrorRate < 0.5 || h["cheap"].Events < 3 {
		t.Fatalf("health = %+v", h["cheap"])
	}
	eng := &fixedConfEngine{choice: "A", conf: 0.9}
	// cold start would pick cheapest (test/cheap); health must flip to bigctx
	up, model, _, _, err := scoreRouteHealth(context.Background(), eng, "write code", metaRegistry(), metaUpstreams(), nil, h, 0)
	if err != nil {
		t.Fatal(err)
	}
	if up == "cheap" || model == "test/cheap" {
		t.Fatalf("routed to %s/%s — unhealthy cheap upstream must lose traffic", up, model)
	}
}

func TestHealthKeepsLastResort(t *testing.T) {
	// ALL upstreams unhealthy → routing must not fail (better degraded than dead)
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	for i := 0; i < 4; i++ {
		store.Record(VerdictEvent{Upstream: "cheap", Outcome: "upstream_error"})
		store.Record(VerdictEvent{Upstream: "bigctx", Outcome: "upstream_error"})
	}
	h := store.UpstreamHealths()
	up, _, _, _, err := scoreRouteHealth(context.Background(), &fixedConfEngine{choice: "A", conf: 0.9}, "x", metaRegistry(), metaUpstreams(), nil, h, 0)
	if err != nil {
		t.Fatalf("all-unhealthy must still route: %v", err)
	}
	if up == "" {
		t.Fatal("no upstream chosen")
	}
}

func TestContextWindowGate(t *testing.T) {
	// tinyctx is cheapest but its 4k window can't fit a ~10k-token request;
	// the gate must promote the next candidate instead of failing later.
	_, model, _, _, err := scoreRouteHealth(context.Background(), &fixedConfEngine{choice: "A", conf: 0.9}, "long task", metaRegistry(), metaUpstreams(), nil, nil, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if model == "test/tinyctx" {
		t.Fatalf("10k-token request routed to a 4k-window model — context gate failed")
	}
	if model != "test/cheap" {
		t.Fatalf("expected cheap (128k) fallback, got %s", model)
	}
}
