# System One Engine Seam Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a high-level `SystemOneEngine` interface so native scoring and HTTP System One sidecars can replace each other without changing Gateway routing logic.

**Architecture:** Keep the existing `Scorer` and low-level `Backend` unchanged. Add a context-aware `SystemOneEngine` interface, wrap the native `Scorer` with `NativeSystemOneEngine`, and make `HTTPSystemOneClient` implement the same interface. Change `Gateway` to depend on the high-level interface while preserving native behavior as the default.

**Tech Stack:** Go 1.25, `net/http`, `encoding/json`, `context`, `httptest`, existing router tests.

## Global Constraints

- Do not change the native `Scorer` algorithm or Zig/libllama ABI.
- Do not add configuration fields or automatic sidecar startup in this change.
- Do not implement `noul` or `score`; preserve the current choice-only routing behavior.
- Keep the current hint-only behavior when no engine is configured.
- Do not add third-party dependencies.
- Preserve current `System1Request` and `System1Response` JSON types.
- Do not modify unrelated dirty files.
- Verify with `go test ./router/... ./router/schema/...` and `go build ./cmd/fast-router`.

---

## Files

### Create

- `router/systemone_engine.go` — high-level interface and native adapter.
- `router/systemone_engine_test.go` — engine seam and native adapter tests.
- `router/systemone_http.go` — generic HTTP System One client.
- `router/systemone_http_test.go` — HTTP protocol, error, cancellation, and endpoint tests.

### Modify

- `router/gateway.go` — replace `*Scorer` dependency with `SystemOneEngine`.
- `cmd/fast-router/main.go` — wrap the existing native scorer with `NativeSystemOneEngine`.

### Reference only

- `router/scorer.go` — existing `Scorer.Score` remains unchanged.
- `docs/System-One-引擎抽象与可替换性.md` — architecture contract.
- `docs/superpowers/specs/2026-09-26-llm2jev-http-adapter-design.md` — HTTP behavior and error contract.

## Task 1: Add the high-level engine interface

**Files:**
- Create: `router/systemone_engine.go`
- Test: `router/systemone_engine_test.go`

**Interfaces:**

```go
type SystemOneEngine interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}

type NativeSystemOneEngine struct {
    scorer *Scorer
}

func NewNativeSystemOneEngine(scorer *Scorer) *NativeSystemOneEngine
func (e *NativeSystemOneEngine) Evaluate(ctx context.Context, req System1Request) (System1Response, error)
```

- [ ] **Step 1: Write the failing interface test**

Add `router/systemone_engine_test.go`:

```go
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
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./router -run 'TestNativeSystemOneEngine' -count=1
```

Expected: compile failure because `NewNativeSystemOneEngine` does not exist.

- [ ] **Step 3: Implement the minimal native adapter**

Add:

```go
package router

import "context"

type SystemOneEngine interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}

type NativeSystemOneEngine struct {
    scorer *Scorer
}

func NewNativeSystemOneEngine(scorer *Scorer) *NativeSystemOneEngine {
    return &NativeSystemOneEngine{scorer: scorer}
}

func (e *NativeSystemOneEngine) Evaluate(ctx context.Context, req System1Request) (System1Response, error) {
    if err := ctx.Err(); err != nil {
        return System1Response{}, err
    }
    if e == nil || e.scorer == nil {
        return System1Response{}, ErrSystemOneEngineUnavailable
    }
    resp, err := e.scorer.Score(req)
    if err != nil {
        return System1Response{}, err
    }
    if err := ctx.Err(); err != nil {
        return System1Response{}, err
    }
    return resp, nil
}
```

Also define:

```go
var ErrSystemOneEngineUnavailable = errors.New("system-one engine unavailable")
```

- [ ] **Step 4: Run the focused tests and verify they pass**

Run:

```bash
go test ./router -run 'TestNativeSystemOneEngine' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run the existing router tests**

Run:

```bash
go test ./router/... ./router/schema/...
```

Expected: all existing tests PASS.

## Task 2: Add the generic HTTP System One engine

**Files:**
- Create: `router/systemone_http.go`
- Test: `router/systemone_http_test.go`

**Interfaces:**

```go
type HTTPSystemOneClient struct {
    endpoint string
    token    string
    client   *http.Client
}

func NewHTTPSystemOneClient(endpoint, token string, client *http.Client) (*HTTPSystemOneClient, error)
func (c *HTTPSystemOneClient) Evaluate(ctx context.Context, req System1Request) (System1Response, error)
```

- [ ] **Step 1: Write failing endpoint and request tests**

Add tests covering:

```go
func TestHTTPSystemOneClientPostsToSystemOneEndpoint(t *testing.T)
func TestHTTPSystemOneClientDoesNotDuplicateSystemOnePath(t *testing.T)
func TestHTTPSystemOneClientSendsBearerToken(t *testing.T)
func TestHTTPSystemOneClientDecodesResponse(t *testing.T)
```

The test server must assert:

```go
r.Method == http.MethodPost
r.URL.Path == "/v1/systemone"
r.Header.Get("Content-Type") == "application/json"
r.Header.Get("Authorization") == "Bearer test-token"
```

Decode the request body into `System1Request` and assert the state, question id, question type, instructions, and criteria.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```bash
go test ./router -run 'TestHTTPSystemOneClient' -count=1
```

Expected: compile failure because `HTTPSystemOneClient` and its constructor do not exist.

- [ ] **Step 3: Write failing error and cancellation tests**

Add:

```go
func TestHTTPSystemOneClientReturnsHTTPErrorForNon2xx(t *testing.T)
func TestHTTPSystemOneClientReturnsDecodeErrorForInvalidJSON(t *testing.T)
func TestHTTPSystemOneClientHonorsContextDeadline(t *testing.T)
func TestNewHTTPSystemOneClientRejectsInvalidEndpoint(t *testing.T)
```

The non-2xx test must assert the returned error can be converted to:

```go
var remoteErr *SystemOneHTTPError
if !errors.As(err, &remoteErr) { ... }
```

The cancellation test must use a test server that blocks until the request context is canceled and assert `errors.Is(err, context.DeadlineExceeded)`.

- [ ] **Step 4: Run the focused tests and verify they fail for the missing implementation**

Run:

```bash
go test ./router -run 'TestHTTPSystemOneClient' -count=1
```

Expected: compile failure until the client implementation exists.

- [ ] **Step 5: Implement endpoint normalization**

Implement a constructor that:

1. rejects an empty endpoint;
2. parses the URL with `net/url`;
3. requires `http` or `https`;
4. requires a host;
5. rejects query and fragment;
6. accepts either a base URL or an existing `/v1/systemone` path;
7. stores exactly one final `/v1/systemone` path.

- [ ] **Step 6: Implement the HTTP request**

`Evaluate` must:

1. return `ctx.Err()` before doing work if already canceled;
2. marshal `System1Request` with `json.Marshal`;
3. create a request using `http.NewRequestWithContext`;
4. set `Content-Type: application/json`;
5. set `Authorization: Bearer <token>` only when token is non-empty;
6. execute through the injected client;
7. read at most 4096 bytes for a non-2xx error body;
8. return `*SystemOneHTTPError` for non-2xx;
9. decode a successful response into `System1Response`;
10. close the response body on every path.

Do not add retries, background goroutines, health checks, configuration fields, or sidecar startup.

- [ ] **Step 7: Run focused HTTP tests and verify they pass**

Run:

```bash
go test ./router -run 'TestHTTPSystemOneClient' -count=1
```

Expected: PASS.

- [ ] **Step 8: Run all router tests**

Run:

```bash
go test ./router/... ./router/schema/...
```

Expected: PASS.

## Task 3: Make Gateway depend on the engine interface

**Files:**
- Modify: `router/gateway.go`
- Modify: `cmd/fast-router/main.go`
- Create or modify: `router/gateway_engine_test.go`

**Interfaces:**

`Gateway` changes from:

```go
scorer *Scorer
func NewGateway(scorer *Scorer, registry []ModelEntry, upstreams map[string]Upstream) *Gateway
```

to:

```go
engine SystemOneEngine
func NewGateway(engine SystemOneEngine, registry []ModelEntry, upstreams map[string]Upstream) *Gateway
```

- [ ] **Step 1: Write the failing Gateway engine test**

Add a fake engine:

```go
type recordingEngine struct {
    calls int
    resp  System1Response
    err   error
}

func (e *recordingEngine) Evaluate(_ context.Context, _ System1Request) (System1Response, error) {
    e.calls++
    return e.resp, e.err
}
```

Test that `Gateway.route` calls the engine for a new task and uses the returned task type:

```go
func TestGatewayRouteUsesSystemOneEngine(t *testing.T)
```

The response must select one valid `SeedTaskTypes` code, and the test must assert `calls == 1`.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./router -run 'TestGatewayRouteUsesSystemOneEngine' -count=1
```

Expected: compile failure because Gateway still has a `scorer *Scorer` field and the constructor signature is unchanged.

- [ ] **Step 3: Update Gateway minimally**

Change only:

```go
scorer *Scorer
```

to:

```go
engine SystemOneEngine
```

Change the constructor parameter and replace:

```go
if g.scorer == nil {
```

with:

```go
if g.engine == nil {
```

Replace the scorer call with:

```go
resp, err := g.engine.Evaluate(r.Context(), System1Request{
    State: state,
    Questions: map[string]Question{
        "task_type": {Type: "choice", Instructions: "pick the task type", Criteria: criteria},
    },
})
```

Do not change session caching, strong hints, C4 matching, C5 selection, or forwarding.

- [ ] **Step 4: Wrap the native scorer in main**

Change the model-configured path in `cmd/fast-router/main.go`:

```go
scorer := router.NewScorer(backend, "jev-local")
engine := router.NewNativeSystemOneEngine(scorer)
gw = router.NewGateway(engine, registry, cfg.ToUpstreams())
```

Change hint-only construction to:

```go
gw = router.NewGateway(nil, registry, cfg.ToUpstreams())
```

- [ ] **Step 5: Run the focused and existing tests**

Run:

```bash
go test ./router -run 'TestGatewayRouteUsesSystemOneEngine|TestSession|TestNewTaskTurn' -count=1
go test ./router/... ./router/schema/...
go build ./cmd/fast-router
```

Expected: all commands PASS.

## Task 4: Final verification and documentation alignment

**Files:**
- Modify only if needed: `WIKI/implementation-status.md`
- Reference: `docs/System-One-引擎抽象与可替换性.md`

- [ ] **Step 1: Run the complete Go test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Build the main binary**

Run:

```bash
CGO_ENABLED=0 go build ./cmd/fast-router
```

Expected: PASS.

- [ ] **Step 3: Check the diff**

Run:

```bash
git diff --check
git status --short
```

Confirm that only the planned files changed, plus already-existing dirty documentation files that were present before implementation.

- [ ] **Step 4: Record the implementation status**

Update `WIKI/implementation-status.md` only if the existing matrix does not already distinguish:

* `SystemOneEngine` interface: implemented and unit-tested;
* native engine adapter: implemented and unit-tested;
* HTTP engine adapter: implemented and unit-tested;
* remote sidecar integration with a real model: still pending.

- [ ] **Step 5: Commit only the implementation files**

Use a focused local commit:

```bash
git add router/systemone_engine.go \
  router/systemone_engine_test.go \
  router/systemone_http.go \
  router/systemone_http_test.go \
  router/gateway.go \
  router/gateway_engine_test.go \
  cmd/fast-router/main.go
git commit -m "feat: add replaceable System One engine seam"
```

Do not stage or commit unrelated existing changes. Do not push.
