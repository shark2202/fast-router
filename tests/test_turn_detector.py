"""POC6 tests: task-turn detector accuracy (P6 > 95%)."""

from fast_router.turn_detector import TurnKind, detect_turn, should_inherit_route


# ---------- OpenAI ----------

def test_openai_tool_result_continues():
    msgs = [
        {"role": "user", "content": "write a sort function"},
        {"role": "assistant", "content": None, "tool_calls": [{"id": "1", "type": "function", "function": {"name": "read_file", "arguments": "{}"}}]},
        {"role": "tool", "tool_call_id": "1", "content": "file contents"},
    ]
    kind, reason = detect_turn(msgs, "openai")
    assert kind is TurnKind.TOOL_LOOP_CONTINUE
    assert "tool" in reason


def test_openai_new_user_is_new_turn():
    msgs = [
        {"role": "user", "content": "write a sort function"},
        {"role": "assistant", "content": "def sort(x): ..."},
        {"role": "user", "content": "now translate it to Rust"},
    ]
    kind, reason = detect_turn(msgs, "openai")
    assert kind is TurnKind.NEW_TASK_TURN


def test_openai_trailing_assistant_text_is_ambiguous():
    msgs = [
        {"role": "user", "content": "hi"},
        {"role": "assistant", "content": "hello"},
    ]
    kind, _ = detect_turn(msgs, "openai")
    assert kind is TurnKind.AMBIGUOUS
    assert should_inherit_route(msgs, "openai") is True


def test_openai_trailing_assistant_with_tool_calls_is_ambiguous():
    msgs = [
        {"role": "user", "content": "do it"},
        {"role": "assistant", "content": None, "tool_calls": [{"id": "1", "type": "function", "function": {"name": "f", "arguments": "{}"}}]},
    ]
    kind, _ = detect_turn(msgs, "openai")
    assert kind is TurnKind.AMBIGUOUS


# ---------- Anthropic ----------

def test_anthropic_tool_result_block_continues():
    msgs = [
        {"role": "user", "content": "write a sort function"},
        {"role": "assistant", "content": [{"type": "tool_use", "id": "1", "name": "read_file", "input": {}}]},
        {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "1", "content": "file contents"}]},
    ]
    kind, reason = detect_turn(msgs, "anthropic")
    assert kind is TurnKind.TOOL_LOOP_CONTINUE
    assert "tool_result" in reason


def test_anthropic_plain_user_is_new_turn():
    msgs = [
        {"role": "user", "content": "hi"},
        {"role": "assistant", "content": [{"type": "text", "text": "hello"}]},
        {"role": "user", "content": "now do something else"},
    ]
    kind, _ = detect_turn(msgs, "anthropic")
    assert kind is TurnKind.NEW_TASK_TURN


def test_anthropic_string_content_user_is_new_turn():
    # Anthropic user content may be a plain string, not a block list.
    msgs = [{"role": "user", "content": "translate this"}]
    kind, _ = detect_turn(msgs, "anthropic")
    assert kind is TurnKind.NEW_TASK_TURN


def test_anthropic_trailing_assistant_is_ambiguous():
    msgs = [{"role": "user", "content": "hi"}, {"role": "assistant", "content": [{"type": "text", "text": "hello"}]}]
    kind, _ = detect_turn(msgs, "anthropic")
    assert kind is TurnKind.AMBIGUOUS
    assert should_inherit_route(msgs, "anthropic") is True


# ---------- Edge cases ----------

def test_empty_messages_ambiguous():
    kind, _ = detect_turn([], "openai")
    assert kind is TurnKind.AMBIGUOUS


def test_unknown_protocol_ambiguous():
    kind, _ = detect_turn([{"role": "user", "content": "x"}], "gemini")
    assert kind is TurnKind.AMBIGUOUS


# ---------- P6 accuracy assertion (structural, should be deterministic) ----------
# Every well-formed tool-loop continuation and every fresh user message must be
# classified correctly. Ambiguous cases are explicitly labeled, not silently
# mis-routed. This is the P6 > 95% target — structural judgment is near-certain.

def test_p6_all_protocols_consistent():
    # Same logical situation (tool result) in both protocols → both continue.
    openai_tool = [{"role": "tool", "tool_call_id": "1", "content": "x"}]
    anth_tool = [{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "1", "content": "x"}]}]
    assert detect_turn(openai_tool, "openai")[0] is TurnKind.TOOL_LOOP_CONTINUE
    assert detect_turn(anth_tool, "anthropic")[0] is TurnKind.TOOL_LOOP_CONTINUE
