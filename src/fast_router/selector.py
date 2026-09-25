"""C5: weighted selector — from C4 survivors, pick the best upstream+model by
measured_matrix + cost.

Cold start (no measured data for this task across candidates): fall back to
cheapest input cost (G-R3 cold-start降级). Once the C8 回填 loop populates
measured pass_rates, selection is weighted:
    score = W_MEASURED * pass_rate + W_COST * cost_score
where cost_score = 1/(1 + 10*input_cost) (cheaper -> higher, bounded in (0,1)).
"""

from .registry import ModelEntry

W_MEASURED = 0.6
W_COST = 0.4


def select(candidates: list[ModelEntry], task_code: str, *, measured: dict | None = None) -> ModelEntry:
    """Pick the best model from C4 survivors.

    Args:
        candidates: C4 survivors (non-empty).
        task_code:  the task type code (A..J) for measured-matrix lookup.
        measured:   {(task_code, model_id): {"pass_rate": float, ...}}. Cold-start None/empty.
    Returns:
        The chosen ModelEntry.
    Raises:
        ValueError if no candidates (should not happen if C4 had any survivor).
    """
    if not candidates:
        raise ValueError("no candidates after blocking — widen registry or relax requirement")
    measured = measured or {}
    # Is there any measured data for THIS task among candidates?
    has_measured = any(_rate(m, task_code, measured) is not None for m in candidates)
    if not has_measured:
        # G-R3 cold start: cheapest input cost (声明-level挡死 already done by C4)
        return min(candidates, key=lambda m: m.input_cost_per_1k)

    def score(m: ModelEntry) -> float:
        rate = _rate(m, task_code, measured) or 0.0
        cost = m.input_cost_per_1k or 1.0
        cost_score = 1.0 / (1.0 + 10.0 * cost)  # cheaper -> higher, in (0, ~1)
        return W_MEASURED * rate + W_COST * cost_score

    return max(candidates, key=score)


def _rate(model: ModelEntry, task_code: str, measured: dict) -> float | None:
    entry = measured.get((task_code, model.model_id))
    return entry.get("pass_rate") if entry else None
