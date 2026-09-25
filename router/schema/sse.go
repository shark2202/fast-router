package schema

import (
	"encoding/json"
	"strings"
)

// --- SSE: OpenAI chunk → Anthropic events ---
//
// OpenAI stream:   data: {choices:[{delta:{content},finish_reason}]}
// Anthropic stream: message_start → content_block_start → content_block_delta* →
//                   content_block_stop → message_delta → message_stop

func (c *SSEConverter) openAIChunkToAnthropic(data []byte) []string {
	var chunk map[string]any
	if json.Unmarshal(data, &chunk) != nil {
		return nil
	}
	model, _ := chunk["model"].(string)
	id, _ := chunk["id"].(string)
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	finish, _ := choice["finish_reason"].(string)

	var out []string
	if !c.anthStarted {
		c.anthStarted = true
		c.msgID = id
		c.model = model
		// message_start
		ms, _ := json.Marshal(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": id, "type": "message", "role": "assistant",
				"model": model, "content": []any{}, "stop_reason": nil,
			},
		})
		out = append(out, "event: message_start\ndata: "+string(ms)+"\n\n")
		// content_block_start
		cbs, _ := json.Marshal(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		out = append(out, "event: content_block_start\ndata: "+string(cbs)+"\n\n")
	}
	// content delta (text)
	if content, ok := delta["content"].(string); ok && content != "" {
		cbd, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": content}})
		out = append(out, "event: content_block_delta\ndata: "+string(cbd)+"\n\n")
	}
	// tool_calls delta → content_block_start (tool_use) + input_json_delta
	if toolCalls, ok := delta["tool_calls"].([]any); ok {
		for _, tc := range toolCalls {
			tcm, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			fn, _ := tcm["function"].(map[string]any)
			idx := int(tcm["index"].(float64)) // OpenAI delta tool_call has index
			// first delta of this tool_call: content_block_start (tool_use)
			if tcm["id"] != nil {
				name, _ := fn["name"].(string)
				cbs, _ := json.Marshal(map[string]any{"type": "content_block_start", "index": idx + 1, "content_block": map[string]any{"type": "tool_use", "id": tcm["id"], "name": name, "input": map[string]any{}}})
				out = append(out, "event: content_block_start\ndata: "+string(cbs)+"\n\n")
			}
			// arguments delta → input_json_delta
			if args, ok := fn["arguments"].(string); ok && args != "" {
				cbd, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": idx + 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": args}})
				out = append(out, "event: content_block_delta\ndata: "+string(cbd)+"\n\n")
			}
		}
	}
	// finish
	if finish != "" {
		// content_block_stop
		out = append(out, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		// message_delta
		md, _ := json.Marshal(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": openAIToAnthropicStop(finish)}})
		out = append(out, "event: message_delta\ndata: "+string(md)+"\n\n")
		// message_stop
		out = append(out, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}
	return out
}

// --- SSE: Anthropic events → OpenAI chunks ---

func (c *SSEConverter) anthropicEventToOpenAI(data []byte) []string {
	var evt map[string]any
	if json.Unmarshal(data, &evt) != nil {
		return nil
	}
	evtType, _ := evt["type"].(string)
	switch evtType {
	case "message_start":
		msg, _ := evt["message"].(map[string]any)
		c.msgID, _ = msg["id"].(string)
		c.model, _ = msg["model"].(string)
		c.started = true
		// initial chunk with role
		ch, _ := json.Marshal(map[string]any{
			"id": c.msgID, "object": "chat.completion.chunk",
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
		})
		return []string{"data: " + string(ch) + "\n\n"}
	case "content_block_delta":
		d, _ := evt["delta"].(map[string]any)
		text, _ := d["text"].(string)
		if text == "" {
			return nil
		}
		ch, _ := json.Marshal(map[string]any{
			"id": c.msgID, "object": "chat.completion.chunk", "model": c.model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}},
		})
		return []string{"data: " + string(ch) + "\n\n"}
	case "message_delta":
		d, _ := evt["delta"].(map[string]any)
		stop, _ := d["stop_reason"].(string)
		ch, _ := json.Marshal(map[string]any{
			"id": c.msgID, "object": "chat.completion.chunk", "model": c.model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": anthropicToOpenAIStop(stop)}},
		})
		return []string{"data: " + string(ch) + "\n\n"}
	case "message_stop":
		return []string{"data: [DONE]\n\n"}
	}
	return nil
}

// StreamProxy reads SSE lines from upstream, converts, writes to client.
// Used by gateway.forward when streaming.
func StreamProxy(upstreamBody []byte) string {
	return strings.TrimSpace(string(upstreamBody))
}
