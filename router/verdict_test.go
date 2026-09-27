package router

// C7 verdict feedback loop (design consensus v0.1, AC8): append-only collection
// of (task_turn, model, verdict) tuples from real routing results, plus the
// C8/C9 data foundation — the measured matrix and empty-layer detection.
// LLM-scout label proposals + independent statistical review (C8 proper) and
// task-type set deltas (C9 proper) consume this data; they are intentionally
// out of scope here (manual/future process).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestGatewayWithUpstream(t *testing.T, status int) (*Gateway, *httptest.Server, *VerdictStore) {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(up.Close)
	gw := NewGateway(nil, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: up.URL, Protocol: "openai"},
	})
	store := NewVerdictStore(filepath.Join(t.TempDir(), "verdicts.jsonl"))
	gw.SetVerdicts(store)
	return gw, up, store
}

func postChat(t *testing.T, gw *Gateway, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	return rec
}

func TestVerdictRecordedOK(t *testing.T) {
	gw, _, store := newTestGatewayWithUpstream(t, 200)
	rec := postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	events := store.Recent(10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Via != "hint" || e.TaskTurn != "new" || e.Outcome != "ok" || e.Status != 200 {
		t.Fatalf("event = %+v", e)
	}
	if e.ModelID != "m" || e.Upstream != "openai" || e.Session == "" {
		t.Fatalf("event = %+v", e)
	}
}

func TestVerdictUpstreamError(t *testing.T) {
	gw, _, store := newTestGatewayWithUpstream(t, 500)
	postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	events := store.Recent(10)
	if len(events) != 1 || events[0].Outcome != "upstream_error" || events[0].Status != 500 {
		t.Fatalf("events = %+v", events)
	}
}

func TestVerdictConnectError(t *testing.T) {
	gw := NewGateway(nil, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "http://127.0.0.1:1/v1", Protocol: "openai"},
	})
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	gw.SetVerdicts(store)
	postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	events := store.Recent(10)
	if len(events) != 1 || events[0].Outcome != "connect_error" || events[0].Status != 0 {
		t.Fatalf("events = %+v", events)
	}
}

func TestVerdictJevRouteAndInherit(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer up.Close()
	engine := &recordingEngine{
		resp: System1Response{Model: "remote", Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "A", Confidence: 1},
		}},
	}
	gw := NewGateway(engine, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: up.URL, Protocol: "openai"},
	})
	store := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	gw.SetVerdicts(store)

	postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"implement a function"}]}`)
	postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"implement a function"},{"role":"assistant","tool_calls":[{"id":"1"}]},{"role":"tool","tool_call_id":"1","content":"r"}]}`)

	events := store.Recent(10) // newest first
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[1].Via != "jev" || events[1].TaskCode != "A" || events[1].TaskTurn != "new" {
		t.Fatalf("first = %+v", events[1])
	}
	if events[0].Via != "inherit" || events[0].TaskTurn != "continue" || events[0].TaskCode != "A" {
		t.Fatalf("second = %+v", events[0])
	}
}

func TestVerdictStorePersistsJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v.jsonl")
	s1 := NewVerdictStore(path)
	s1.Record(VerdictEvent{Session: "s1", ModelID: "m1", Outcome: "ok"})
	s2 := NewVerdictStore(path) // reload
	if got := len(s2.Recent(10)); got != 1 {
		t.Fatalf("reloaded events = %d, want 1", got)
	}
}

func TestMeasuredMatrixAndEmptyLayers(t *testing.T) {
	s := NewVerdictStore(filepath.Join(t.TempDir(), "v.jsonl"))
	s.Record(VerdictEvent{TaskCode: "A", ModelID: "m1", Outcome: "ok"})
	s.Record(VerdictEvent{TaskCode: "A", ModelID: "m1", Outcome: "ok"})
	s.Record(VerdictEvent{TaskCode: "A", ModelID: "m1", Outcome: "upstream_error"})
	s.Record(VerdictEvent{TaskCode: "B", ModelID: "m2", Outcome: "ok"})

	rep := s.Measured()
	if rep.Total != 4 {
		t.Fatalf("total = %d", rep.Total)
	}
	var cellA *MeasuredCell
	for i := range rep.Cells {
		if rep.Cells[i].TaskCode == "A" && rep.Cells[i].ModelID == "m1" {
			cellA = &rep.Cells[i]
		}
	}
	if cellA == nil || cellA.Count != 3 || cellA.OK != 2 || cellA.Err != 1 {
		t.Fatalf("cellA = %+v", cellA)
	}
	if cellA.OKRate < 0.66 || cellA.OKRate > 0.67 {
		t.Fatalf("okRate = %v", cellA.OKRate)
	}
	// empty layers: seed types with zero decisions (C has none)
	found := false
	for _, c := range rep.EmptyLayers {
		if c == "C" {
			found = true
		}
	}
	if !found {
		t.Fatalf("empty layers = %v, want C included", rep.EmptyLayers)
	}
}

func TestAdminVerdictEndpoints(t *testing.T) {
	gw, _, store := newTestGatewayWithUpstream(t, 200)
	postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	admin := NewAdmin(&Config{Listen: ":0"}, os.DevNull, gw)

	for _, ep := range []string{"/api/verdicts", "/api/measured"} {
		rec := httptest.NewRecorder()
		admin.ServeHTTP(rec, httptest.NewRequest("GET", ep, nil))
		if rec.Code != 200 {
			t.Fatalf("%s status = %d", ep, rec.Code)
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("%s not json: %v", ep, err)
		}
	}
	_ = store
}

func TestVersionedBaseURL(t *testing.T) {
	cases := map[string]bool{
		"https://open.bigmodel.cn/api/paas/v4": true,
		"https://api.openai.com/v1":            true,
		"https://api.anthropic.com":            false,
		"https://api.deepseek.com/v1/":         true, // trailing slash tolerated
		"https://example.com/very":             false, // letters after v
		"https://example.com/v":                false, // no digits
		"https://example.com/v12":              true,
	}
	for base, want := range cases {
		if got := versionedBaseURL(base); got != want {
			t.Errorf("versionedBaseURL(%q) = %v, want %v", base, got, want)
		}
	}
}
