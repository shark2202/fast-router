package router

// C-017: confidence-gated MoA escalation — fan-out to references, aggregate,
// seamless to the client. Gates: low confidence only, no tool requests,
// aggregator/no-reference failures fall back to the routed model.
import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type moaUpstreams struct {
	ref1, ref2, agg, routed                 *httptest.Server
	ref1Hits, ref2Hits, aggHits, routedHits int
	mu                                      sync.Mutex
}

func newMoaFixture(t *testing.T) *moaUpstreams {
	f := &moaUpstreams{}
	mk := func(hit *int, resp string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			*hit++
			f.mu.Unlock()
			w.Write([]byte(resp))
		}))
	}
	f.ref1 = mk(&f.ref1Hits, `{"choices":[{"message":{"role":"assistant","content":"ref1-answer"}}]}`)
	f.ref2 = mk(&f.ref2Hits, `{"choices":[{"message":{"role":"assistant","content":"ref2-answer"}}]}`)
	f.agg = mk(&f.aggHits, `{"choices":[{"message":{"role":"assistant","content":"SYNTHESIZED"}}]}`)
	f.routed = mk(&f.routedHits, `{"choices":[{"message":{"role":"assistant","content":"routed-answer"}}]}`)
	t.Cleanup(func() { f.ref1.Close(); f.ref2.Close(); f.agg.Close(); f.routed.Close() })
	return f
}

func moaGateway(f *moaUpstreams, conf float64) *Gateway {
	reg := []ModelEntry{{"test/model", "routed", "Test", 128_000, 1.0, 2.0,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high"}, nil}}
	gw := NewGateway(nil, reg, map[string]Upstream{
		"ref1":   {Name: "ref1", BaseURL: f.ref1.URL, Protocol: "openai"},
		"ref2":   {Name: "ref2", BaseURL: f.ref2.URL, Protocol: "openai"},
		"agg":    {Name: "agg", BaseURL: f.agg.URL, Protocol: "openai"},
		"routed": {Name: "routed", BaseURL: f.routed.URL, Protocol: "openai"},
	})
	gw.SetMoA(&MoAConfig{
		Enabled: true, MinConfidence: 0.5,
		References: []MoAModel{
			{Upstream: "ref1", Model: "ref1/m", Temperature: 0.6},
			{Upstream: "ref2", Model: "ref2/m", Temperature: 0.6},
		},
		Aggregator: MoAModel{Upstream: "agg", Model: "agg/m", Temperature: 0.4},
	})
	// fixed-confidence engine with an openai-routable registry entry
	gw.SetFastEngine(&fixedConfEngine{choice: "A", conf: conf})
	gw.SetAsyncScore(true)
	return gw
}

type fixedConfEngine struct {
	choice string
	conf   float64
}

func (e *fixedConfEngine) Evaluate(context.Context, System1Request) (System1Response, error) {
	return System1Response{Model: "remote", Answers: map[string]Answer{
		"task_type": {Type: "choice", Choice: e.choice, Confidence: e.conf},
	}}, nil
}

func TestMoALowConfidenceEscalates(t *testing.T) {
	f := newMoaFixture(t)
	gw := moaGateway(f, 0.2) // low confidence → MoA
	rec := postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hard question"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "SYNTHESIZED") {
		t.Fatalf("client got %q, want aggregator output (seamless MoA)", rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ref1Hits == 0 || f.ref2Hits == 0 || f.aggHits == 0 {
		t.Fatalf("hits: ref1=%d ref2=%d agg=%d — fan-out did not happen", f.ref1Hits, f.ref2Hits, f.aggHits)
	}
	if f.routedHits != 0 {
		t.Fatalf("routed model hit %d times — should be bypassed on escalation", f.routedHits)
	}
}

func TestMoAHighConfidenceBypasses(t *testing.T) {
	f := newMoaFixture(t)
	gw := moaGateway(f, 0.95) // high confidence → plain routing
	rec := postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"easy question"}]}`)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.aggHits != 0 || f.ref1Hits != 0 {
		t.Fatalf("MoA ran on a high-confidence route (agg=%d ref1=%d)", f.aggHits, f.ref1Hits)
	}
}

func TestMoAToolRequestsSkip(t *testing.T) {
	f := newMoaFixture(t)
	gw := moaGateway(f, 0.1) // low confidence BUT tools present
	postChat(t, gw, `{"model":"m","tools":[{"type":"function"}],"messages":[{"role":"user","content":"use a tool"}]}`)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.aggHits != 0 {
		t.Fatalf("MoA must skip tool-call requests (agg hits = %d)", f.aggHits)
	}
}

func TestMoAAllReferencesDeadFallsBack(t *testing.T) {
	// pre-flight fallback: when no reference survives, the routed model
	// serves the request (mid-proxy aggregator failures pass through to the
	// client, same as single-model routing — documented v1 boundary)
	f := newMoaFixture(t)
	f.ref1.Close()
	f.ref2.Close()
	gw := moaGateway(f, 0.2)
	rec := postChat(t, gw, `{"model":"m","messages":[{"role":"user","content":"hard"}]}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "routed-answer") {
		t.Fatalf("fallback failed: %d %q", rec.Code, rec.Body.String())
	}
}
