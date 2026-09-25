package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicToOpenAISystemMoves(t *testing.T) {
	in := `{"model":"claude","system":"be brief","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`
	out := anthropicToOpenAI([]byte(in))
	var m map[string]any
	json.Unmarshal(out, &m)
	msgs, _ := m["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages (system+user), got %d", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Fatalf("first msg role: want system, got %v", first["role"])
	}
	if first["content"] != "be brief" {
		t.Fatalf("system content: want 'be brief', got %v", first["content"])
	}
	if _, ok := m["system"]; ok {
		t.Fatal("top-level system should be removed")
	}
}

func TestOpenAIToAnthropicSystemExtracts(t *testing.T) {
	in := `{"model":"gpt","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`
	out := openAIToAnthropic([]byte(in))
	var m map[string]any
	json.Unmarshal(out, &m)
	if m["system"] != "be brief" {
		t.Fatalf("system: want 'be brief', got %v", m["system"])
	}
	msgs, _ := m["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 message (user only), got %d", len(msgs))
	}
}

func TestOpenAIResponseToAnthropic(t *testing.T) {
	in := `{"id":"x","model":"gpt","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"input_tokens":5,"output_tokens":2}}`
	out := openAIResponseToAnthropic([]byte(in))
	var m map[string]any
	json.Unmarshal(out, &m)
	if m["type"] != "message" {
		t.Fatalf("type: want message, got %v", m["type"])
	}
	content, _ := m["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("want 1 content block, got %d", len(content))
	}
	blk, _ := content[0].(map[string]any)
	if blk["text"] != "hello" {
		t.Fatalf("text: want hello, got %v", blk["text"])
	}
	if m["stop_reason"] != "end_turn" {
		t.Fatalf("stop_reason: want end_turn, got %v", m["stop_reason"])
	}
}

func TestAnthropicResponseToOpenAI(t *testing.T) {
	in := `{"id":"x","model":"claude","content":[{"type":"text","text":"world"}],"stop_reason":"end_turn","usage":{"input_tokens":3}}`
	out := anthropicResponseToOpenAI([]byte(in))
	var m map[string]any
	json.Unmarshal(out, &m)
	if m["object"] != "chat.completion" {
		t.Fatalf("object: want chat.completion, got %v", m["object"])
	}
	choices, _ := m["choices"].([]any)
	ch, _ := choices[0].(map[string]any)
	msg, _ := ch["message"].(map[string]any)
	if msg["content"] != "world" {
		t.Fatalf("content: want world, got %v", msg["content"])
	}
	if ch["finish_reason"] != "stop" {
		t.Fatalf("finish: want stop, got %v", ch["finish_reason"])
	}
}

func TestSSEOpenAIChunkToAnthropic(t *testing.T) {
	conv := NewSSEConverter("openai", "anthropic")
	// first chunk: should emit message_start + content_block_start + delta
	chunk := `{"id":"x","object":"chat.completion.chunk","model":"gpt","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`
	lines := conv.ConvertChunk([]byte(chunk))
	if len(lines) < 3 {
		t.Fatalf("first chunk should emit start+block_start+delta, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], "message_start") {
		t.Fatalf("first line should be message_start: %s", lines[0])
	}
	// finish chunk
	finChunk := `{"id":"x","object":"chat.completion.chunk","model":"gpt","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	finLines := conv.ConvertChunk([]byte(finChunk))
	// should emit content_block_stop + message_delta + message_stop
	joined := strings.Join(finLines, "")
	if !strings.Contains(joined, "content_block_stop") || !strings.Contains(joined, "message_stop") {
		t.Fatalf("finish should emit block_stop + message_stop: %s", joined)
	}
}

func TestSSEAnthropicEventToOpenAI(t *testing.T) {
	conv := NewSSEConverter("anthropic", "openai")
	// message_start
	ms := `{"type":"message_start","message":{"id":"x","type":"message","role":"assistant","model":"claude","content":[]}}`
	lines := conv.ConvertChunk([]byte(ms))
	if len(lines) == 0 || !strings.Contains(lines[0], "chat.completion.chunk") {
		t.Fatalf("message_start should emit OpenAI chunk: %v", lines)
	}
	// content_block_delta
	cbd := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	lines = conv.ConvertChunk([]byte(cbd))
	if len(lines) == 0 || !strings.Contains(lines[0], "hello") {
		t.Fatalf("delta should contain 'hello': %v", lines)
	}
	// message_stop
	lines = conv.ConvertChunk([]byte(`{"type":"message_stop"}`))
	if len(lines) == 0 || !strings.Contains(lines[0], "[DONE]") {
		t.Fatalf("message_stop should emit [DONE]: %v", lines)
	}
}

func TestSameProtocolPassthrough(t *testing.T) {
	out, _ := ConvertRequest([]byte(`{"model":"x"}`), "openai", "openai")
	if string(out) != `{"model":"x"}` {
		t.Fatalf("same proto should passthrough: %s", out)
	}
}
