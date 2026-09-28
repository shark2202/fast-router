package router

// Two-tier cascade scoring for high-frequency agent workloads:
//   fast tier — small model, synchronous (~2s), gives the FIRST request an
//               informed route instead of the arbitrary default upstream
//   slow tier — 9B, background (~38.6s), overwrites the session cache with
//               the accurate route; continuations inherit it
import (
	"context"
	"testing"
	"time"
)

func TestFastTierGivesImmediateInformedRoute(t *testing.T) {
	slow := &slowEngine{delay: 100 * time.Millisecond} // choice A (per slowEngine impl)
	fast := &recordingEngine{
		resp: System1Response{Model: "remote", Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "B", Confidence: 1},
		}},
	}
	gw := NewGateway(slow, SeedRegistry, map[string]Upstream{
		"openai":    {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
		"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", Protocol: "anthropic"},
		"deepseek":  {Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", Protocol: "openai"},
	})
	gw.SetAsyncScore(true)
	gw.SetFastEngine(fast)

	// first request: fast tier routes synchronously (choice B → distinct model)
	dec, err := gw.route(context.Background(), newTaskMsgs("implement a function"), "client-hint", "openai")
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if dec.Via != "fast" {
		t.Fatalf("via = %q, want fast", dec.Via)
	}
	if dec.ModelID == "client-hint" || dec.ModelID == "" {
		t.Fatalf("modelID = %q, want fast-tier informed route", dec.ModelID)
	}
	if dec.TaskCode != "B" {
		t.Fatalf("taskCode = %q, want B", dec.TaskCode)
	}

	// slow tier still spawns and OVERWRITES the session cache (accuracy wins)
	waitFor(t, 2*time.Second, "slow backfill", func() bool {
		gw.sessionsMu.Lock()
		defer gw.sessionsMu.Unlock()
		c, ok := gw.sessions[sessionKey(newTaskMsgs("implement a function"))]
		return ok && c.taskCode == "A"
	})
	// continuation inherits the SLOW (accurate) route
	dec, err = gw.route(context.Background(), toolLoopMsgs, "client-hint", "openai")
	if err != nil {
		t.Fatalf("continuation error = %v", err)
	}
	if dec.Via != "inherit" || dec.TaskCode != "A" {
		t.Fatalf("continuation = via %q code %q, want inherit/A", dec.Via, dec.TaskCode)
	}
}

func TestFastTierFailureFallsBackToHint(t *testing.T) {
	slow := &slowEngine{delay: 50 * time.Millisecond}
	fast := &recordingEngine{} // zero value: empty answers → error path
	gw := NewGateway(slow, SeedRegistry, map[string]Upstream{
		"openai":    {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
		"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", Protocol: "anthropic"},
		"deepseek":  {Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", Protocol: "openai"},
	})
	gw.SetAsyncScore(true)
	gw.SetFastEngine(fast)

	dec, err := gw.route(context.Background(), newTaskMsgs("do it"), "client-hint", "openai")
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if dec.Via != "hint" || dec.ModelID != "client-hint" {
		t.Fatalf("fallback = %+v, want hint/client-hint", dec)
	}
	// slow tier still backfills
	waitFor(t, 2*time.Second, "slow backfill", func() bool {
		gw.sessionsMu.Lock()
		defer gw.sessionsMu.Unlock()
		_, ok := gw.sessions[sessionKey(newTaskMsgs("do it"))]
		return ok
	})
}

func TestFastTierTimeoutFallsBack(t *testing.T) {
	slow := &slowEngine{delay: 30 * time.Millisecond}
	fast := &slowEngine{delay: 300 * time.Millisecond} // exceeds the 50ms budget
	gw := NewGateway(slow, SeedRegistry, map[string]Upstream{
		"openai":    {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
		"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", Protocol: "anthropic"},
		"deepseek":  {Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", Protocol: "openai"},
	})
	gw.SetAsyncScore(true)
	gw.SetFastEngine(fast)
	gw.SetFastBudget(50 * time.Millisecond)

	start := time.Now()
	dec, err := gw.route(context.Background(), newTaskMsgs("do it"), "client-hint", "openai")
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if dec.Via != "hint" {
		t.Fatalf("via = %q, want hint after fast-tier timeout", dec.Via)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("fast tier blocked %v — budget not enforced", elapsed)
	}
}

func TestDefaultUpstreamDeterministic(t *testing.T) {
	gw := NewGateway(nil, SeedRegistry, map[string]Upstream{
		"zeta":  {Name: "zeta", BaseURL: "https://z.example", Protocol: "openai"},
		"alpha": {Name: "alpha", BaseURL: "https://a.example", Protocol: "openai"},
	})
	a := gw.defaultUpstream().Name
	b := gw.defaultUpstream().Name
	if a != b {
		t.Fatalf("default upstream not deterministic: %q vs %q", a, b)
	}
	if a != "alpha" {
		t.Fatalf("default = %q, want lexicographically-first alpha", a)
	}
}

func TestContinuationInheritsFastRouteBeforeSlowBackfill(t *testing.T) {
	slow := &slowEngine{delay: 500 * time.Millisecond} // still scoring
	fast := &recordingEngine{
		resp: System1Response{Model: "remote", Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "A", Confidence: 1},
		}},
	}
	gw := NewGateway(slow, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
	})
	gw.SetAsyncScore(true)
	gw.SetFastEngine(fast)

	if _, err := gw.route(context.Background(), newTaskMsgs("implement a function"), "client-hint", "openai"); err != nil {
		t.Fatal(err)
	}
	// continuation arrives while the slow tier is still scoring — it must
	// inherit the FAST-tier cached route (no re-score, no waiting).
	dec, err := gw.route(context.Background(), toolLoopMsgs, "client-hint", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Via != "inherit" || dec.TaskCode != "A" {
		t.Fatalf("continuation = via %q code %q, want inherit/A (fast-tier seed)", dec.Via, dec.TaskCode)
	}
	if n := fast.calls; n != 1 {
		t.Fatalf("fast engine called %d times, want 1 (no re-score on continuation)", n)
	}
}
