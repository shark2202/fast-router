// Package router: POC6 — task-turn detector (C2 closure).
//
// Decides whether the last message of a chat request is:
//   - a tool_result continuation (tool loop in progress → inherit current route)
//   - a new user message (new task turn → trigger Jev re-scoring)
//   - ambiguous (e.g. trailing assistant → fallback: inherit last route)
//
// Pure structural judgment, zero deps. Design: design共识 Q7 (task-turn routing).
package router

import "fmt"

type TurnKind int

const (
	NewTaskTurn TurnKind = iota // new user message → trigger Jev
	ToolLoopContinue            // tool_result → inherit route
	Ambiguous                   // fallback → inherit last route
)

func (k TurnKind) String() string {
	switch k {
	case NewTaskTurn:
		return "new_task_turn"
	case ToolLoopContinue:
		return "tool_loop_continue"
	case Ambiguous:
		return "ambiguous"
	}
	return "unknown"
}

// DetectTurn classifies the task turn from the last message.
//
// messages: chat request messages (OpenAI or Anthropic shape, as map[string]any).
// protocol: "openai" or "anthropic".
// Returns (TurnKind, reason).
func DetectTurn(messages []map[string]any, protocol string) (TurnKind, string) {
	if len(messages) == 0 {
		return Ambiguous, "empty messages"
	}
	last := messages[len(messages)-1]
	switch protocol {
	case "openai":
		return detectOpenai(last)
	case "anthropic":
		return detectAnthropic(last)
	}
	return Ambiguous, fmt.Sprintf("unknown protocol: %s", protocol)
}

func detectOpenai(last map[string]any) (TurnKind, string) {
	role, _ := last["role"].(string)
	switch role {
	case "tool":
		return ToolLoopContinue, "trailing role=tool (tool result) → continue"
	case "user":
		return NewTaskTurn, "trailing role=user (new user message) → new turn"
	case "assistant":
		if tc, ok := last["tool_calls"].([]any); ok && len(tc) > 0 {
			return Ambiguous, "trailing assistant with tool_calls (interrupted?) → inherit"
		}
		return Ambiguous, "trailing assistant text (task may have ended) → inherit"
	}
	return Ambiguous, fmt.Sprintf("unknown role: %s", role)
}

func detectAnthropic(last map[string]any) (TurnKind, string) {
	role, _ := last["role"].(string)
	switch role {
	case "user":
		// Anthropic user content can be a string or a list of typed blocks.
		// A tool_result block inside user content = tool-loop continuation.
		if blocks, ok := last["content"].([]any); ok {
			for _, b := range blocks {
				if blk, ok := b.(map[string]any); ok {
					if t, _ := blk["type"].(string); t == "tool_result" {
						return ToolLoopContinue, "trailing user with tool_result block → continue"
					}
				}
			}
		}
		return NewTaskTurn, "trailing user with plain text → new turn"
	case "assistant":
		return Ambiguous, "trailing assistant → inherit"
	}
	return Ambiguous, fmt.Sprintf("unknown role: %s", role)
}

// ShouldInheritRoute returns true if the current turn should inherit the prior
// route (tool loop or ambiguous), false if it is a fresh task turn needing Jev.
func ShouldInheritRoute(messages []map[string]any, protocol string) bool {
	kind, _ := DetectTurn(messages, protocol)
	return kind != NewTaskTurn
}
