package schema

import (
	"encoding/json"
	"testing"
)

func TestOpenAIToolCallsToAnthropic(t *testing.T) {
	msg := map[string]any{
		"role": "assistant",
		"content": "let me check",
		"tool_calls": []any{
			map[string]any{
				"id":   "call_001",
				"type": "function",
				"function": map[string]any{
					"name":      "get_weather",
					"arguments": `{"city":"SF"}`,
				},
			},
		},
	}
	content := openAIToolCallsToAnthropic(msg)
	if len(content) != 2 {
		t.Fatalf("want 2 blocks (text + tool_use), got %d", len(content))
	}
	// first: text
	if content[0].(map[string]any)["type"] != "text" {
		t.Fatal("first block should be text")
	}
	// second: tool_use
	tu := content[1].(map[string]any)
	if tu["type"] != "tool_use" {
		t.Fatal("second block should be tool_use")
	}
	if tu["name"] != "get_weather" {
		t.Fatalf("name: want get_weather, got %v", tu["name"])
	}
	input := tu["input"].(map[string]any)
	if input["city"] != "SF" {
		t.Fatalf("input.city: want SF, got %v", input["city"])
	}
	if tu["id"] != "call_001" {
		t.Fatalf("id: want call_001, got %v", tu["id"])
	}
}

func TestAnthropicToolUseToOpenAI(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "checking"},
		map[string]any{"type": "tool_use", "id": "tu_01", "name": "search", "input": map[string]any{"q": "hello"}},
	}
	text, toolCalls := anthropicToolUseToOpenAI(content)
	if text != "checking" {
		t.Fatalf("text: want checking, got %s", text)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("want 1 tool_call, got %d", len(toolCalls))
	}
	tc := toolCalls[0].(map[string]any)
	if tc["id"] != "tu_01" {
		t.Fatalf("id: want tu_01, got %v", tc["id"])
	}
	fn := tc["function"].(map[string]any)
	if fn["name"] != "search" {
		t.Fatalf("name: want search, got %v", fn["name"])
	}
	// arguments should be JSON string
	var args map[string]any
	json.Unmarshal([]byte(fn["arguments"].(string)), &args)
	if args["q"] != "hello" {
		t.Fatalf("args.q: want hello, got %v", args["q"])
	}
}

func TestOpenAIToolResultToAnthropic(t *testing.T) {
	msg := map[string]any{
		"role":         "tool",
		"tool_call_id": "call_001",
		"content":      "sunny, 72F",
	}
	result := openAIToolResultToAnthropic(msg)
	if result["role"] != "user" {
		t.Fatal("role should be user (Anthropic puts tool_result in user message)")
	}
	content := result["content"].([]any)
	blk := content[0].(map[string]any)
	if blk["type"] != "tool_result" {
		t.Fatal("type should be tool_result")
	}
	if blk["tool_use_id"] != "call_001" {
		t.Fatalf("tool_use_id: want call_001, got %v", blk["tool_use_id"])
	}
	if blk["content"] != "sunny, 72F" {
		t.Fatalf("content: want 'sunny, 72F', got %v", blk["content"])
	}
}

func TestAnthropicToolResultToOpenAI(t *testing.T) {
	msg := map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "tu_01", "content": "rainy"},
		},
	}
	results := anthropicToolResultToOpenAI(msg)
	if len(results) != 1 {
		t.Fatalf("want 1 tool message, got %d", len(results))
	}
	r := results[0]
	if r["role"] != "tool" {
		t.Fatal("role should be tool")
	}
	if r["tool_call_id"] != "tu_01" {
		t.Fatalf("tool_call_id: want tu_01, got %v", r["tool_call_id"])
	}
	if r["content"] != "rainy" {
		t.Fatalf("content: want rainy, got %v", r["content"])
	}
}

func TestOpenAIToolsToAnthropic(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}
	out := convertOpenAIToolsToAnthropic(tools)
	if len(out) != 1 {
		t.Fatalf("want 1 tool, got %d", len(out))
	}
	tm := out[0].(map[string]any)
	if tm["name"] != "get_weather" {
		t.Fatalf("name: want get_weather, got %v", tm["name"])
	}
	if tm["input_schema"] == nil {
		t.Fatal("input_schema should be set (from parameters)")
	}
}

func TestAnthropicRequestWithToolUseConvertsToOpenAI(t *testing.T) {
	// Anthropic request with tool_use in assistant message
	in := `{"model":"claude","messages":[
		{"role":"user","content":"what's the weather?"},
		{"role":"assistant","content":[{"type":"tool_use","id":"tu_01","name":"get_weather","input":{"city":"SF"}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_01","content":"sunny"}]}
	],"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object"}}]}`
	out := anthropicToOpenAI([]byte(in))
	var m map[string]any
	json.Unmarshal(out, &m)
	msgs := m["messages"].([]any)
	// should have: user, assistant(with tool_calls), tool
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	assistant := msgs[1].(map[string]any)
	tcs := assistant["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("want 1 tool_call, got %d", len(tcs))
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Fatalf("3rd msg role: want tool, got %v", toolMsg["role"])
	}
	// tools should be converted
	tools := m["tools"].([]any)
	tm := tools[0].(map[string]any)
	if tm["type"] != "function" {
		t.Fatal("tool type should be function")
	}
}
