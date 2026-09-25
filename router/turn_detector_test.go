package router

import "testing"

func TestOpenaiToolResultContinues(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "write a sort function"},
		{"role": "assistant", "content": nil, "tool_calls": []any{"x"}},
		{"role": "tool", "tool_call_id": "1", "content": "file contents"},
	}
	kind, reason := DetectTurn(msgs, "openai")
	if kind != ToolLoopContinue {
		t.Fatalf("want ToolLoopContinue, got %v (%s)", kind, reason)
	}
}

func TestOpenaiNewUserIsNewTurn(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "write a sort function"},
		{"role": "assistant", "content": "def sort(x): ..."},
		{"role": "user", "content": "now translate it to Rust"},
	}
	kind, _ := DetectTurn(msgs, "openai")
	if kind != NewTaskTurn {
		t.Fatalf("want NewTaskTurn, got %v", kind)
	}
}

func TestOpenaiTrailingAssistantTextIsAmbiguous(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello"},
	}
	kind, _ := DetectTurn(msgs, "openai")
	if kind != Ambiguous || !ShouldInheritRoute(msgs, "openai") {
		t.Fatalf("want Ambiguous+inherit, got %v", kind)
	}
}

func TestOpenaiTrailingAssistantWithToolCallsIsAmbiguous(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "do it"},
		{"role": "assistant", "content": nil, "tool_calls": []any{"x"}},
	}
	kind, _ := DetectTurn(msgs, "openai")
	if kind != Ambiguous {
		t.Fatalf("want Ambiguous, got %v", kind)
	}
}

func TestAnthropicToolResultBlockContinues(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "write a sort function"},
		{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "1"}}},
		{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "1", "content": "x"}}},
	}
	kind, reason := DetectTurn(msgs, "anthropic")
	if kind != ToolLoopContinue {
		t.Fatalf("want ToolLoopContinue, got %v (%s)", kind, reason)
	}
}

func TestAnthropicPlainUserIsNewTurn(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "hello"}}},
		{"role": "user", "content": "now do something else"},
	}
	kind, _ := DetectTurn(msgs, "anthropic")
	if kind != NewTaskTurn {
		t.Fatalf("want NewTaskTurn, got %v", kind)
	}
}

func TestAnthropicStringContentUserIsNewTurn(t *testing.T) {
	msgs := []map[string]any{{"role": "user", "content": "translate this"}}
	kind, _ := DetectTurn(msgs, "anthropic")
	if kind != NewTaskTurn {
		t.Fatalf("want NewTaskTurn, got %v", kind)
	}
}

func TestAnthropicTrailingAssistantIsAmbiguous(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "hello"}}},
	}
	kind, _ := DetectTurn(msgs, "anthropic")
	if kind != Ambiguous || !ShouldInheritRoute(msgs, "anthropic") {
		t.Fatalf("want Ambiguous+inherit, got %v", kind)
	}
}

func TestEmptyMessagesAmbiguous(t *testing.T) {
	kind, _ := DetectTurn(nil, "openai")
	if kind != Ambiguous {
		t.Fatalf("want Ambiguous, got %v", kind)
	}
}

func TestUnknownProtocolAmbiguous(t *testing.T) {
	kind, _ := DetectTurn([]map[string]any{{"role": "user", "content": "x"}}, "gemini")
	if kind != Ambiguous {
		t.Fatalf("want Ambiguous, got %v", kind)
	}
}

func TestP6AllProtocolsConsistent(t *testing.T) {
	// Same logical situation (tool result) in both protocols → both continue.
	openaiTool := []map[string]any{{"role": "tool", "tool_call_id": "1", "content": "x"}}
	anthTool := []map[string]any{
		{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "1", "content": "x"}}},
	}
	if k, _ := DetectTurn(openaiTool, "openai"); k != ToolLoopContinue {
		t.Fatalf("openai tool not continue: %v", k)
	}
	if k, _ := DetectTurn(anthTool, "anthropic"); k != ToolLoopContinue {
		t.Fatalf("anthropic tool not continue: %v", k)
	}
}
