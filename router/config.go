// Package router: config — persistent JSON config for the gateway.
//
// Holds listen addr, model paths, and upstream API configs. Loaded from a JSON
// file at startup (--config flag / FR_CONFIG env / default ./fast-router.json),
// editable via the /admin web UI, and saved back on changes.
package router

import (
	"encoding/json"
	"os"
)

// Config is the persistent gateway configuration.
type Config struct {
	Listen     string                 `json:"listen"`      // ":8080"
	MoA        MoAConfig              `json:"moa"`         // C-017: confidence-gated Mixture-of-Agents escalation
	AsyncScore *bool                  `json:"async_score"` // R5: absent → true (async first-score + cache); false restores blocking Jev
	Model      ModelConfig            `json:"model"`       // GGUF model + lib paths
	Upstreams  map[string]UpstreamCfg `json:"upstreams"`   // name -> upstream config
	Registry   []ModelEntry           `json:"registry"`    // model registry (empty → SeedRegistry)
}

// AsyncScoreEnabled reports whether R5 async first-score routing is on.
// Defaults to true when the config field is absent.
func (c *Config) AsyncScoreEnabled() bool { return c.AsyncScore == nil || *c.AsyncScore }

// ModelConfig: paths to the Jev scorer model + zig wrapper + llama.cpp libs.
type ModelConfig struct {
	Path     string `json:"path"`      // GGUF model file (accurate tier)
	FastPath string `json:"fast_path"` // optional small GGUF for the fast tier (first-turn synchronous routing)
	Lib      string `json:"lib"`       // libfrwrapper.{so,dylib,dll}
	LibDir   string `json:"lib_dir"`   // dir with libllama/libggml (for DYLD_LIBRARY_PATH)
}

// UpstreamCfg: a real LLM backend to forward to.
type UpstreamCfg struct {
	BaseURL  string `json:"base_url"` // "https://api.openai.com/v1"
	APIKey   string `json:"api_key"`  // "sk-..."
	Protocol string `json:"protocol"` // "openai" or "anthropic"
}

// DefaultConfig returns a sane starting point (no API keys — user fills via UI).
func DefaultConfig() *Config {
	return &Config{
		Listen: ":8080",
		Model:  ModelConfig{Lib: "zig/libfrwrapper.dylib"},
		Upstreams: map[string]UpstreamCfg{
			"openai":    {BaseURL: "https://api.openai.com/v1", Protocol: "openai"},
			"anthropic": {BaseURL: "https://api.anthropic.com", Protocol: "anthropic"},
			"deepseek":  {BaseURL: "https://api.deepseek.com/v1", Protocol: "openai"},
		},
	}
}

// LoadConfig reads a JSON config file. If the file doesn't exist, returns
// DefaultConfig (and the caller may Save it as a template).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c := DefaultConfig()
			return c, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.Upstreams == nil {
		c.Upstreams = map[string]UpstreamCfg{}
	}
	return &c, nil
}

// Save writes the config to a JSON file (pretty-printed for human editing).
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600) // 0600: contains API keys
}

// ToUpstreams converts config entries to the Gateway's Upstream map.
func (c *Config) ToUpstreams() map[string]Upstream {
	out := make(map[string]Upstream, len(c.Upstreams))
	for name, u := range c.Upstreams {
		out[name] = Upstream{Name: name, BaseURL: u.BaseURL, APIKey: u.APIKey, Protocol: u.Protocol}
	}
	return out
}

// MoAConfig: confidence-gated MoA escalation (C-017). Requests whose scorer
// confidence is below MinConfidence fan out to References (parallel, each
// non-streaming) and the collected answers are synthesized by the Aggregator
// (streamed back through the normal forwarding path). Tool-call requests are
// excluded (aggregating structured tool_calls is unproven); aggregator failure
// falls back to the normally routed model.
type MoAConfig struct {
	Enabled       bool       `json:"enabled"`
	MinConfidence float64    `json:"min_confidence"` // escalate when 0 < conf < this
	References    []MoAModel `json:"references"`
	Aggregator    MoAModel   `json:"aggregator"`
}

// MoAModel names one MoA participant.
type MoAModel struct {
	Upstream    string  `json:"upstream"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}
