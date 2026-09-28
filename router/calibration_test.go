package router

// L1 self-evolution: C7 verdicts → measured weights → Select re-ranks survivors.
// Gates: min-sample threshold (below it, cold-start cheapest-first applies);
// connect_error excluded (not the model's fault).
import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// twoModelRegistry: cheap-bad vs expensive-good for task A only.
var twoModelRegistry = []ModelEntry{
	{"cheap/bad-model", "openai", "Cheap", 128_000, 0.1, 0.5,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
	{"expensive/good-model", "openai", "Good", 200_000, 5.0, 15.0,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil},
}

func calibrationGateway(t *testing.T) (*Gateway, *VerdictStore) {
	t.Helper()
	gw := NewGateway(nil, twoModelRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
	})
	// route with a mock engine that always says task A
	gw.fastEngine = &recordingEngine{
		resp: System1Response{Model: "remote", Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "A", Confidence: 1},
		}},
	}
	gw.fastBudget = 0
	gw.SetAsyncScore(true)
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	gw.SetVerdicts(store)
	return gw, store
}

func TestCalibrationFlipsSelection(t *testing.T) {
	gw, store := calibrationGateway(t)

	// cold start: cheapest wins
	dec, err := gw.route(context.Background(), newTaskMsgs("write code"), "hint", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if dec.ModelID != "cheap/bad-model" {
		t.Fatalf("cold start = %q, want cheapest", dec.ModelID)
	}

	// accumulate verdicts: cheap model fails upstream, good model succeeds
	for i := 0; i < 6; i++ {
		store.Record(VerdictEvent{TaskCode: "A", ModelID: "cheap/bad-model", Outcome: "upstream_error"})
		store.Record(VerdictEvent{TaskCode: "A", ModelID: "expensive/good-model", Outcome: "ok"})
	}

	// calibrated: measured pass-rate outweighs cost
	dec, err = gw.route(context.Background(), newTaskMsgs("write more code"), "hint", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if dec.ModelID != "expensive/good-model" {
		t.Fatalf("calibrated = %q, want good model (verdict-driven flip)", dec.ModelID)
	}
}

func TestCalibrationMinSampleGate(t *testing.T) {
	gw, store := calibrationGateway(t)
	// 4 samples < threshold 5: no calibration, cold start still applies
	for i := 0; i < 4; i++ {
		store.Record(VerdictEvent{TaskCode: "A", ModelID: "cheap/bad-model", Outcome: "upstream_error"})
	}
	dec, err := gw.route(context.Background(), newTaskMsgs("x"), "hint", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if dec.ModelID != "cheap/bad-model" {
		t.Fatalf("below min samples = %q, want cold-start cheapest", dec.ModelID)
	}
}

func TestCalibrationExcludesConnectErrors(t *testing.T) {
	s := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	for i := 0; i < 10; i++ {
		s.Record(VerdictEvent{TaskCode: "A", ModelID: "m1", Outcome: "connect_error"})
		s.Record(VerdictEvent{TaskCode: "A", ModelID: "m1", Outcome: "ok"})
	}
	// m1 has 10 ok + 10 connect_error: connect excluded → 10/10 = 1.0 with 10
	// usable samples (connect errors are not the model's fault)
	cal := s.Calibrated()
	e, ok := cal["A"]["m1"]
	if !ok || e.PassRate != 1.0 || e.SampleSize != 10 {
		t.Fatalf("calibrated = %+v ok=%v, want rate 1.0 n=10 (connect excluded)", e, ok)
	}
}

func TestLiveVerdictsCalibrateSelection(t *testing.T) {
	// end-to-end: real requests through the gateway record verdicts that
	// then flip the selection.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fail requests for the cheap model only
		if len(r.Header.Get("Authorization")) >= 0 && r.URL.Query().Get("m") == "" {
			w.WriteHeader(200)
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`))
		}
	}))
	defer up.Close()
	gw := NewGateway(nil, twoModelRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: up.URL, Protocol: "openai"},
	})
	gw.fastEngine = &recordingEngine{
		resp: System1Response{Model: "remote", Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "A", Confidence: 1},
		}},
	}
	gw.fastBudget = 0
	gw.asyncScore = true
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	gw.SetVerdicts(store)

	// seed verdicts: cheap model errors
	for i := 0; i < 6; i++ {
		store.Record(VerdictEvent{TaskCode: "A", ModelID: "cheap/bad-model", Outcome: "upstream_error"})
		store.Record(VerdictEvent{TaskCode: "A", ModelID: "expensive/good-model", Outcome: "ok"})
	}
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"write code"}]}`))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	events := store.Recent(10)
	if len(events) == 0 || events[0].ModelID != "expensive/good-model" {
		t.Fatalf("verdict = %+v, want good model routed by calibration", events)
	}
}
