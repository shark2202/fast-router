"""C4 (matcher) + C5 (selector) tests — pure logic, zero model dependency."""

from fast_router.registry import SEED_REGISTRY, ModelEntry
from fast_router.matcher import match, blocked_reasons
from fast_router.selector import select
from fast_router.task_types import BY_CODE


# ---------- C4: capability挡死 ----------

def test_c4_code_generation_survives_high_code_models():
    surv = match("A", SEED_REGISTRY)
    ids = [m.model_id for m in surv]
    # req: code:high, reasoning:med, tool_use:med
    # gpt-5/sonnet (code:high, tool_use:high) survive; qwen (code:high, tool_use:med) survive
    # deepseek tool_use:low -> blocked; gpt5-mini/haiku code:med -> blocked
    assert "openai/gpt-5" in ids
    assert "anthropic/claude-sonnet-4-5" in ids
    assert "qwen/qwen-max" in ids
    assert "deepseek/deepseek-chat" not in ids  # tool_use:low < med (correctly blocked)
    assert "openai/gpt-5-mini" not in ids       # code:med < high
    assert "anthropic/claude-haiku-4-5" not in ids


def test_c4_blocks_code_none_model():
    weak = ModelEntry("weak/code-none", "weak", "Weak", 8000, 0.1, 0.2,
                      {"code": "none", "reasoning": "low"})
    surv = match("A", [weak] + SEED_REGISTRY)
    assert weak not in surv


def test_c4_simple_qa_passes_all_with_general():
    # simple_qa requires general:med; all seed models have general:high
    surv = match("I", SEED_REGISTRY)
    assert len(surv) == len(SEED_REGISTRY)


def test_c4_multimodal_blocks_non_vision():
    # multimodal requires vision:high; deepseek/qwen have vision:none -> blocked
    surv = match("G", SEED_REGISTRY)
    ids = [m.model_id for m in surv]
    assert "openai/gpt-5" in ids
    assert "anthropic/claude-sonnet-4-5" in ids
    assert "deepseek/deepseek-chat" not in ids
    assert "qwen/qwen-max" not in ids


def test_c4_blocked_reasons_reports_axes():
    reasons = blocked_reasons("G", SEED_REGISTRY)
    assert "deepseek/deepseek-chat" in reasons
    assert any("vision" in r for r in reasons["deepseek/deepseek-chat"])


# ---------- C5: weighted selection ----------

def test_c5_cold_start_picks_cheapest():
    # no measured data -> cheapest input cost (deepseek 0.27)
    surv = match("I", SEED_REGISTRY)
    chosen = select(surv, "I")
    assert chosen.model_id == "deepseek/deepseek-chat"


def test_c5_with_measured_picks_best_score():
    surv = match("I", SEED_REGISTRY)
    measured = {
        ("I", "deepseek/deepseek-chat"): {"pass_rate": 0.5, "sample_size": 10, "status": "candidate"},
        ("I", "qwen/qwen-max"): {"pass_rate": 0.9, "sample_size": 10, "status": "candidate"},
    }
    chosen = select(surv, "I", measured=measured)
    # qwen: 0.6*0.9 + 0.4*(1/(1+4)) = 0.54+0.08 = 0.62
    # deepseek: 0.6*0.5 + 0.4*(1/(1+2.7)) = 0.3+0.108 = 0.408
    assert chosen.model_id == "qwen/qwen-max"


def test_c5_low_rate_cheap_cost_can_win_over_high_rate_expensive():
    # measured favors a cheap model with decent rate over an expensive one with high rate
    surv = match("I", SEED_REGISTRY)
    measured = {
        ("I", "deepseek/deepseek-chat"): {"pass_rate": 0.7, "sample_size": 10, "status": "active"},
        ("I", "openai/gpt-5"): {"pass_rate": 0.95, "sample_size": 10, "status": "active"},
    }
    chosen = select(surv, "I", measured=measured)
    # deepseek: 0.6*0.7 + 0.4*(1/3.7) = 0.42+0.108 = 0.528
    # gpt-5: 0.6*0.95 + 0.4*(1/51) = 0.57+0.0078 = 0.578  -> gpt-5 wins (rate dominates)
    assert chosen.model_id == "openai/gpt-5"


def test_c5_no_candidates_raises():
    try:
        select([], "I")
        assert False, "should have raised"
    except ValueError:
        pass


# ---------- end-to-end: Jev code -> C4 -> C5 ----------

def test_route_chain_all_task_types_produce_a_model():
    """For every seed task type, C4挡死 + C5 select must yield a model."""
    for code in "ABCDEFGHIJ":
        surv = match(code, SEED_REGISTRY)
        chosen = select(surv, code)
        assert chosen is not None, f"no model for task {code} ({BY_CODE[code].name})"


def test_route_chain_code_generation_picks_high_code_model():
    surv = match("A", SEED_REGISTRY)
    chosen = select(surv, "A")  # cold start -> cheapest among survivors
    # survivors: gpt-5(5.0), sonnet(3.0), qwen(0.4); deepseek blocked (tool_use:low)
    # cold start cheapest = qwen
    assert chosen.model_id == "qwen/qwen-max"
