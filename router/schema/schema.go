// Package schema: OpenAI ↔ Anthropic request/response/SSE conversion.
//
// When a client speaks Anthropic (/v1/messages) but the chosen upstream speaks
// OpenAI (or vice versa), this converts between the two. Same-protocol
// requests pass through unchanged.
//
// Conversions cover (per docs/fast-router-架构设计-v0.2.md 附录 B):
//   - request: system field position, message structure, tool format
//   - response: choices→content blocks, finish_reason→stop_reason
//   - SSE: OpenAI chunk stream ↔ Anthropic event state machine
package schema

import (
	"encoding/json"
	"strings"
)

// ConvertRequest converts a client request body to the upstream's protocol.
// clientProto/upstreamProto: "openai" or "anthropic".
// Returns the (possibly rewritten) body + the content-type.
func ConvertRequest(body []byte, clientProto, upstreamProto string) ([]byte, string) {
	if clientProto == upstreamProto {
		return body, contentType(clientProto)
	}
	if clientProto == "anthropic" && upstreamProto == "openai" {
		return anthropicToOpenAI(body), "application/json"
	}
	if clientProto == "openai" && upstreamProto == "anthropic" {
		return openAIToAnthropic(body), "application/json"
	}
	return body, contentType(clientProto)
}

// ConvertResponse converts an upstream response body to the client's protocol.
func ConvertResponse(body []byte, upstreamProto, clientProto string) []byte {
	if upstreamProto == clientProto {
		return body
	}
	if upstreamProto == "openai" && clientProto == "anthropic" {
		return openAIResponseToAnthropic(body)
	}
	if upstreamProto == "anthropic" && clientProto == "openai" {
		return anthropicResponseToOpenAI(body)
	}
	return body
}

// SSEConverter streams SSE chunks, converting between OpenAI chunk format and
// Anthropic event format. Use one per streaming response.
type SSEConverter struct {
	from, to string
	// Anthropic→OpenAI state
	started   bool
	msgID     string
	model     string
	// OpenAI→Anthropic state
	anthStarted bool
}

// NewSSEConverter creates a converter for streaming SSE.
func NewSSEConverter(from, to string) *SSEConverter {
	return &SSEConverter{from: from, to: to}
}

// ConvertChunk takes one SSE "data: ..." line (without the "data: " prefix)
// and returns zero or more output lines (each including "data: " prefix or
// empty for pass-through). Returns nil for lines to skip.
func (c *SSEConverter) ConvertChunk(data []byte) []string {
	if c.from == c.to {
		return []string{"data: " + string(data) + "\n\n"}
	}
	if c.from == "openai" && c.to == "anthropic" {
		return c.openAIChunkToAnthropic(data)
	}
	if c.from == "anthropic" && c.to == "openai" {
		return c.anthropicEventToOpenAI(data)
	}
	return []string{"data: " + string(data) + "\n\n"}
}

// IsDone checks if the chunk signals stream end.
func (c *SSEConverter) IsDone(data []byte) bool {
	return strings.TrimSpace(string(data)) == "[DONE]"
}

func contentType(proto string) string {
	return "application/json"
}

// --- helpers ---

func jget(raw json.RawMessage) any {
	var v any
	json.Unmarshal(raw, &v)
	return v
}

func jmust(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
