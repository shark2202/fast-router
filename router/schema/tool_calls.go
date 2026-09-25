package schema

import "encoding/json"

// --- Tool call conversion: OpenAI tool_calls ↔ Anthropic tool_use ---
//
// OpenAI:
//   assistant message: {role:"assistant", tool_calls:[{id, type:"function", function:{name, arguments}}]}
//   tool result:       {role:"tool", tool_call_id, content}
//   request tools:     [{type:"function", function:{name, description, parameters}}]
//
// Anthropic:
//   assistant content: [{type:"tool_use", id, name, input}]
//   tool result:       user content: [{type:"tool_result", tool_use_id, content, is_error}]
//   request tools:     [{name, description, input_schema}]

// convertOpenAIToolsToAnthropic: request "tools" array (OpenAI → Anthropic).
func convertOpenAIToolsToAnthropic(tools []any) []any {
	var out []any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tm["function"].(map[string]any)
		if fn == nil {
			continue
		}
		out = append(out, map[string]any{
			"name":         fn["name"],
			"description":  fn["description"],
			"input_schema": fn["parameters"],
		})
	}
	return out
}

func convertAnthropicToolsToOpenAI(tools []any) []any {
	var out []any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tm["name"],
				"description": tm["description"],
				"parameters":  tm["input_schema"],
			},
		})
	}
	return out
}

// convertOpenAIMessagesToolCalls: transform tool_calls in assistant messages
// and tool-result messages between OpenAI and Anthropic formats.
// Called by anthropicToOpenAI / openAIToAnthropic for each message.

// openAIToolCallsToAnthropic: convert OpenAI assistant message with tool_calls
// to Anthropic content blocks.
func openAIToolCallsToAnthropic(msg map[string]any) []any {
	var content []any
	// text content (if any)
	if text, ok := msg["content"].(string); ok && text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	// tool_calls → tool_use blocks
	toolCalls, _ := msg["tool_calls"].([]any)
	for _, tc := range toolCalls {
		tcm, ok := tc.(map[string]any)
		if !ok {
			continue
		}
		fn, _ := tcm["function"].(map[string]any)
		if fn == nil {
			continue
		}
		name, _ := fn["name"].(string)
		argsStr, _ := fn["arguments"].(string)
		var input map[string]any
		json.Unmarshal([]byte(argsStr), &input) // OpenAI arguments is JSON string
		id, _ := tcm["id"].(string)
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": input,
		})
	}
	return content
}

// anthropicToolUseToOpenAI: convert Anthropic content blocks with tool_use
// to OpenAI assistant message with tool_calls.
func anthropicToolUseToOpenAI(content []any) (text string, toolCalls []any) {
	for _, blk := range content {
		b, ok := blk.(map[string]any)
		if !ok {
			continue
		}
		switch b["type"] {
		case "text":
			if t, ok := b["text"].(string); ok {
				text += t
			}
		case "tool_use":
			id, _ := b["id"].(string)
			name, _ := b["name"].(string)
			input := b["input"]
			argsBytes, _ := json.Marshal(input)
			toolCalls = append(toolCalls, map[string]any{
				"id":   id,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": string(argsBytes), // OpenAI arguments is JSON string
				},
			})
		}
	}
	return
}

// openAIToolResultToAnthropic: convert OpenAI {role:"tool"} message to
// Anthropic user message with tool_result content block.
func openAIToolResultToAnthropic(msg map[string]any) map[string]any {
	toolCallID, _ := msg["tool_call_id"].(string)
	content, _ := msg["content"].(string)
	return map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{
				"type":        "tool_result",
				"tool_use_id": toolCallID,
				"content":     content,
			},
		},
	}
}

// anthropicToolResultToOpenAI: convert Anthropic user message with tool_result
// block to OpenAI {role:"tool"} message.
func anthropicToolResultToOpenAI(msg map[string]any) []map[string]any {
	content, _ := msg["content"].([]any)
	var out []map[string]any
	for _, blk := range content {
		b, ok := blk.(map[string]any)
		if !ok {
			continue
		}
		if b["type"] != "tool_result" {
			continue
		}
		toolUseID, _ := b["tool_use_id"].(string)
		// tool_result content can be string or array; flatten to string for OpenAI
		var resultText string
		switch c := b["content"].(type) {
		case string:
			resultText = c
		case []any:
			for _, rb := range c {
				if rm, ok := rb.(map[string]any); ok {
					if t, ok := rm["text"].(string); ok {
						resultText += t
					}
				}
			}
		}
		out = append(out, map[string]any{
			"role":         "tool",
			"tool_call_id": toolUseID,
			"content":      resultText,
		})
	}
	// if no tool_result blocks found, return original message
	if len(out) == 0 {
		return []map[string]any{msg}
	}
	return out
}
