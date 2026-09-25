// Package router: C3 — Jev system-one scorer (Choice type), backend-agnostic.
//
// Follows the Jev system-one API format (https://docs.typesafe.ai/api):
//   POST /v1/system1 {state, model, questions:{id:{type,instructions,criteria}}}
//   -> {model, answers:{id:{choice, probabilities, confidence}}, usage}
//
// This file = pure-Go API surface (structs + Choice logic):
// candidate code labels, criteria→candidate mapping, softmax over logits.
// The inference backend (local zig wrapper+libllama, or cloud Jev API) plugs
// in via the Backend interface — Score() is backend-agnostic and testable
// with a mock.
package router

import (
	"errors"
	"math"
	"sort"
	"strings"
)

// --- Jev system-one API structs (JSON schema per docs.typesafe.ai/api) ---

type System1Request struct {
	State     string              `json:"state"`     // content to evaluate (message text)
	Model     string              `json:"model"`     // "jev-latest" (local)
	Questions map[string]Question `json:"questions"` // {id: Question}
}

type Question struct {
	Type         string            `json:"type"`         // "choice" | "noul" | "score"
	Instructions string            `json:"instructions"` // policy / what to decide
	Criteria     map[string]string `json:"criteria"`     // choice: {option: rubric} (≤255)
}

type System1Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage            `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Answer: the choice variant (router uses Choice for task-type routing).
type Answer struct {
	Type          string             `json:"type"`          // "choice"
	Choice        string             `json:"choice"`         // highest-probability option
	Probabilities map[string]float64 `json:"probabilities"`  // {option: prob}, sum=1
	Confidence    float64            `json:"confidence"`     // 0-1, derived from distribution
}

// --- Backend: pluggable inference (local zig+libllama, or cloud Jev API) ---

// Backend scores candidate code labels for a prompt. The impl tokenizes the
// prompt, verifies each code is a single token, runs one forward pass, and
// returns the raw logit for each candidate (in order). Go-side softmaxes.
type Backend interface {
	ChoiceScore(prompt string, candidateCodes []string) (logits []float64, usage Usage, err error)
}

// --- C3 scorer: Choice logic over a Backend ---

type Scorer struct {
	backend Backend
	model   string
}

func NewScorer(backend Backend, model string) *Scorer {
	return &Scorer{backend: backend, model: model}
}

// Score evaluates a system-one request. For each "choice" question, it maps
// criteria options to single-token candidate codes, asks the backend for
// logits on those candidates, softmaxes into probabilities, and returns the
// highest-probability choice + full distribution + confidence.
func (s *Scorer) Score(req System1Request) (System1Response, error) {
	answers := make(map[string]Answer, len(req.Questions))
	totalIn, totalOut := 0, 0
	for id, q := range req.Questions {
		switch q.Type {
		case "choice":
			ans, in, out, err := s.scoreChoice(req.State, q)
			if err != nil {
				return System1Response{}, err
			}
			answers[id] = ans
			totalIn += in
			totalOut += out
		default:
			return System1Response{}, errors.New("only choice questions supported (router path)")
		}
	}
	return System1Response{
		Model:   s.model,
		Answers: answers,
		Usage:   Usage{InputTokens: totalIn, OutputTokens: totalOut},
	}, nil
}

// scoreChoice: criteria options -> candidate codes -> backend logits -> softmax.
func (s *Scorer) scoreChoice(state string, q Question) (Answer, int, int, error) {
	if len(q.Criteria) == 0 {
		return Answer{}, 0, 0, errors.New("choice question has no criteria")
	}
	if len(q.Criteria) > 255 {
		return Answer{}, 0, 0, errors.New("choice criteria exceeds 255 options (Jev limit)")
	}
	options := orderedOptions(q.Criteria)            // deterministic option order
	codes := candidateCodeLabels(len(options))        // A, B, ..., Z, AA, ...
	prompt := buildChoicePrompt(q.Instructions, options, codes, q.Criteria, state)
	logits, usage, err := s.backend.ChoiceScore(prompt, codes)
	if err != nil {
		return Answer{}, 0, 0, err
	}
	if len(logits) != len(options) {
		return Answer{}, 0, 0, errors.New("backend returned wrong number of logits")
	}
	probs := softmax(logits)
	probMap := make(map[string]float64, len(options))
	for i, opt := range options {
		probMap[opt] = probs[i]
	}
	bestIdx := 0
	for i := range probs {
		if probs[i] > probs[bestIdx] {
			bestIdx = i
		}
	}
	return Answer{
		Type:          "choice",
		Choice:        options[bestIdx],
		Probabilities: probMap,
		Confidence:    probs[bestIdx],
	}, usage.InputTokens, usage.OutputTokens, nil
}

// orderedOptions: deterministic (sorted) order of criteria keys for stability.
func orderedOptions(crit map[string]string) []string {
	keys := make([]string, 0, len(crit))
	for k := range crit {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// candidateCodeLabels: A, B, ..., Z, AA, AB, ... (single-token-friendly).
// Mirrors fast_browser_use candidate_codes; backend verifies each is one token.
func candidateCodeLabels(n int) []string {
	pool := codeLabelPool()
	if n > len(pool) {
		n = len(pool)
	}
	out := make([]string, n)
	copy(out, pool[:n])
	return out
}

func codeLabelPool() []string {
	var out []string
	for c := 'A'; c <= 'Z'; c++ {
		out = append(out, string(c))
	}
	for c1 := 'A'; c1 <= 'Z'; c1++ {
		for c2 := 'A'; c2 <= 'Z'; c2++ {
			out = append(out, string([]rune{c1, c2}))
		}
	}
	return out // 26 + 676 = 702 > 255 Jev limit
}

// buildChoicePrompt: Qwen2.5 chat-formatted (so llama.cpp tokenizes with proper
// special tokens). Raw prompt without chat template gives poor P1 (model expects
// chat format). Wraps content in <|im_start|>user...<|im_end|>\n<|im_start|>assistant\n.
func buildChoicePrompt(instructions string, options, codes []string, criteria map[string]string, state string) string {
	var b strings.Builder
	b.WriteString("<|im_start|>user\n")
	b.WriteString(instructions)
	b.WriteString("\nCANDIDATES:\n")
	for i, opt := range options {
		rubric := criteria[opt]
		b.WriteString(codes[i])
		b.WriteString(": ")
		b.WriteString(opt)
		if rubric != "" {
			b.WriteString(" - ")
			b.WriteString(rubric)
		}
		b.WriteString("\n")
	}
	b.WriteString("STATE:\n")
	b.WriteString(state)
	b.WriteString("\nWhich code?")
	b.WriteString("<|im_end|>\n<|im_start|>assistant\n")
	return b.String()
}

func softmax(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}
	mx := scores[0]
	for _, s := range scores[1:] {
		if s > mx {
			mx = s
		}
	}
	var sum float64
	exp := make([]float64, len(scores))
	for i, s := range scores {
		exp[i] = math.Exp(s - mx)
		sum += exp[i]
	}
	out := make([]float64, len(scores))
	for i, e := range exp {
		out[i] = e / sum
	}
	return out
}
