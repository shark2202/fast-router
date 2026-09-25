package schema

import "encoding/json"

// --- Anthropic request → OpenAI request ---

// Anthropic request: {model, messages:[{role,content}], system, max_tokens, ...}
// OpenAI request:    {model, messages:[{role,content}], max_tokens, ...}
// Key diff: Anthropic system is top-level; OpenAI system is a message with role:"system".

func anthropicToOpenAI(body []byte) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	var msgs []map[string]any
	if raw, ok := m["messages"]; ok {
		json.Unmarshal(raw, &msgs)
	}
	if sys, ok := m["system"]; ok {
		var sysText any
		json.Unmarshal(sys, &sysText)
		sysMsg := map[string]any{"role": "system", "content": sysText}
		msgs = append([]map[string]any{sysMsg}, msgs...)
	}
	// convert Anthropic messages to OpenAI format
	var openAIMsgs []map[string]any
	for _, msg := range msgs {
		openAIMsgs = append(openAIMsgs, anthropicMsgToOpenAI(msg)...)
	}
	out := make(map[string]any)
	for k, v := range m {
		if k == "system" || k == "messages" || k == "tools" {
			continue
		}
		out[k] = jget(v)
	}
	out["messages"] = openAIMsgs
	// convert tools: Anthropic {name,description,input_schema} → OpenAI {type:function,function:{...}}
	if rawTools, ok := m["tools"]; ok {
		var tools []any
		json.Unmarshal(rawTools, &tools)
		out["tools"] = convertAnthropicToolsToOpenAI(tools)
	}
	b, _ := json.Marshal(out)
	return b
}

// anthropicMsgToOpenAI: convert one Anthropic message to OpenAI message(s).
// assistant with tool_use → OpenAI assistant with tool_calls.
// user with tool_result → multiple OpenAI tool messages (one per result block).
func anthropicMsgToOpenAI(msg map[string]any) []map[string]any {
	role, _ := msg["role"].(string)
	if role == "assistant" {
		content, _ := msg["content"].([]any)
		text, toolCalls := anthropicToolUseToOpenAI(content)
		result := map[string]any{"role": "assistant"}
		if text != "" {
			result["content"] = text
		}
		if len(toolCalls) > 0 {
			result["tool_calls"] = toolCalls
		}
		return []map[string]any{result}
	}
	if role == "user" {
		content, _ := msg["content"].([]any)
		if hasToolResult(content) {
			return anthropicToolResultToOpenAI(msg)
		}
	}
	return []map[string]any{msg}
}

func hasToolResult(content []any) bool {
	for _, blk := range content {
		if b, ok := blk.(map[string]any); ok {
			if t, _ := b["type"].(string); t == "tool_result" {
				return true
			}
		}
	}
	return false
}

// --- OpenAI request → Anthropic request ---

func openAIToAnthropic(body []byte) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	var msgs []map[string]any
	if raw, ok := m["messages"]; ok {
		json.Unmarshal(raw, &msgs)
	}
	var system any
	var filtered []map[string]any
	for _, msg := range msgs {
		if r, _ := msg["role"].(string); r == "system" {
			system = msg["content"]
			continue
		}
		filtered = append(filtered, msg)
	}
	// convert OpenAI messages to Anthropic format
	var anthMsgs []map[string]any
	for _, msg := range filtered {
		anthMsgs = append(anthMsgs, openAIMsgToAnthropic(msg))
	}
	out := make(map[string]any)
	for k, v := range m {
		if k == "messages" || k == "tools" {
			continue
		}
		out[k] = jget(v)
	}
	if system != nil {
		out["system"] = system
	}
	out["messages"] = anthMsgs
	// convert tools: OpenAI {type:function,function:{...}} → Anthropic {name,description,input_schema}
	if rawTools, ok := m["tools"]; ok {
		var tools []any
		json.Unmarshal(rawTools, &tools)
		out["tools"] = convertOpenAIToolsToAnthropic(tools)
	}
	b, _ := json.Marshal(out)
	return b
}

// openAIMsgToAnthropic: convert one OpenAI message to Anthropic format.
// assistant with tool_calls → Anthropic assistant with content blocks (text + tool_use).
// tool message → Anthropic user message with tool_result content block.
func openAIMsgToAnthropic(msg map[string]any) map[string]any {
	role, _ := msg["role"].(string)
	if role == "assistant" {
		if _, hasToolCalls := msg["tool_calls"]; hasToolCalls {
			content := openAIToolCallsToAnthropic(msg)
			return map[string]any{"role": "assistant", "content": content}
		}
		return msg
	}
	if role == "tool" {
		return openAIToolResultToAnthropic(msg)
	}
	return msg
}

// --- OpenAI response → Anthropic response ---

// OpenAI:  {choices:[{message:{role,content},finish_reason}], usage}
// Anthropic: {id, content:[{type:"text",text}], stop_reason, usage}

func openAIResponseToAnthropic(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	choices, _ := m["choices"].([]any)
	if len(choices) == 0 {
		return body
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	content, _ := msg["content"].(string)
	finish, _ := choice["finish_reason"].(string)

	// build content blocks
	var blocks []any
	if content != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": content})
	}
	// convert tool_calls to tool_use blocks
	if toolCalls, ok := msg["tool_calls"].([]any); ok {
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
			json.Unmarshal([]byte(argsStr), &input)
			id, _ := tcm["id"].(string)
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
				"input": input,
			})
		}
	}

	out := map[string]any{
		"id":   m["id"],
		"type": "message",
		"role": "assistant",
		"content": blocks,
		"model":       m["model"],
		"stop_reason": openAIToAnthropicStop(finish),
	}
	if u, ok := m["usage"]; ok {
		out["usage"] = u
	}
	b, _ := json.Marshal(out)
	return b
}

// --- Anthropic response → OpenAI response ---

func anthropicResponseToOpenAI(body []byte) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return body
	}
	// content blocks → message.content string
	content, _ := m["content"].([]any)
	var text string
	for _, blk := range content {
		if b, ok := blk.(map[string]any); ok {
			if t, _ := b["type"].(string); t == "text" {
				if s, _ := b["text"].(string); s != "" {
					text += s
				}
			}
		}
	}
	stop, _ := m["stop_reason"].(string)
	out := map[string]any{
		"id":      m["id"],
		"object":  "chat.completion",
		"model":   m["model"],
		"choices": []map[string]any{
			{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": anthropicToOpenAIStop(stop)},
		},
	}
	if u, ok := m["usage"]; ok {
		out["usage"] = u
	}
	b, _ := json.Marshal(out)
	return b
}

func openAIToAnthropicStop(s string) string {
	switch s {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	}
	return s
}

func anthropicToOpenAIStop(s string) string {
	switch s {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	}
	return s
}
