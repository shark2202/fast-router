// Package router: shared error mapping for the zig wrapper (no build tag).
package router

import "fmt"

// frScoreError maps the zig wrapper's error codes to messages.
func frScoreError(code int32) error {
	messages := map[int32]string{
		-1: "nil handle",
		-2: "nil ctx",
		-3: "nil model",
		-4: "prompt tokenize failed (too long for buffer?)",
		-5: "too many candidates (>256)",
		-6: "candidate code is not a single token",
		-7: "llama_decode failed",
		-8: "llama_get_logits_ith returned null",
	}
	if msg, ok := messages[code]; ok {
		return fmt.Errorf("fr_score: %s (code %d)", msg, code)
	}
	return fmt.Errorf("fr_score: unknown error (code %d)", code)
}
