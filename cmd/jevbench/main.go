// cmd/jevbench: 30-sample P1 benchmark — Go(purego)→zig→llama.cpp, in-process.
//
// Reads data/task_type_samples.json, runs Scorer on each, reports P1 accuracy
// + per-class + latency. Compares against the Python POC (0.5B transformers:
// 13.3%, 1.5B: 46.7%) — same paradigm, different backend (zig+libllama vs
// torch), cross-checks that the Go path reproduces/exceeds the Python result.
//
// Usage:
//   DYLD_LIBRARY_PATH=/tmp/llama-bins/llama-b11175 \
//   go run ./cmd/jevbench <gguf-model-path>
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"fast-router/router"
)

type sample struct {
	Message  string `json:"message"`
	Expected string `json:"expected"`
	Label    string `json:"label"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: jevbench <gguf-model-path>")
	}
	modelPath := os.Args[1]

	backend, err := router.NewZigBackend("zig/libfrwrapper.dylib", modelPath)
	if err != nil {
		log.Fatalf("NewZigBackend: %v", err)
	}
	defer backend.Close()
	scorer := router.NewScorer(backend, "jev-local-zig")

	raw, err := os.ReadFile("data/task_type_samples.json")
	if err != nil {
		log.Fatalf("read samples: %v", err)
	}
	var samples []sample
	if err := json.Unmarshal(raw, &samples); err != nil {
		log.Fatalf("parse samples: %v", err)
	}

	criteria := map[string]string{}
	for _, t := range router.SeedTaskTypes {
		criteria[t.Code] = t.Description
	}

	correct, total := 0, 0
	perClass := map[string][2]int{} // expected -> {correct, total}
	var latencies []time.Duration
	var mistakes []string

	for _, s := range samples {
		start := time.Now()
		resp, err := scorer.Score(router.System1Request{
			State: s.Message,
			Questions: map[string]router.Question{
				"task_type": {Type: "choice", Instructions: "pick the task type", Criteria: criteria},
			},
		})
		lat := time.Since(start)
		latencies = append(latencies, lat)
		if err != nil {
			log.Printf("score failed %q: %v", s.Message, err)
			continue
		}
		choice := resp.Answers["task_type"].Choice
		total++
		pc := perClass[s.Expected]
		pc[1]++
		if choice == s.Expected {
			correct++
			pc[0]++
		} else {
			mistakes = append(mistakes, fmt.Sprintf("  exp=%s(%s) got=%s msg=%q", s.Expected, s.Label, choice, trunc(s.Message, 40)))
		}
		perClass[s.Expected] = pc
	}

	acc := float64(correct) / float64(total)
	var avgLat time.Duration
	for _, l := range latencies {
		avgLat += l
	}
	avgLat /= time.Duration(len(latencies))

	fmt.Println("============================================================")
	fmt.Printf("P1 accuracy: %d/%d = %.1f%% (target >70%%) %s\n", correct, total, acc*100, passFail(acc >= 0.70))
	fmt.Printf("P2 latency:  avg %v (target <1s) %s\n", avgLat, passFail(avgLat < time.Second))
	fmt.Println()
	fmt.Println("per-class:")
	for code := "ABCDEFGHIJ"; len(code) > 0; code = code[1:] {
		c := code[:1]
		pc := perClass[c]
		if pc[1] == 0 {
			continue
		}
		tt := router.ByCode[c]
		fmt.Printf("  %s %-28s %d/%d = %.0f%%\n", c, tt.Name, pc[0], pc[1], float64(pc[0])/float64(pc[1])*100)
	}
	if len(mistakes) > 0 {
		fmt.Printf("\nmisclassifications (%d):\n", len(mistakes))
		for _, m := range mistakes {
			fmt.Println(m)
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
