package router

// Virtual model-name contract: clients use one FIXED name ("fast-router");
// every routing path must resolve to a REAL backend model. A virtual/unknown
// hint maps to the cheapest real model on the target upstream; a real model
// id passes through (client intent).
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVirtualNameResolvesToRealModel(t *testing.T) {
	// capture what actually reaches the upstream
	var forwarded string
	gw := NewGateway(nil, []ModelEntry{
		{"cheap/real-mini", "openai", "Mini", 128_000, 0.1, 0.2,
			map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
	}, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "http://capture.invalid", Protocol: "openai"},
	})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8192)
		n, _ := r.Body.Read(buf)
		forwarded = string(buf[:n])
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer up.Close()
	gw.SetUpstreams(map[string]Upstream{"openai": {Name: "openai", BaseURL: up.URL, Protocol: "openai"}})

	rec := postChat(t, gw, `{"model":"fast-router","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(forwarded, `"model":"cheap/real-mini"`) {
		t.Fatalf("virtual name leaked or misresolved; upstream saw: %s", forwarded)
	}
}

func TestRealModelHintPassesThrough(t *testing.T) {
	gw2 := NewGateway(nil, []ModelEntry{
		{"cheap/real-mini", "openai", "Mini", 128_000, 0.1, 0.2,
			map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
	}, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://x", Protocol: "openai"},
	})
	dec, err := gw2.route(nil, []map[string]any{{"role": "user", "content": "hi"}}, "cheap/real-mini", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if dec.ModelID != "cheap/real-mini" {
		t.Fatalf("real hint rewritten to %q — client intent must pass through", dec.ModelID)
	}
}
