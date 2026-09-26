package router

import (
	"context"
	"testing"
)

type recordingEngine struct {
	calls int
	resp  System1Response
	err   error
}

func (e *recordingEngine) Evaluate(_ context.Context, _ System1Request) (System1Response, error) {
	e.calls++
	return e.resp, e.err
}

func TestGatewayRouteUsesSystemOneEngine(t *testing.T) {
	engine := &recordingEngine{
		resp: System1Response{
			Model: "remote",
			Answers: map[string]Answer{
				"task_type": {
					Type:       "choice",
					Choice:     "A",
					Confidence: 1,
				},
			},
		},
	}
	gw := NewGateway(engine, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
	})

	up, modelID, err := gw.route(
		context.Background(),
		[]map[string]any{{"role": "user", "content": "implement a function"}},
		"",
		"openai",
	)
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if engine.calls != 1 {
		t.Fatalf("engine calls = %d, want 1", engine.calls)
	}
	if up.Name != "openai" || modelID == "" {
		t.Fatalf("route = upstream %q, model %q", up.Name, modelID)
	}
}
