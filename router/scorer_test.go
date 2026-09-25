package router

import (
	"math"
	"testing"
)

// mockBackend returns pre-canned logits for any prompt/codes (test-only).
type mockBackend struct {
	logits []float64
}

func (m mockBackend) ChoiceScore(prompt string, codes []string) ([]float64, Usage, error) {
	return m.logits, Usage{InputTokens: 100, OutputTokens: 1}, nil
}

func TestScoreChoicePicksHighestProbability(t *testing.T) {
	// 3 candidates; logits favor the 2nd (index 1).
	mb := mockBackend{logits: []float64{1.0, 5.0, 2.0}}
	s := NewScorer(mb, "jev-local-test")
	resp, err := s.Score(System1Request{
		State: "write a sort function",
		Questions: map[string]Question{
			"task_type": {
				Type:         "choice",
				Instructions: "pick the task type",
				Criteria: map[string]string{
					"A": "code generation",
					"B": "code review",
					"C": "reasoning",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ans := resp.Answers["task_type"]
	if ans.Type != "choice" {
		t.Fatalf("want choice, got %s", ans.Type)
	}
	// options sorted: A, B, C; logits [1,5,2] -> B highest
	if ans.Choice != "B" {
		t.Fatalf("want B, got %s", ans.Choice)
	}
	// probabilities sum to ~1
	var sum float64
	for _, p := range ans.Probabilities {
		sum += p
	}
	if math.Abs(sum-1.0) > 0.01 {
		t.Fatalf("probabilities sum %v != 1", sum)
	}
	// confidence = max prob = B's prob
	if ans.Confidence != ans.Probabilities["B"] {
		t.Fatalf("confidence %v != max prob %v", ans.Confidence, ans.Probabilities["B"])
	}
	// B should have highest prob
	if ans.Probabilities["B"] <= ans.Probabilities["A"] || ans.Probabilities["B"] <= ans.Probabilities["C"] {
		t.Fatalf("B should be highest: %v", ans.Probabilities)
	}
}

func TestScoreChoiceRespects255Limit(t *testing.T) {
	big := make(map[string]string)
	for i := 0; i < 256; i++ {
		big[string(rune('a'+i%26))+string(rune('a'+i/26))] = "x"
	}
	mb := mockBackend{logits: make([]float64, 256)}
	s := NewScorer(mb, "m")
	_, err := s.Score(System1Request{
		State:     "x",
		Questions: map[string]Question{"q": {Type: "choice", Criteria: big}},
	})
	if err == nil {
		t.Fatal("want error for >255 options")
	}
}

func TestScoreChoiceEmptyCriteriaErrors(t *testing.T) {
	mb := mockBackend{}
	s := NewScorer(mb, "m")
	_, err := s.Score(System1Request{
		State:     "x",
		Questions: map[string]Question{"q": {Type: "choice", Criteria: map[string]string{}}},
	})
	if err == nil {
		t.Fatal("want error for empty criteria")
	}
}

func TestScoreRejectsNonChoice(t *testing.T) {
	mb := mockBackend{}
	s := NewScorer(mb, "m")
	_, err := s.Score(System1Request{
		State:     "x",
		Questions: map[string]Question{"q": {Type: "noul", Criteria: map[string]string{}}},
	})
	if err == nil {
		t.Fatal("want error for non-choice question")
	}
}

func TestCandidateCodeLabelsAtoJ(t *testing.T) {
	labels := candidateCodeLabels(10)
	want := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}
	for i, w := range want {
		if labels[i] != w {
			t.Fatalf("label %d: want %s got %s", i, w, labels[i])
		}
	}
}

func TestSoftmaxSumsToOne(t *testing.T) {
	probs := softmax([]float64{1.0, 2.0, 3.0})
	var sum float64
	for _, p := range probs {
		sum += p
	}
	if math.Abs(sum-1.0) > 0.0001 {
		t.Fatalf("sum %v != 1", sum)
	}
	// softmax is monotonic: higher logit -> higher prob
	if !(probs[2] > probs[1] && probs[1] > probs[0]) {
		t.Fatalf("not monotonic: %v", probs)
	}
}

func TestBuildChoicePromptIncludesRubric(t *testing.T) {
	prompt := buildChoicePrompt(
		"pick task",
		[]string{"A", "B"},
		[]string{"A", "B"},
		map[string]string{"A": "code gen", "B": "review"},
		"write code",
	)
	if !contains(prompt, "code gen") || !contains(prompt, "review") {
		t.Fatalf("rubric missing in prompt:\n%s", prompt)
	}
	if !contains(prompt, "write code") {
		t.Fatalf("state missing in prompt:\n%s", prompt)
	}
	if !contains(prompt, "Which code?") {
		t.Fatalf("answer slot missing:\n%s", prompt)
	}
}

func TestRouteChainScorerToMatcher(t *testing.T) {
	// End-to-end: Scorer picks a task type -> C4 Match -> C5 Select.
	mb := mockBackend{logits: []float64{5.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}}
	s := NewScorer(mb, "jev-local")
	resp, _ := s.Score(System1Request{
		State: "write a sort function",
		Questions: map[string]Question{
			"task": {Type: "choice", Instructions: "pick", Criteria: map[string]string{
				"A": "x", "B": "x", "C": "x", "D": "x", "E": "x",
				"F": "x", "G": "x", "H": "x", "I": "x", "J": "x",
			}},
		},
	})
	choice := resp.Answers["task"].Choice // "A" (highest logit)
	if choice != "A" {
		t.Fatalf("want A, got %s", choice)
	}
	// A = code_generation -> C4 Match -> C5 Select
	surv := Match(choice, SeedRegistry)
	chosen, err := Select(surv, choice, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	// code:high survivors cold-start cheapest = qwen (deepseek blocked tool_use:low)
	if chosen.ModelID != "qwen/qwen-max" {
		t.Fatalf("want qwen, got %s", chosen.ModelID)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
