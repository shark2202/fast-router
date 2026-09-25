"""10 seed task types + capability requirement vectors (design consensus Q5).

Each task type maps to a single-token code (A..J) for Jev scoring, and carries a
capability requirement vector used by the matcher (C4) to block-disqualify models
whose declared capability vector does not meet the requirement.

Source: docs/fast-router-智能路由设计共识-v0.1.md Q5 seed table.
"""

from dataclasses import dataclass, field


@dataclass(frozen=True)
class TaskType:
    code: str                       # single-token label for Jev (A..J)
    name: str                       # stable identifier
    label: str                      # human-readable
    description: str                # what counts as this task
    capability_requirement: dict[str, str] = field(default_factory=dict)
    # capability axis -> required level: "high" | "med" | "low"
    # matcher blocks models whose declared capability_vector does not meet these.


# 10 seed task types. Codes A..J are single ASCII tokens (Qwen tokenizer encodes
# each as exactly one token — verified by candidate_codes at runtime).
SEED_TASK_TYPES: tuple[TaskType, ...] = (
    TaskType("A", "code_generation", "代码生成",
             "生成或修改源代码、实现功能、写函数/类/脚本。",
             {"code": "high", "reasoning": "med", "tool_use": "med"}),
    TaskType("B", "code_review_debug", "代码审查/调试",
             "审查代码、定位 bug、解释代码行为、提出修复。",
             {"code": "high", "reasoning": "high", "long_context": "med"}),
    TaskType("C", "reasoning_analysis", "推理/分析",
             "多步推理、逻辑分析、因果推断、决策推演（非代码非数学专属）。",
             {"reasoning": "high", "math": "med"}),
    TaskType("D", "long_document_processing", "长文档处理",
             "总结/抽取/问答长文档（超 8K token），依赖长上下文。",
             {"long_context": "high", "reasoning": "med"}),
    TaskType("E", "structured_extraction", "结构化抽取",
             "从文本抽取结构化字段、生成 JSON/表格、严格遵循 schema。",
             {"structured_output": "high", "instruction_follow": "high"}),
    TaskType("F", "creative_writing", "创意写作",
             "写文案、故事、邮件、营销文本等创意性内容。",
             {"creative": "high", "multilingual": "med"}),
    TaskType("G", "multimodal_understanding", "多模态理解",
             "理解图片/截图/图表内容（vision 输入）。",
             {"vision": "high", "reasoning": "med"}),
    TaskType("H", "tool_agent_task", "工具调用 agent 任务",
             "需要调用外部工具/函数/API 完成的多步 agent 任务。",
             {"tool_use": "high", "reasoning": "high", "code": "med"}),
    TaskType("I", "simple_qa", "简单问答",
             "事实性问答、短回答、闲聊，无需深度推理或长上下文。",
             {"general": "med"}),
    TaskType("J", "multilingual_translation", "多语言翻译",
             "翻译文本到另一语言。",
             {"multilingual": "high"}),
)

# code -> TaskType, for O(1) lookup after Jev scoring picks a code.
BY_CODE: dict[str, TaskType] = {t.code: t for t in SEED_TASK_TYPES}

# Cap level ordering for matcher: high > med > low. A model meets a requirement
# if its declared level >= required level.
_LEVEL_ORDER = {"low": 0, "med": 1, "high": 2}


def meets_requirement(declared_capability: dict[str, str | float], requirement: dict[str, str]) -> bool:
    """True if declared capability meets every axis in the requirement.

    declared_capability: model's capability_vector (axis -> level str or numeric).
    requirement: task type's capability_requirement (axis -> required level str).
    Missing axis in declared = does not meet (blocked).
    """
    for axis, required_level in requirement.items():
        declared = declared_capability.get(axis)
        if declared is None:
            return False
        decl_lvl = _LEVEL_ORDER.get(str(declared), -1)
        req_lvl = _LEVEL_ORDER.get(required_level, 0)
        if decl_lvl < req_lvl:
            return False
    return True


def candidate_descriptions() -> str:
    """Build the candidate table text for the Jev prompt (code: description lines)."""
    return "\n".join(f"{t.code}: {t.name} - {t.description}" for t in SEED_TASK_TYPES)


# MAIN ACTION cues per code — explicit discrimination signals for small models.
# Added after POC1 v0 (0.5B P1=13.3%) showed 0.5B mis-routing broadly to B
# (whose description contained the over-broad verb "解释"). Each cue names the
# single discriminating action, so a weak model matches by verb rather than
# by guessing among overlapping descriptions.
_MAIN_ACTION_CUES = {
    "A": "produce new code (write/implement/generate a function/script/program)",
    "B": "inspect EXISTING code for problems (review/find bug/debug)",
    "C": "reason / analyze / compare / weigh (not code, not a given document)",
    "D": "process a GIVEN long document or report (summarize/extract/QA it)",
    "E": "output structured data (JSON / CSV / table / fields)",
    "F": "write creative text (prose / story / copy / email)",
    "G": "understand an image / screenshot / chart (vision input)",
    "H": "call external tools / calendar / API / deploy (agent task)",
    "I": "answer a simple factual question (short lookup)",
    "J": "translate between languages",
}


def prompt_descriptions() -> str:
    """Candidate table with MAIN ACTION cues — richer than candidate_descriptions().

    Used by the Jev prompt to help small models discriminate. Large models may
    not need these cues, but they do not hurt (the description stays intact).
    """
    lines = []
    for t in SEED_TASK_TYPES:
        cue = _MAIN_ACTION_CUES.get(t.code, "")
        lines.append(f"{t.code}: {t.name} - {t.description} (MAIN ACTION: {cue})")
    return "\n".join(lines)
