package router

import (
	"context"
	"testing"
)

func TestNativeSystemOneEngineEvaluatesThroughScorer(t *testing.T) {
	backend := mockBackend{logits: []float64{5, 1}}
	native := NewNativeSystemOneEngine(NewScorer(backend, "native-test"))

	got, err := native.Evaluate(context.Background(), System1Request{
		State: "route this request",
		Questions: map[string]Question{
			"task_type": {
				Type:         "choice",
				Instructions: "pick one",
				Criteria:     map[string]string{"a": "first", "b": "second"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if got.Model != "native-test" {
		t.Fatalf("model = %q, want native-test", got.Model)
	}
	if got.Answers["task_type"].Choice != "a" {
		t.Fatalf("choice = %q, want a", got.Answers["task_type"].Choice)
	}
}

func TestNativeSystemOneEngineHonorsCanceledContextBeforeScoring(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	native := NewNativeSystemOneEngine(NewScorer(mockBackend{logits: []float64{1}}, "native-test"))
	_, err := native.Evaluate(ctx, System1Request{})
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
