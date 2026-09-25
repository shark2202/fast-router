// Package router: C1/C6 gateway — HTTP双端点 + 路由链串联 + upstream转发.
//
// /v1/chat/completions (OpenAI) + /v1/messages (Anthropic) 双端点,
// 串联 C2(任务轮判定)→C3(Jev打分)→C4(挡死)→C5(选模)→forward(转发+SSE透传).
//
// POC骨架: 接收raw JSON, 提取messages末user作state, 路由选模, 转发到upstream.
// SSE透传用io.Copy; schema深度转换(OpenAI↔Anthropic)留TODO(后续).
package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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
	if kind != NewTaskTurn {
		// tool-loop continuation or ambiguous — would inherit prior route.
		// POC: no session state yet, fall through to re-route (TODO: session lock).
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

	// C4 + C5: match + select
	surv := Match(taskCode, g.registry)
	chosen, err := Select(surv, taskCode, nil)
	if err != nil {
		return Upstream{}, "", fmt.Errorf("select: %w", err)
	}
	up, ok := g.upstreams[chosen.Upstream]
	if !ok {
		return Upstream{}, "", fmt.Errorf("no upstream configured for %s", chosen.Upstream)
	}
	return up, chosen.ModelID, nil
}

// defaultUpstream returns the first configured upstream (hint-only fallback).
func (g *Gateway) defaultUpstream() Upstream {
	for _, u := range g.upstreams {
		return u
	}
	return Upstream{}
}

// strongHint: "anthropic/claude-sonnet-4-5" -> model_id, bypassing Jev.
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

// forward: send to upstream + stream response back (SSE passthrough via io.Copy).
func (g *Gateway) forward(w http.ResponseWriter, r *http.Request, up Upstream, path string, body []byte, protocol string) {
	url := up.BaseURL + path
	// dedupe /v1/v1 (OpenAI base_url already includes /v1)
	url = strings.Replace(url, "/v1/v1/", "/v1/", 1)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build request: "+err.Error(), 500)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if protocol == "anthropic" {
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
	// passthrough headers + body (SSE chunks stream through io.Copy)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
