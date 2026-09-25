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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

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
	scorer    *Scorer
	registry  []ModelEntry
	upstreams map[string]Upstream // by ModelEntry.Upstream
	client    *http.Client
	mu        sync.Mutex // guards upstreams (hot-reload via Admin)

	// session state: cache the last route per session key, so tool-loop
	// continuations (TurnKind != NewTaskTurn) inherit the prior route.
	sessions   map[string]cachedRoute
	sessionsMu sync.Mutex
}

type cachedRoute struct {
	upstream  string
	modelID  string
	protocol string
}

func NewGateway(scorer *Scorer, registry []ModelEntry, upstreams map[string]Upstream) *Gateway {
	return &Gateway{scorer: scorer, registry: registry, upstreams: upstreams, client: &http.Client{}}
}

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
	up, modelID, err := g.route(req.Messages, req.Model, "openai")
	if err != nil {
		http.Error(w, "route: "+err.Error(), 500)
		return
	}
	// rewrite model field to the chosen upstream model, forward.
	rewritten := g.rewriteOpenAIModel(body, modelID)
	g.forward(w, r, up, "/v1/chat/completions", rewritten, "openai")
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
	up, modelID, err := g.route(req.Messages, req.Model, "anthropic")
	if err != nil {
		http.Error(w, "route: "+err.Error(), 500)
		return
	}
	rewritten := g.rewriteAnthropicModel(body, modelID)
	g.forward(w, r, up, "/v1/messages", rewritten, "anthropic")
}

func (g *Gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"data":[{"id":"fast-router","object":"model"}]}`)
}

// route: C2 task-turn → (new turn?) → C3 Jev → C4 Match → C5 Select.
// Returns the chosen upstream + model_id. If model name is a strong hint
// (e.g. "anthropic/claude-sonnet"), bypasses Jev (design共识 model名强hint).
func (g *Gateway) route(messages []map[string]any, modelHint, protocol string) (Upstream, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// C2: task-turn detection
	kind, _ := DetectTurn(messages, protocol)

	// session key: hash of message history (or X-Session-Id header — TODO)
	sessionKey := sessionKey(messages)

	// if not a new task turn (tool-loop or ambiguous), inherit prior route
	if kind != NewTaskTurn {
		g.sessionsMu.Lock()
		if cached, ok := g.sessions[sessionKey]; ok {
			up := g.upstreams[cached.upstream]
			g.sessionsMu.Unlock()
			return up, cached.modelID, nil
		}
		g.sessionsMu.Unlock()
		// no prior route cached — fall through to route (first call in session)
	}

	// strong hint: model name with upstream prefix bypasses Jev
	if hid, ok := strongHint(modelHint); ok {
		for _, m := range g.registry {
			if m.ModelID == hid {
				if up, ok := g.upstreams[m.Upstream]; ok {
					return up, m.ModelID, nil
				}
			}
		}
	}

	// hint-only mode (no scorer configured): skip Jev, use default upstream + client model.
	if g.scorer == nil {
		return g.defaultUpstream(), modelHint, nil
	}

	// extract state from last user message
	state := lastUserText(messages, protocol)
	if state == "" {
		state = modelHint
	}

	// C3: Jev Choice over task types
	criteria := map[string]string{}
	for _, t := range SeedTaskTypes {
		criteria[t.Code] = t.Description
	}
	resp, err := g.scorer.Score(System1Request{
		State: state,
		Questions: map[string]Question{
			"task_type": {Type: "choice", Instructions: "pick the task type", Criteria: criteria},
		},
	})
	if err != nil {
		return Upstream{}, "", fmt.Errorf("jev score: %w", err)
	}
	taskCode := resp.Answers["task_type"].Choice
	log.Printf("[route] jev chose task_type=%s (%s) confidence=%.3f", taskCode, ByCode[taskCode].Name, resp.Answers["task_type"].Confidence)

	// C4 + C5: match + select
	surv := Match(taskCode, g.registry)
	chosen, err := Select(surv, taskCode, nil)
	if err != nil {
		return Upstream{}, "", fmt.Errorf("select: %w", err)
	}
	log.Printf("[route] C4 blocked %d/%d, C5 selected %s (upstream=%s)", len(g.registry)-len(surv), len(g.registry), chosen.ModelID, chosen.Upstream)
	up, ok := g.upstreams[chosen.Upstream]
	if !ok {
		return Upstream{}, "", fmt.Errorf("no upstream configured for %s", chosen.Upstream)
	}
	// cache route for this session (tool-loop continuations inherit it)
	g.sessionsMu.Lock()
	if g.sessions == nil {
		g.sessions = make(map[string]cachedRoute)
	}
	g.sessions[sessionKey] = cachedRoute{upstream: chosen.Upstream, modelID: chosen.ModelID, protocol: protocol}
	g.sessionsMu.Unlock()
	return up, chosen.ModelID, nil
}

// defaultUpstream returns the first configured upstream (hint-only fallback).
func (g *Gateway) defaultUpstream() Upstream {
	for _, u := range g.upstreams {
		return u
	}
	return Upstream{}
}

// sessionKey: derive a session key from the message history.
// Messages up to (but not including) the last user message form the session
// identity — tool-loop continuations share the same key.
// TODO: also accept X-Session-Id header for explicit session control.
func sessionKey(messages []map[string]any) string {
	if len(messages) <= 1 {
		return "single"
	}
	// hash all messages except the last (the last changes per turn)
	h := uint64(0)
	for i := 0; i < len(messages)-1; i++ {
		m := messages[i]
		role, _ := m["role"].(string)
		h = h*31 + uint64(len(role))
		// include content hash (simplified: length + first chars)
		if content, ok := m["content"].(string); ok {
			h = h*31 + uint64(len(content))
			if len(content) > 0 {
				h = h*31 + uint64(content[0])
			}
		}
	}
	return fmt.Sprintf("s%x", h)
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
func (g *Gateway) forward(w http.ResponseWriter, r *http.Request, up Upstream, path string, body []byte, clientProto string) {
	upstreamProto := up.Protocol
	// path determined by upstream protocol (not client endpoint)
	upPath := "/v1/chat/completions"
	if upstreamProto == "anthropic" {
		upPath = "/v1/messages"
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
		http.Error(w, "upstream: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()

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
				if flusher != nil { flusher.Flush() }
				break
			}
			for _, out := range conv.ConvertChunk([]byte(data)) {
				w.Write([]byte(out))
			}
			if flusher != nil { flusher.Flush() }
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
