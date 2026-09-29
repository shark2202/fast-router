package router

// R5: async first-score + route cache (see docs/fast-router-引擎评估-2026-09-27.md).
//
// On a new task turn with async enabled, route() must:
//   1. return the fallback (default upstream + client hint) immediately —
//      the 77s CPU scoring must NOT block the request;
//   2. spawn a single-flight background score that backfills the session
//      cache, so subsequent tool-loop turns inherit the Jev route;
//   3. never let a background engine error fail a request or poison the cache;
//   4. keep the synchronous (blocking) path byte-identical when disabled.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// slowEngine mimics a CPU-bound system-one engine (e.g. 9B GGUF on CPU).
type slowEngine struct {
	mu    sync.Mutex
	calls int
	delay time.Duration
	err   error
}

func (e *slowEngine) Evaluate(ctx context.Context, _ System1Request) (System1Response, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	select {
	case <-time.After(e.delay):
	case <-ctx.Done():
		return System1Response{}, ctx.Err()
	}
	if e.err != nil {
		return System1Response{}, e.err
	}
	return System1Response{
		Model: "remote",
		Answers: map[string]Answer{
			"task_type": {Type: "choice", Choice: "A", Confidence: 1},
		},
	}, nil
}

func (e *slowEngine) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func newTaskMsgs(text string) []map[string]any {
	return []map[string]any{{"role": "user", "content": text}}
}

var toolLoopMsgs = []map[string]any{
	{"role": "user", "content": "implement a function"},
	{"role": "assistant", "tool_calls": []any{"x"}},
	{"role": "tool", "tool_call_id": "1", "content": "result"},
}

func asyncTestGateway(engine SystemOneEngine) *Gateway {
	gw := NewGateway(engine, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
	})
	gw.SetAsyncScore(true)
	return gw
}

func waitFor(t *testing.T, timeout time.Duration, name string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("%s: condition not met within %v", name, timeout)
}

func TestAsyncRouteServesFallbackThenBackfills(t *testing.T) {
	engine := &slowEngine{delay: 80 * time.Millisecond}
	gw := asyncTestGateway(engine)

	start := time.Now()
	dec, err := gw.route(context.Background(), newTaskMsgs("implement a function"), "client-hint", "openai")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if elapsed >= 80*time.Millisecond {
		t.Fatalf("route() blocked %v — async path must not wait for scoring", elapsed)
	}
	if dec.Upstream.Name != "openai" || dec.ModelID != "qwen/qwen-max" {
		t.Fatalf("fallback = %q/%q, want openai + resolved real model (virtual name contract)", dec.Upstream.Name, dec.ModelID)
	}
	// background score spawned (goroutine may not have started yet — wait for it)
	waitFor(t, 2*time.Second, "score spawned", func() bool { return engine.callCount() >= 1 })

	// background score completes → session cache backfilled
	waitFor(t, 2*time.Second, "backfill", func() bool {
		gw.sessionsMu.Lock()
		defer gw.sessionsMu.Unlock()
		_, ok := gw.sessions[sessionKey(newTaskMsgs("implement a function"))]
		return ok
	})

	// tool-loop continuation inherits the scored route without a second evaluation
	dec, err = gw.route(context.Background(), toolLoopMsgs, "client-hint", "openai")
	if err != nil {
		t.Fatalf("continuation route() error = %v", err)
	}
	if dec.ModelID == "client-hint" || dec.ModelID == "" {
		t.Fatalf("continuation modelID = %q, want inherited Jev route (not the hint)", dec.ModelID)
	}
	if dec.Upstream.Name != "openai" {
		t.Fatalf("continuation upstream = %q, want openai", dec.Upstream.Name)
	}
	if n := engine.callCount(); n != 1 {
		t.Fatalf("engine calls = %d after inheritance, want 1", n)
	}
}

func TestAsyncRouteSingleFlight(t *testing.T) {
	engine := &slowEngine{delay: 80 * time.Millisecond}
	gw := asyncTestGateway(engine)

	msgs := newTaskMsgs("implement a function")
	key := sessionKey(msgs)

	// two rapid requests in the same session before the score completes
	for i := 0; i < 2; i++ {
		_, err := gw.route(context.Background(), msgs, "client-hint", "openai")
		if err != nil {
			t.Fatalf("route #%d error = %v", i, err)
		}
	}

	waitFor(t, 2*time.Second, "backfill", func() bool {
		gw.sessionsMu.Lock()
		defer gw.sessionsMu.Unlock()
		_, ok := gw.sessions[key]
		return ok
	})
	if n := engine.callCount(); n != 1 {
		t.Fatalf("engine calls = %d, want 1 (single-flight)", n)
	}
}

func TestAsyncRouteEngineErrorDoesNotPoisonCache(t *testing.T) {
	engine := &slowEngine{delay: 30 * time.Millisecond, err: errors.New("backend down")}
	gw := asyncTestGateway(engine)
	msgs := newTaskMsgs("implement a function")
	key := sessionKey(msgs)

	dec, err := gw.route(context.Background(), msgs, "client-hint", "openai")
	if err != nil {
		t.Fatalf("route() error = %v — async path must absorb engine errors", err)
	}
	if dec.Upstream.Name != "openai" || dec.ModelID != "qwen/qwen-max" {
		t.Fatalf("fallback = %q/%q, want openai + resolved real model (virtual name contract)", dec.Upstream.Name, dec.ModelID)
	}

	// score fails; inflight cleared; cache stays empty
	waitFor(t, 2*time.Second, "inflight cleared", func() bool {
		gw.scoringMu.Lock()
		defer gw.scoringMu.Unlock()
		_, busy := gw.scoring[key]
		return !busy
	})
	gw.sessionsMu.Lock()
	_, cached := gw.sessions[key]
	gw.sessionsMu.Unlock()
	if cached {
		t.Fatalf("failed score must not poison the session cache")
	}

	// next request retries scoring (still serves fallback, never fails)
	if _, err := gw.route(context.Background(), msgs, "client-hint", "openai"); err != nil {
		t.Fatalf("second route() error = %v", err)
	}
	waitFor(t, 2*time.Second, "retry spawned", func() bool { return engine.callCount() >= 2 })
}

// TestSessionKeyStableAcrossToolLoop: the session key must be identical for
// turn 1 and every growing tool-loop continuation of the same task, and must
// differ across tasks. (Regression for the delivered-v1.0 key scheme that
// hashed the growing prefix and thus never hit the cache in real flows.)
func TestSessionKeyStableAcrossToolLoop(t *testing.T) {
	k1 := sessionKey(newTaskMsgs("implement a function"))
	k2 := sessionKey(toolLoopMsgs)
	grown := append(append([]map[string]any{}, toolLoopMsgs...),
		map[string]any{"role": "assistant", "tool_calls": []any{"y"}},
		map[string]any{"role": "tool", "tool_call_id": "2", "content": "r2"},
	)
	k3 := sessionKey(grown)
	if k1 == "" || k1 != k2 || k2 != k3 {
		t.Fatalf("session key not stable across tool loop: %q vs %q vs %q", k1, k2, k3)
	}
	if other := sessionKey(newTaskMsgs("write a poem")); other == k1 {
		t.Fatalf("distinct tasks must not collide: %q", k1)
	}
}

func TestSyncRouteBlockingPreserved(t *testing.T) {
	engine := &slowEngine{delay: 60 * time.Millisecond}
	gw := NewGateway(engine, SeedRegistry, map[string]Upstream{
		"openai": {Name: "openai", BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
	})
	// async disabled (default): route() must block and return the scored route.

	start := time.Now()
	dec, err := gw.route(context.Background(), newTaskMsgs("implement a function"), "client-hint", "openai")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("route() error = %v", err)
	}
	if elapsed < 60*time.Millisecond {
		t.Fatalf("route() returned in %v — sync path must wait for scoring", elapsed)
	}
	if dec.ModelID == "client-hint" || dec.ModelID == "" {
		t.Fatalf("sync modelID = %q, want scored route", dec.ModelID)
	}
	if dec.Upstream.Name != "openai" {
		t.Fatalf("sync upstream = %q", dec.Upstream.Name)
	}
}
