// Package router: C1/C6 gateway — HTTP双端点 + 路由链串联 + upstream转发.
//
// /v1/chat/completions (OpenAI) + /v1/messages (Anthropic) 双端点,
// 串联 C2(任务轮判定)→C3(Jev打分)→C4(挡死)→C5(选模)→forward(转发+SSE透传).
//
// POC骨架: 接收raw JSON, 提取messages末user作state, 路由选模, 转发到upstream.
// SSE透传用io.Copy; schema深度转换(OpenAI↔Anthropic)留TODO(后续).
package router

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"fast-router/router/schema"
)

// Upstream: a real LLM backend to forward to.
type Upstream struct {
	Name     string // "openai" / "anthropic" / "deepseek" / ...
	BaseURL  string // "https://api.openai.com/v1"
	APIKey   string
	Protocol string // "openai" or "anthropic" — determines endpoint + auth header
}

// Gateway: HTTP server tying the route chain to upstream forwarding.
type Gateway struct {
	engine    SystemOneEngine
	registry  []ModelEntry
	upstreams map[string]Upstream // by ModelEntry.Upstream
	client    *http.Client
	mu        sync.Mutex // guards upstreams (hot-reload via Admin)

	// session state: cache the last route per session key, so tool-loop
	// continuations (TurnKind != NewTaskTurn) inherit the prior route.
	sessions   map[string]cachedRoute
	sessionsMu sync.Mutex

	// R5 async first-score: on a new task turn, serve the hint fallback
	// immediately and score in the background (single-flight per session key),
	// backfilling the session cache for subsequent turns. Effective P2 ≈ hint
	// path (<1s) instead of the synchronous 77s CPU first score.
	asyncScore bool
	scoring    map[string]struct{} // in-flight background scores, by session key
	scoringMu  sync.Mutex

	// C7 verdict feedback: routing outcomes (nil = not configured, no-op)
	verdicts *VerdictStore

	// retraining loop: scorer evaluations for distillation (nil = not configured)
	trainLog *trainLogger

	// hot reload: closes the PREVIOUS fast-tier backend after a swap
	fastCloser func()

	// two-tier cascade: fast tier = small model, synchronous, gives the first
	// request an informed route; slow tier (engine) refines in the background.
	fastEngine SystemOneEngine
	fastBudget time.Duration
}

// SetVerdicts attaches the C7 verdict store (nil-safe when never called).
func (g *Gateway) SetVerdicts(s *VerdictStore) { g.verdicts = s }

// SetTrainLog attaches the scorer-evaluation logger (data/train_log.jsonl).
func (g *Gateway) SetTrainLog(path string) { g.trainLog = newTrainLogger(path) }

// Verdicts exposes the C7 store for the admin API.
func (g *Gateway) Verdicts() *VerdictStore { return g.verdicts }

// SetFastEngine attaches the fast (small-model) tier for cascade scoring.
func (g *Gateway) SetFastEngine(e SystemOneEngine) {
	g.fastEngine = e
	g.fastBudget = 10 * time.Second
}

// SetFastBudget overrides the synchronous fast-tier time budget.
func (g *Gateway) SetFastBudget(d time.Duration) { g.fastBudget = d }

// SwapFastEngine atomically replaces the fast tier (hot reload). The old
// backend's Close runs after a grace period so in-flight scores (which hold
// the old backend's own mutex) finish first.
func (g *Gateway) SwapFastEngine(e SystemOneEngine, closer func()) {
	g.mu.Lock()
	old := g.fastEngine
	oldCloser := g.fastCloser
	g.fastEngine = e
	g.fastCloser = closer
	g.mu.Unlock()
	if old != nil && oldCloser != nil {
		grace := g.fastBudget
		if grace <= 0 {
			grace = 10 * time.Second
		}
		go func() {
			time.Sleep(2 * grace)
			oldCloser()
			log.Printf("[hot-reload] previous fast tier released")
		}()
	}
	log.Printf("[hot-reload] fast tier swapped")
}

// routeDecision: what route() decided, with the metadata C7 needs.
type routeDecision struct {
	Upstream Upstream
	ModelID  string
	Session  string
	TaskTurn string // "new" | "continue"
	TaskCode string // A-J for jev routes (inherited on continuation)
	Via      string // "jev" | "hint" | "inherit" | "strong-hint"
}

type cachedRoute struct {
	upstream string
	modelID  string
	protocol string
	taskCode string
}

func NewGateway(engine SystemOneEngine, registry []ModelEntry, upstreams map[string]Upstream) *Gateway {
	return &Gateway{engine: engine, registry: registry, upstreams: upstreams, client: &http.Client{}, scoring: make(map[string]struct{})}
}

// SetAsyncScore toggles R5 async first-score routing (sync blocking when false).
func (g *Gateway) SetAsyncScore(enabled bool) { g.asyncScore = enabled }

// SetUpstreams hot-reloads the upstream map (called by Admin on config save).
func (g *Gateway) SetUpstreams(upstreams map[string]Upstream) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.upstreams = upstreams
}

// SetRegistry hot-reloads the model registry (called by Admin on config save).
func (g *Gateway) SetRegistry(registry []ModelEntry) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.registry = registry
}

// ServeHTTP routes by path to the OpenAI or Anthropic handler.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/chat/completions":
		g.handleOpenAI(w, r)
	case "/v1/messages":
		g.handleAnthropic(w, r)
	case "/v1/models":
		g.handleModels(w, r)
	default:
		http.NotFound(w, r)
	}
}

// openAIRequest: minimal fields for routing (messages + model hint).
type openAIRequest struct {
	Messages []map[string]any `json:"messages"`
	Model    string           `json:"model"`
	Stream   bool             `json:"stream"`
}

func (g *Gateway) handleOpenAI(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req openAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), 400)
		return
	}
	dec, err := g.route(r.Context(), req.Messages, req.Model, "openai")
	if err != nil {
		http.Error(w, "route: "+err.Error(), 500)
		return
	}
	// rewrite model field to the chosen upstream model, forward.
	rewritten := g.rewriteOpenAIModel(body, dec.ModelID)
	g.forward(w, r, dec, "/v1/chat/completions", rewritten, "openai")
}

// anthropicRequest: minimal fields for routing.
type anthropicRequest struct {
	Messages []map[string]any `json:"messages"`
	Model    string           `json:"model"`
	Stream   bool             `json:"stream"`
}

func (g *Gateway) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), 400)
		return
	}
	dec, err := g.route(r.Context(), req.Messages, req.Model, "anthropic")
	if err != nil {
		http.Error(w, "route: "+err.Error(), 500)
		return
	}
	rewritten := g.rewriteAnthropicModel(body, dec.ModelID)
	g.forward(w, r, dec, "/v1/messages", rewritten, "anthropic")
}

func (g *Gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"data":[{"id":"fast-router","object":"model"}]}`)
}

// route: C2 task-turn → (new turn?) → C3 Jev → C4 Match → C5 Select.
// Returns the chosen upstream + model_id. If model name is a strong hint
// (e.g. "anthropic/claude-sonnet"), bypasses Jev (design共识 model名强hint).
func (g *Gateway) route(ctx context.Context, messages []map[string]any, modelHint, protocol string) (routeDecision, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// C2: task-turn detection
	kind, _ := DetectTurn(messages, protocol)

	// session key: hash of message history (or X-Session-Id header — TODO)
	sessionKey := sessionKey(messages)
	taskTurn := "new"
	if kind != NewTaskTurn {
		taskTurn = "continue"
	}

	// if not a new task turn (tool-loop or ambiguous), inherit prior route
	if kind != NewTaskTurn {
		g.sessionsMu.Lock()
		if cached, ok := g.sessions[sessionKey]; ok {
			up := g.upstreams[cached.upstream]
			g.sessionsMu.Unlock()
			return routeDecision{Upstream: up, ModelID: cached.modelID, Session: sessionKey,
				TaskTurn: taskTurn, TaskCode: cached.taskCode, Via: "inherit"}, nil
		}
		g.sessionsMu.Unlock()
		// no prior route cached — fall through to route (first call in session)
	}

	// strong hint: model name with upstream prefix bypasses Jev
	if hid, ok := strongHint(modelHint); ok {
		for _, m := range g.registry {
			if m.ModelID == hid {
				if up, ok := g.upstreams[m.Upstream]; ok {
					return routeDecision{Upstream: up, ModelID: m.ModelID, Session: sessionKey,
						TaskTurn: taskTurn, Via: "strong-hint"}, nil
				}
			}
		}
	}

	// R5 async first-score (needs at least one scoring tier): serve an informed
	// route immediately, refine in the background (single-flight per session).
	// Fast tier (small model) scores synchronously within a time budget and
	// seeds the session cache; slow tier backfills/overwrites.
	if g.asyncScore && (g.engine != nil || g.fastEngine != nil) {
		state := lastUserText(messages, protocol)
		if state == "" {
			state = modelHint
		}
		if g.fastEngine != nil {
			budget := g.fastBudget
			if budget <= 0 {
				budget = 10 * time.Second
			}
			fctx, fcancel := context.WithTimeout(ctx, budget)
			upName, modelID, taskCode, fconf, ferr := scoreRoute(fctx, g.fastEngine, state, g.registry, g.upstreams, g.verdicts.Calibrated())
			g.recordTrainSample("fast", sessionKey, state, taskCode, fconf)
			fcancel()
			if ferr == nil {
				if up, ok := g.upstreams[upName]; ok {
					// seed the session cache with the fast route NOW so continuations
					// inherit it instead of re-scoring; the slow tier overwrites later.
					g.cacheRoute(sessionKey, upName, modelID, protocol, taskCode)
					if g.engine != nil {
						g.spawnScore(sessionKey, state, protocol) // slow tier refines
					}
					return routeDecision{Upstream: up, ModelID: modelID, Session: sessionKey,
						TaskTurn: taskTurn, TaskCode: taskCode, Via: "fast"}, nil
				}
			}
			// fast tier failed/timed out — hint fallback + slow spawn below
			if ferr != nil {
				log.Printf("[route-fast] fast tier failed (%v) — falling back to hint", ferr)
			} else {
				log.Printf("[route-fast] fast tier chose unconfigured upstream %q — falling back to hint", upName)
			}
		}
		if g.engine != nil {
			g.spawnScore(sessionKey, state, protocol)
		}
		return routeDecision{Upstream: g.defaultUpstream(), ModelID: modelHint, Session: sessionKey,
			TaskTurn: taskTurn, Via: "hint"}, nil
	}

	// hint-only mode (no scorer configured): skip Jev, use default upstream + client model.
	if g.engine == nil {
		return routeDecision{Upstream: g.defaultUpstream(), ModelID: modelHint, Session: sessionKey,
			TaskTurn: taskTurn, Via: "hint"}, nil
	}

	// extract state from last user message
	state := lastUserText(messages, protocol)
	if state == "" {
		state = modelHint
	}

	upName, modelID, taskCode, sconf, err := scoreRoute(ctx, g.engine, state, g.registry, g.upstreams, g.verdicts.Calibrated())
	g.recordTrainSample("slow", sessionKey, state, taskCode, sconf)
	if err != nil {
		return routeDecision{}, err
	}
	up, ok := g.upstreams[upName]
	if !ok {
		return routeDecision{}, fmt.Errorf("no upstream configured for %s", upName)
	}
	// cache route for this session (tool-loop continuations inherit it)
	g.cacheRoute(sessionKey, upName, modelID, protocol, taskCode)
	return routeDecision{Upstream: up, ModelID: modelID, Session: sessionKey,
		TaskTurn: taskTurn, TaskCode: taskCode, Via: "jev"}, nil
}

// scoreRoute: C3 evaluate + C4 match + C5 select. Reads no Gateway state —
// safe to call from the request path (under g.mu) or a background goroutine
// (with snapshots).
func scoreRoute(ctx context.Context, engine SystemOneEngine, state string, registry []ModelEntry, upstreams map[string]Upstream, measured map[string]map[string]MeasuredEntry) (upstream, modelID, taskCode string, confidence float64, err error) {
	// C3: Jev Choice over task types
	criteria := map[string]string{}
	for _, t := range SeedTaskTypes {
		criteria[t.Code] = t.Description
	}
	resp, err := engine.Evaluate(ctx, System1Request{
		State: state,
		Questions: map[string]Question{
			"task_type": {Type: "choice", Instructions: "pick the task type", Criteria: criteria},
		},
	})
	if err != nil {
		return "", "", "", 0, fmt.Errorf("jev score: %w", err)
	}
	taskCode = resp.Answers["task_type"].Choice
	confidence = resp.Answers["task_type"].Confidence
	log.Printf("[route] jev chose task_type=%s (%s) confidence=%.3f", taskCode, ByCode[taskCode].Name, resp.Answers["task_type"].Confidence)

	// C4 + C5: match + select
	surv := Match(taskCode, registry)
	// keep only models whose upstream is actually configured — otherwise C5
	// cold-start (cheapest-first) picks e.g. deepseek in a single-upstream
	// deployment and the whole decision is discarded downstream.
	configured := surv[:0]
	for _, m := range surv {
		if _, ok := upstreams[m.Upstream]; ok {
			configured = append(configured, m)
		}
	}
	surv = configured
	chosen, err := Select(surv, taskCode, measured)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("select: %w", err)
	}
	log.Printf("[route] C4 blocked %d/%d, C5 selected %s (upstream=%s)", len(registry)-len(surv), len(registry), chosen.ModelID, chosen.Upstream)
	return chosen.Upstream, chosen.ModelID, taskCode, confidence, nil
}

// cacheRoute stores a scored route for the session (tool-loop inheritance).
func (g *Gateway) cacheRoute(sessionKey, upstream, modelID, protocol, taskCode string) {
	g.sessionsMu.Lock()
	if g.sessions == nil {
		g.sessions = make(map[string]cachedRoute)
	}
	g.sessions[sessionKey] = cachedRoute{upstream: upstream, modelID: modelID, protocol: protocol, taskCode: taskCode}
	g.sessionsMu.Unlock()
}

// spawnScore launches a single-flight background score for the session.
// Caller must hold g.mu (route does); snapshots registry/upstreams so the
// goroutine never touches shared mutable state.
func (g *Gateway) spawnScore(sessionKey, state, protocol string) {
	g.scoringMu.Lock()
	if _, busy := g.scoring[sessionKey]; busy {
		g.scoringMu.Unlock()
		return
	}
	g.scoring[sessionKey] = struct{}{}
	g.scoringMu.Unlock()

	engine := g.engine
	registry := make([]ModelEntry, len(g.registry))
	copy(registry, g.registry)
	upstreams := make(map[string]Upstream, len(g.upstreams))
	for k, v := range g.upstreams {
		upstreams[k] = v
	}

	go func() {
		defer func() {
			g.scoringMu.Lock()
			delete(g.scoring, sessionKey)
			g.scoringMu.Unlock()
		}()
		// background ctx: must outlive the request that spawned it (77s+ CPU score)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		upName, modelID, taskCode, aconf, err := scoreRoute(ctx, engine, state, registry, upstreams, g.verdicts.Calibrated())
		g.recordTrainSample("slow", sessionKey, state, taskCode, aconf)
		if err != nil {
			log.Printf("[route-async] background score failed: %v", err)
			return
		}
		if _, ok := upstreams[upName]; !ok {
			log.Printf("[route-async] selected upstream %q not configured", upName)
			return
		}
		g.cacheRoute(sessionKey, upName, modelID, protocol, taskCode)
		log.Printf("[route-async] backfilled session route: %s (upstream=%s)", modelID, upName)
	}()
}

// defaultUpstream returns a deterministic fallback (hint-only / pre-backfill
// window): the lexicographically-first configured upstream — NOT map order,
// which would route the blind window to a random upstream.
func (g *Gateway) defaultUpstream() Upstream {
	best := ""
	for name := range g.upstreams {
		if best == "" || name < best {
			best = name
		}
	}
	return g.upstreams[best]
}

// sessionKey: derive a session key from the message history.
// sessionKey identifies the task thread that owns this conversation: a hash
// of the FIRST user message. It is stable across a growing tool loop (turn N
// appends messages but the first user message stays), so a route scored on
// turn 1 is inherited by every continuation. Task switches are gated by
// DetectTurn: NewTaskTurn re-scores and overwrites the same bucket.
// (Replaces the delivered-v1.0 scheme that hashed the growing message prefix
// — keys never matched across tool-loop turns, so the cache never hit.)
// TODO: also accept X-Session-Id header for explicit session control.
func sessionKey(messages []map[string]any) string {
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "user" {
			content, _ := m["content"].(string)
			sum := fnv.New64a()
			sum.Write([]byte(role))
			sum.Write([]byte{0})
			sum.Write([]byte(content))
			return fmt.Sprintf("s%x", sum.Sum64())
		}
	}
	return "single"
}
func strongHint(model string) (string, bool) {
	for _, sep := range []string{"/", ":"} {
		for i := 0; i < len(model); i++ {
			if string(model[i]) == sep {
				return model, true
			}
		}
	}
	return "", false
}

// lastUserText: extract text from the last user message (the "state" for Jev).
func lastUserText(messages []map[string]any, protocol string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		role, _ := m["role"].(string)
		if role != "user" {
			continue
		}
		switch c := m["content"].(type) {
		case string:
			return c
		case []any:
			for _, b := range c {
				if blk, ok := b.(map[string]any); ok {
					if t, _ := blk["type"].(string); t == "text" {
						if txt, ok := blk["text"].(string); ok {
							return txt
						}
					}
				}
			}
		}
	}
	return ""
}

// rewriteOpenAIModel: patch the "model" field in the request body to the chosen model_id.
func (g *Gateway) rewriteOpenAIModel(body []byte, modelID string) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	m["model"] = modelID
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func (g *Gateway) rewriteAnthropicModel(body []byte, modelID string) []byte {
	return g.rewriteOpenAIModel(body, modelID) // same patch
}

// forward: send to upstream + stream response back, with schema conversion
// (OpenAI↔Anthropic) when client and upstream protocols differ.
func (g *Gateway) forward(w http.ResponseWriter, r *http.Request, dec routeDecision, path string, body []byte, clientProto string) {
	up := dec.Upstream
	upstreamProto := up.Protocol
	// path determined by upstream protocol (not client endpoint).
	// Versioned base URLs (…/v1, …/v4 — e.g. GLM open.bigmodel.cn/api/paas/v4)
	// already carry the version: append only the method path.
	upPath := "/v1/chat/completions"
	if upstreamProto == "anthropic" {
		upPath = "/v1/messages"
	}
	if versionedBaseURL(up.BaseURL) {
		if upstreamProto == "anthropic" {
			upPath = "/messages"
		} else {
			upPath = "/chat/completions"
		}
	}
	// convert request body if protocols differ
	body, _ = schema.ConvertRequest(body, clientProto, upstreamProto)
	url := up.BaseURL + upPath
	url = strings.Replace(url, "/v1/v1/", "/v1/", 1)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build request: "+err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if upstreamProto == "anthropic" {
		req.Header.Set("x-api-key", up.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+up.APIKey)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		g.recordVerdict(dec, 0, "connect_error")
		http.Error(w, "upstream: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	outcome := "ok"
	if resp.StatusCode >= 400 {
		outcome = "upstream_error"
	}
	g.recordVerdict(dec, resp.StatusCode, outcome)

	isStream := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	needConvert := clientProto != upstreamProto

	if isStream && needConvert {
		// SSE: convert each chunk between protocols
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(resp.StatusCode)
		flusher, _ := w.(http.Flusher)
		conv := schema.NewSSEConverter(upstreamProto, clientProto)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if conv.IsDone([]byte(data)) {
				w.Write([]byte("data: [DONE]\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
				break
			}
			for _, out := range conv.ConvertChunk([]byte(data)) {
				w.Write([]byte(out))
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	} else if needConvert {
		// non-stream: convert response body
		respBody, _ := io.ReadAll(resp.Body)
		respBody = schema.ConvertResponse(respBody, upstreamProto, clientProto)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
	} else {
		// same protocol: passthrough headers + body (SSE streams through io.Copy)
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// recordVerdict appends a C7 routing-outcome event (no-op when no store).
func (g *Gateway) recordVerdict(dec routeDecision, status int, outcome string) {
	g.verdicts.Record(VerdictEvent{
		TS:       time.Now().UnixMilli(),
		Session:  dec.Session,
		TaskTurn: dec.TaskTurn,
		TaskCode: dec.TaskCode,
		ModelID:  dec.ModelID,
		Upstream: dec.Upstream.Name,
		Via:      dec.Via,
		Status:   status,
		Outcome:  outcome,
	})
}

// versionedBaseURL reports whether the URL's last path segment is a version
// tag like v1, v2, v4 (case-insensitive, digits only after 'v').
func versionedBaseURL(base string) bool {
	u := strings.TrimRight(base, "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		seg := u[i+1:]
		if len(seg) >= 2 && (seg[0] == 'v' || seg[0] == 'V') {
			for _, c := range seg[1:] {
				if c < '0' || c > '9' {
					return false
				}
			}
			return true
		}
	}
	return false
}

// TrainSample: one scorer evaluation, captured for distillation retraining.
// The slow tier's records are teacher labels (design ② of the self-training
// loop); the fast tier's are the student's predictions for drift monitoring.
type TrainSample struct {
	TS         int64   `json:"ts"`
	Session    string  `json:"session"`
	Tier       string  `json:"tier"`      // "fast" | "slow"
	State      string  `json:"state"`     // the scored user text
	TaskCode   string  `json:"task_code"` // chosen category
	Confidence float64 `json:"confidence"`
}

// trainLogger: append-only JSONL of scorer evaluations (data/train_log.jsonl).
type trainLogger struct {
	mu   sync.Mutex
	path string
}

func newTrainLogger(path string) *trainLogger { return &trainLogger{path: path} }

func (t *trainLogger) log(s TrainSample) {
	if t == nil || t.path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	json.NewEncoder(f).Encode(s)
	f.Close()
}

// recordTrainSample captures a scorer evaluation for the retraining loop.
func (g *Gateway) recordTrainSample(tier, session, state, taskCode string, conf float64) {
	g.trainLog.log(TrainSample{
		TS: time.Now().UnixMilli(), Session: session, Tier: tier,
		State: state, TaskCode: taskCode, Confidence: conf,
	})
}
