"""Model registry (C4/C5 supporting): upstream + model + capability_vector + cost.

POC lightweight — mirrors litellm Deployment/ModelInfo shape (in production,
capability_vector would hang off ModelInfo via extra="allow"). Here a plain
dataclass. Seed entries are 声明-level (human-prefilled, source=声明); the
measured_matrix (task_type × model pass_rate) is cold-start empty, filled by
the C8 回填 loop over time.
"""

from dataclasses import dataclass, field


@dataclass
class ModelEntry:
    model_id: str                          # "openai/gpt-5", "anthropic/claude-sonnet-4-5", ...
    upstream: str                          # "openai" / "anthropic" / "deepseek" / ...
    display_name: str
    context_window: int
    input_cost_per_1k: float               # USD per 1K input tokens
    output_cost_per_1k: float             # USD per 1K output tokens
    capability_vector: dict                # axis -> "high"/"med"/"low"/"none"  (source=声明)
    measured: dict = field(default_factory=dict)
    # measured: {(task_code): {"pass_rate": float, "sample_size": int, "status": str}}
    # cold-start empty; filled by C8. status: candidate | active.


# Seed registry — 声明-level capability vectors (human-prefilled).
# Costs are illustrative POC values (G-R4: real cost sourcing TODO).
SEED_REGISTRY: list[ModelEntry] = [
    ModelEntry("openai/gpt-5", "openai", "GPT-5", 200_000, 5.0, 15.0,
        {"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high",
         "vision": "high", "multilingual": "high", "general": "high",
         "structured_output": "high", "creative": "high", "instruction_follow": "high", "math": "high"}),
    ModelEntry("openai/gpt-5-mini", "openai", "GPT-5 mini", 128_000, 0.5, 2.0,
        {"code": "med", "reasoning": "med", "long_context": "med", "tool_use": "med",
         "vision": "med", "multilingual": "med", "general": "high",
         "structured_output": "med", "creative": "med", "instruction_follow": "high", "math": "med"}),
    ModelEntry("anthropic/claude-sonnet-4-5", "anthropic", "Claude Sonnet 4.5", 200_000, 3.0, 15.0,
        {"code": "high", "reasoning": "high", "long_context": "high", "tool_use": "high",
         "vision": "high", "multilingual": "high", "general": "high",
         "structured_output": "high", "creative": "high", "instruction_follow": "high", "math": "high"}),
    ModelEntry("anthropic/claude-haiku-4-5", "anthropic", "Claude Haiku 4.5", 200_000, 1.0, 5.0,
        {"code": "med", "reasoning": "med", "long_context": "high", "tool_use": "med",
         "vision": "med", "multilingual": "med", "general": "high",
         "structured_output": "med", "creative": "med", "instruction_follow": "high", "math": "med"}),
    ModelEntry("deepseek/deepseek-chat", "deepseek", "DeepSeek V3", 64_000, 0.27, 1.1,
        {"code": "high", "reasoning": "high", "long_context": "med", "tool_use": "low",
         "vision": "none", "multilingual": "med", "general": "high",
         "structured_output": "high", "creative": "med", "instruction_follow": "high", "math": "high"}),
    ModelEntry("qwen/qwen-max", "openai", "Qwen Max", 128_000, 0.4, 1.2,
        {"code": "high", "reasoning": "high", "long_context": "med", "tool_use": "med",
         "vision": "none", "multilingual": "high", "general": "high",
         "structured_output": "high", "creative": "high", "instruction_follow": "high", "math": "high"}),
]
