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
	// extract top-level system → prepend as role:"system" message
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
	out := make(map[string]any)
	for k, v := range m {
		if k == "system" || k == "messages" {
			continue
		}
		out[k] = jget(v)
	}
	out["messages"] = msgs
	b, _ := json.Marshal(out)
	return b
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
	// extract system messages → top-level system
	var system any
	var filtered []map[string]any
	for _, msg := range msgs {
		if r, _ := msg["role"].(string); r == "system" {
			system = msg["content"]
			continue
		}
		filtered = append(filtered, msg)
	}
	out := make(map[string]any)
	for k, v := range m {
		if k == "messages" {
			continue
		}
		out[k] = jget(v)
	}
	if system != nil {
		out["system"] = system
	}
	out["messages"] = filtered
	b, _ := json.Marshal(out)
	return b
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

	out := map[string]any{
		"id":   m["id"],
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
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
