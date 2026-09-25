"""POC6: task-turn detector (C2 closure).

Decides, for an incoming chat request's message list, whether the last message is:
  - a tool_result continuation (tool loop in progress → inherit current route)
  - a new user message (new task turn → trigger Jev re-scoring)
  - ambiguous (e.g. trailing assistant → fallback: inherit last route)

This is pure structural judgment on the message list — no model, no network.
Design: docs/fast-router-智能路由设计共识-v0.1.md Q7 (task-turn routing).
"""

from enum import Enum
from typing import Any


class TurnKind(Enum):
    NEW_TASK_TURN = "new_task_turn"          # new user message → trigger Jev
    TOOL_LOOP_CONTINUE = "tool_loop_continue"  # tool_result → inherit route
    AMBIGUOUS = "ambiguous"                    # fallback → inherit last route


def detect_turn(messages: list[dict[str, Any]], protocol: str = "openai") -> tuple[TurnKind, str]:
    """Classify the task turn from the last message.

    Args:
        messages: chat request messages list (OpenAI or Anthropic shape).
        protocol: "openai" (messages have role/content/tool_calls/tool_call_id)
                  or "anthropic" (user content may be a list of typed blocks,
                  tool_result appears as a content block, not a separate role).

    Returns:
        (TurnKind, reason) — reason is a short human-readable justification.
    """
    if not messages:
        return TurnKind.AMBIGUOUS, "empty messages"
    last = messages[-1]
    if protocol == "openai":
        return _detect_openai(last)
    if protocol == "anthropic":
        return _detect_anthropic(last)
    return TurnKind.AMBIGUOUS, f"unknown protocol: {protocol}"


def _detect_openai(last: dict[str, Any]) -> tuple[TurnKind, str]:
    role = last.get("role")
    if role == "tool":
        # OpenAI tool-call loop: assistant emits tool_calls, then a role="tool"
        # message carries the tool result back. A trailing tool message means
        # the model must process the result → same task, inherit route.
        return TurnKind.TOOL_LOOP_CONTINUE, "trailing role=tool (tool result) → continue"
    if role == "user":
        # A plain user message is a potential new task turn. (OpenAI user
        # messages do not carry tool_result; that is a separate role="tool".)
        return TurnKind.NEW_TASK_TURN, "trailing role=user (new user message) → new turn"
    if role == "assistant":
        if last.get("tool_calls"):
            # Trailing assistant with tool_calls: stream interrupted mid-tool-call,
            # or the assistant just requested tools. Treat as ambiguous (inherit)
            # rather than firing a new task turn.
            return TurnKind.AMBIGUOUS, "trailing assistant with tool_calls (interrupted?) → inherit"
        return TurnKind.AMBIGUOUS, "trailing assistant text (task may have ended) → inherit"
    return TurnKind.AMBIGUOUS, f"unknown role: {role}"


def _detect_anthropic(last: dict[str, Any]) -> tuple[TurnKind, str]:
    role = last.get("role")
    if role == "user":
        content = last.get("content")
        # Anthropic user content can be a plain string or a list of typed blocks.
        # A tool_result block inside user content = tool-loop continuation.
        if isinstance(content, list):
            for block in content:
                if isinstance(block, dict) and block.get("type") == "tool_result":
                    return TurnKind.TOOL_LOOP_CONTINUE, "trailing user with tool_result block → continue"
        return TurnKind.NEW_TASK_TURN, "trailing user with plain text → new turn"
    if role == "assistant":
        return TurnKind.AMBIGUOUS, "trailing assistant → inherit"
    return TurnKind.AMBIGUOUS, f"unknown role: {role}"


def should_inherit_route(messages: list[dict[str, Any]], protocol: str = "openai") -> bool:
    """Convenience: True if the current turn should inherit the prior route
    (tool loop or ambiguous), False if it is a fresh task turn needing Jev."""
    kind, _ = detect_turn(messages, protocol)
    return kind != TurnKind.NEW_TASK_TURN
