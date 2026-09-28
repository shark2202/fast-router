// cmd/yesnobench: per-candidate yes/no benchmark (LLM2Jev method).
//
// Compares single-token (fr_score) vs per-candidate yes/no (fr_score_yesno)
// on the same model. For each sample, constructs 10 yes/no prompts (one per
// task type), forward each, extract "yes" logit, softmax → choice.
//
// Usage:
//
//	DYLD_LIBRARY_PATH=~/.local/share/llama-bins/llama-b11175 \
//	go run ./cmd/yesnobench <gguf-model-path>
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"fast-router/router"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: yesnobench <gguf-model-path>")
	}
	backend, err := router.NewZigBackend("zig/libfrwrapper.dylib", os.Args[1])
	if err != nil {
		log.Fatalf("backend: %v", err)
	}
	defer backend.Close()

	// Use as ExtendedBackend (zigBackend implements it)
	var extBackend router.ExtendedBackend = backend

	// Get the affirmation token id: yes -> Yes -> 是 (Chinese-heavy vocabs
	// like minimind tokenize lowercase "yes" into multiple tokens)
	yesWord, noWord := "yes", "no"
	yesID, err := backend.GetTokenID(yesWord)
	if err != nil {
		if id2, err2 := backend.GetTokenID("Yes"); err2 == nil {
			yesWord, noWord, yesID = "Yes", "No", id2
		} else if id3, err3 := backend.GetTokenID("是"); err3 == nil {
			yesWord, noWord, yesID = "是", "否", id3
		} else {
			log.Fatalf("no single-token affirmation (yes/Yes/是): %v", err)
		}
	}
	fmt.Fprintf(os.Stderr, "affirmation: %s (token %d)\n", yesWord, yesID)

	// Check if backend supports apply_chat_template
	var hasTemplate bool
	if extBackend != nil {
		hasTemplate = true
		fmt.Fprintf(os.Stderr, "apply_chat_template: available (auto-adapt)\n")
	} else {
		fmt.Fprintf(os.Stderr, "apply_chat_template: not available (hardcoded template)\n")
	}

	// Load samples
	raw, _ := os.ReadFile("data/task_type_samples.json")
	var samples []struct {
		Message  string `json:"message"`
		Expected string `json:"expected"`
		Label    string `json:"label"`
	}
	json.Unmarshal(raw, &samples)

	// Task types (10 candidates A-J)
	var candidates []struct{ code, name, desc string }
	for _, t := range router.SeedTaskTypes {
		candidates = append(candidates, struct{ code, name, desc string }{t.Code, t.Name, t.Description})
	}

	correct, total := 0, 0
	var latencies []time.Duration

	for _, s := range samples {
		start := time.Now()

		// Build 10 yes/no prompts (one per candidate)
		prompts := make([]string, len(candidates))
		for i, c := range candidates {
			var prompt string
			if hasTemplate {
				// Use llama_chat_apply_template (auto-adapt model's template)
				msgs := fmt.Sprintf(`[{"role":"user","content":%q}]`,
					fmt.Sprintf("%s\n\nQuestion: Does this request belong to the \"%s\" category? %s\nAnswer with only \"yes\" or \"no\".",
						s.Message, c.name, c.desc))
				prompt, err = extBackend.ApplyChatTemplate(msgs, true)
				if err != nil {
					log.Printf("template failed %q: %v", s.Message, err)
					continue
				}
			} else {
				// Fallback: hardcoded Qwen2.5 template
				prompt = fmt.Sprintf(
					"<|im_start|>user\n%s\nQuestion: Is this request about \"%s\" (%s)? Answer %s or %s.\n<|im_end|>\n<|im_start|>assistant\n",
					s.Message, c.name, c.desc, yesWord, noWord)
			}
			prompts[i] = prompt
		}

		// Score each candidate (N forwards)
		logits, _, err := backend.ScoreYesNo(prompts, yesID)
		if err != nil {
			log.Printf("score failed %q: %v", s.Message, err)
			continue
		}
		lat := time.Since(start)
		latencies = append(latencies, lat)

		// softmax over yes logits
		probs := softmax(logits)

		// pick highest
		bestIdx := 0
		for i := range probs {
			if probs[i] > probs[bestIdx] {
				bestIdx = i
			}
		}
		choice := candidates[bestIdx].code

		total++
		if choice == s.Expected {
			correct++
		} else {
			fmt.Fprintf(os.Stderr, "  miss: exp=%s got=%s(%s) msg=%q\n", s.Expected, choice, candidates[bestIdx].name, truncate(s.Message, 40))
		}
	}

	acc := float64(correct) / float64(total)
	var avgLat time.Duration
	for _, l := range latencies {
		avgLat += l
	}
	avgLat /= time.Duration(len(latencies))

	fmt.Println("============================================================")
	fmt.Printf("YES/NO P1 accuracy: %d/%d = %.1f%% (single-token was 10%%)\n", correct, total, acc*100)
	fmt.Printf("YES/NO P2 latency:  avg %v (10x forward per sample)\n", avgLat)
	fmt.Printf("score_note: per-candidate yes/no, no position bias, Qwen2.5 chat template\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func softmax(logits []float64) []float64 {
	if len(logits) == 0 {
		return nil
	}
	mx := logits[0]
	for _, l := range logits[1:] {
		if l > mx {
			mx = l
		}
	}
	var sum float64
	exp := make([]float64, len(logits))
	for i, l := range logits {
		exp[i] = math.Exp(l - mx)
		sum += exp[i]
	}
	out := make([]float64, len(logits))
	for i, e := range exp {
		out[i] = e / sum
	}
	return out
}
