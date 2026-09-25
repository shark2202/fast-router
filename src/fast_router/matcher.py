"""C4: capability matcher — block-disqualify models whose declared capability
does not meet the task type's requirement.

task_code -> capability_requirement (from task_types) -> filter registry.
Survivors go to the C5 selector. This is the 声明-level挡死 layer (design
consensus Q4): a model that declares code:none is blocked for code_generation
regardless of cost or measured performance — prevents Type III routing.
"""

from .task_types import BY_CODE, meets_requirement, _LEVEL_ORDER
from .registry import ModelEntry


def match(task_code: str, registry: list[ModelEntry]) -> list[ModelEntry]:
    """Block-disqualify models whose declared capability_vector does not meet
    the task type's capability_requirement. Returns surviving candidates."""
    tt = BY_CODE[task_code]
    req = tt.capability_requirement
    return [m for m in registry if meets_requirement(m.capability_vector, req)]


def blocked_reasons(task_code: str, registry: list[ModelEntry]) -> dict:
    """For logging/debugging: which requirement axes each blocked model failed."""
    tt = BY_CODE[task_code]
    req = tt.capability_requirement
    reasons: dict[str, list[str]] = {}
    for m in registry:
        failed = []
        for axis, required_level in req.items():
            declared = m.capability_vector.get(axis)
            if declared is None:
                failed.append(f"{axis}:missing")
            elif _LEVEL_ORDER.get(str(declared), -1) < _LEVEL_ORDER.get(required_level, 0):
                failed.append(f"{axis}:{declared}<{required_level}")
        if failed:
            reasons[m.model_id] = failed
    return reasons
