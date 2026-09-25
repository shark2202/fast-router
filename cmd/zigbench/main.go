// cmd/zigbench: end-to-end C3 test — Go(purego) → zig wrapper → libllama.
//
// Loads a GGUF model via the zig wrapper and runs one Choice score,
// verifying the full system-one chain works in-process (no cgo, no HTTP).
//
// Usage:
//   DYLD_LIBRARY_PATH=/tmp/llama-bins/llama-b11119 \
//   go run ./cmd/zigbench /path/to/qwen2.5-0.5b-instruct-q4_k_m.gguf
package main

import (
	"fmt"
	"log"
	"os"

	"fast-router/router"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: zigbench <gguf-model-path>")
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

	scorer := router.NewScorer(backend, "jev-local-zig")

	// Build Choice criteria from the 10 seed task types.
	criteria := map[string]string{}
	for _, t := range router.SeedTaskTypes {
		criteria[t.Code] = t.Description
	}

	// Run on a few sample messages (subset of the Python POC samples).
	samples := []string{
		"实现一个 Python 快速排序函数，要求原地排序",   // expect A
		"把这段中文翻译成英文",                       // expect J
		"法国的首都是哪里",                          // expect I
		"写一篇关于秋天的散文，800 字",                // expect F
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
