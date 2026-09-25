package router

// ModelEntry: a registered upstream model + its 声明-level capability vector + cost.
// Mirrors litellm Deployment/ModelInfo shape (in production, capability_vector
// would hang off ModelInfo via extra="allow"). POC plain struct.
type ModelEntry struct {
	ModelID          string  // "openai/gpt-5", "anthropic/claude-sonnet-4-5", ...
	Upstream         string  // "openai" / "anthropic" / "deepseek" / ...
	DisplayName      string
	ContextWindow    int
	InputCostPer1k   float64 // USD per 1K input tokens
	OutputCostPer1k  float64
	CapabilityVector  map[string]string // axis -> "high"|"med"|"low"|"none"  (source=声明)
	Measured          map[string]MeasuredEntry // task_code -> measured (cold-start empty, filled by C8)
}

// MeasuredEntry: per (task_code, model) measured pass rate, filled by C8 回填.
type MeasuredEntry struct {
	PassRate   float64
	SampleSize int
	Status     string // "candidate" | "active"
}

// SeedRegistry: 声明-level capability vectors (human-prefilled).
// Costs are illustrative POC values (G-R4: real cost sourcing TODO).
var SeedRegistry = []ModelEntry{
	{"openai/gpt-5", "openai", "GPT-5", 200_000, 5.0, 15.0,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high",
			"vision": "high", "multilingual": "high", "general": "high", "structured_output": "high",
			"creative": "high", "instruction_follow": "high", "math": "high"},
		nil},
	{"openai/gpt-5-mini", "openai", "GPT-5 mini", 128_000, 0.5, 2.0,
		map[string]string{"code": "med", "reasoning": "med", "long_context": "med", "tool_use": "med",
			"vision": "med", "multilingual": "med", "general": "high", "structured_output": "med",
			"creative": "med", "instruction_follow": "high", "math": "med"},
		nil},
	{"anthropic/claude-sonnet-4-5", "anthropic", "Claude Sonnet 4.5", 200_000, 3.0, 15.0,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high",
			"vision": "high", "multilingual": "high", "general": "high", "structured_output": "high",
			"creative": "high", "instruction_follow": "high", "math": "high"},
		nil},
	{"anthropic/claude-haiku-4-5", "anthropic", "Claude Haiku 4.5", 200_000, 1.0, 5.0,
		map[string]string{"code": "med", "reasoning": "med", "long_context": "high", "tool_use": "med",
			"vision": "med", "multilingual": "med", "general": "high", "structured_output": "med",
			"creative": "med", "instruction_follow": "high", "math": "med"},
		nil},
	{"deepseek/deepseek-chat", "deepseek", "DeepSeek V3", 64_000, 0.27, 1.1,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "med", "tool_use": "low",
			"vision": "none", "multilingual": "med", "general": "high", "structured_output": "high",
			"creative": "med", "instruction_follow": "high", "math": "high"},
		nil},
	{"qwen/qwen-max", "openai", "Qwen Max", 128_000, 0.4, 1.2,
		map[string]string{"code": "high", "reasoning": "high", "long_context": "med", "tool_use": "med",
			"vision": "none", "multilingual": "high", "general": "high", "structured_output": "high",
			"creative": "high", "instruction_follow": "high", "math": "high"},
		nil},
}
