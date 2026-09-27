// cmd/zigbench: end-to-end C3 test — Go(purego) → zig wrapper → libllama.
//
// Loads a GGUF model via the zig wrapper and runs one Choice score,
// verifying the full system-one chain works in-process (no cgo, no HTTP).
//
// Usage:
//
//	DYLD_LIBRARY_PATH=/tmp/llama-bins/llama-b11119 \
//	go run ./cmd/zigbench /path/to/qwen2.5-0.5b-instruct-q4_k_m.gguf
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"fast-router/router"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: zigbench <gguf-model-path> [-yesno-bench]")
	}
	modelPath := os.Args[1]
	libPath := "zig/libfrwrapper.dylib"

	fmt.Fprintf(os.Stderr, "loading zig backend: lib=%s model=%s\n", libPath, modelPath)
	backend, err := router.NewZigBackend(libPath, modelPath)
	if err != nil {
		log.Fatalf("NewZigBackend: %v", err)
	}
	defer backend.Close()
	fmt.Fprintf(os.Stderr, "backend loaded; scorer ready\n")

	if len(os.Args) > 2 && os.Args[2] == "-yesno-bench" {
		var ext router.ExtendedBackend = backend
		// PAD (env): repeat the long state to simulate agent-length contexts —
		// the KV-reuse win scales with the shared-prefix fraction.
		pad := 1
		if v := os.Getenv("PAD"); v != "" {
			fmt.Sscanf(v, "%d", &pad)
		}
		long := strings.Repeat("Help me refactor this Go module to reduce coupling between the gateway and the scorer package; it currently imports internal types across boundaries and the tests construct fixtures manually. ", pad)
		yesnoBench(ext, []string{
			"实现一个 Python 快速排序函数，要求原地排序",
			long,
		})
		return
	}

	scorer := router.NewScorer(backend, "jev-local-zig")

	// Build Choice criteria from the 10 seed task types.
	criteria := map[string]string{}
	for _, t := range router.SeedTaskTypes {
		criteria[t.Code] = t.Description
	}

	// Run on a few sample messages (subset of the Python POC samples).
	samples := []string{
		"实现一个 Python 快速排序函数，要求原地排序", // expect A
		"把这段中文翻译成英文",                // expect J
		"法国的首都是哪里",                  // expect I
		"写一篇关于秋天的散文，800 字",          // expect F
	}
	for _, msg := range samples {
		resp, err := scorer.Score(router.System1Request{
			State: msg,
			Questions: map[string]router.Question{
				"task_type": {Type: "choice", Instructions: "pick the task type", Criteria: criteria},
			},
		})
		if err != nil {
			log.Printf("score failed for %q: %v", msg, err)
			continue
		}
		ans := resp.Answers["task_type"]
		fmt.Printf("msg=%q\n  choice=%s confidence=%.3f\n  probs=%v\n\n", msg, ans.Choice, ans.Confidence, ans.Probabilities)
	}
}

// yesnoBench: A/B benchmark — per-candidate ScoreYesNo (N full prefills) vs
// ScoreYesNoBatch (shared-prefix KV reuse: 1 prefill + N suffix decodes).
// Verifies logit agreement and reports wall-time speedup.
func yesnoBench(ext router.ExtendedBackend, states []string) {
	batcher, canBatch := ext.(router.YesNoBatcher)
	if !canBatch {
		log.Fatal("backend does not implement YesNoBatcher")
	}
	yesID, err := ext.GetTokenID("yes")
	if err != nil {
		log.Fatalf("GetTokenID(yes): %v", err)
	}
	criteria := map[string]string{}
	for _, t := range router.SeedTaskTypes {
		criteria[t.Code] = t.Description
	}
	// deterministic option order (same as scorer.orderedOptions: sorted keys)
	opts := make([]string, 0, len(criteria))
	for code := range criteria {
		opts = append(opts, code)
	}
	sort.Strings(opts)

	var tSingle, tBatch int64
	var maxDelta float64
	argmaxAgree := 0
	for _, state := range states {
		prompts := make([]string, len(opts))
		for i, opt := range opts {
			msgs := fmt.Sprintf(`[{"role":"user","content":"%s\nQuestion: Is this about \"%s\" (%s)? Answer yes or no."}]`,
				strings.ReplaceAll(strings.ReplaceAll(state, "\\", "\\\\"), "\"", "\\\""), opt, criteria[opt])
			p, err := ext.ApplyChatTemplate(msgs, true)
			if err != nil {
				log.Fatalf("ApplyChatTemplate: %v", err)
			}
			prompts[i] = p
		}

		start := time.Now()
		// byte-level LCP across prompts (diagnostic: where do prompts diverge?)
		blcp := len(prompts[0])
		for i := 1; i < len(prompts); i++ {
			a, b := prompts[i-1], prompts[i]
			j := 0
			for j < len(a) && j < len(b) && a[j] == b[j] {
				j++
			}
			if j < blcp {
				blcp = j
			}
		}
		fmt.Printf("[diag] prompt0 len=%d byteLCP=%d\n  p0: %q\n  p1: %q\n", len(prompts[0]), blcp,
			prompts[0][maxInt(0, blcp-25):minInt(len(prompts[0]), blcp+45)],
			prompts[1][maxInt(0, blcp-25):minInt(len(prompts[1]), blcp+45)])
		if idx := strings.Index(prompts[0], "Help me refactor"); idx >= 0 {
			occ := strings.Count(prompts[0], "Help me refactor")
			fmt.Printf("[diag2] state occurrences=%d first@%d len=%d\n", occ, idx, len(prompts[0]))
		}
		ls, _, err := ext.ScoreYesNo(prompts, yesID)
		if err != nil {
			log.Fatalf("ScoreYesNo: %v", err)
		}
		dSingle := time.Since(start).Milliseconds()

		start = time.Now()
		lb, _, err := batcher.ScoreYesNoBatch(prompts, yesID)
		if err != nil {
			log.Fatalf("ScoreYesNoBatch: %v", err)
		}
		dBatch := time.Since(start).Milliseconds()

		bestS, bestB := 0, 0
		for i := range ls {
			if d := math.Abs(ls[i] - lb[i]); d > maxDelta {
				maxDelta = d
			}
			if ls[i] > ls[bestS] {
				bestS = i
			}
			if lb[i] > lb[bestB] {
				bestB = i
			}
		}
		if bestS == bestB {
			argmaxAgree++
		}
		tSingle += dSingle
		tBatch += dBatch
		fmt.Printf("state=%-30q single=%4dms batch=%4dms argmax=%s/%s\n",
			state, dSingle, dBatch, opts[bestS], opts[bestB])
	}
	fmt.Printf("\nTOTAL single=%dms batch=%dms speedup=%.2fx | maxLogitDelta=%.4f | argmax agree %d/%d\n",
		tSingle, tBatch, float64(tSingle)/float64(max(tBatch, 1)), maxDelta, argmaxAgree, len(states))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
