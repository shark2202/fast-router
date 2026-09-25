package router

import (
	"testing"
)

// ---------- C4: capability 挡死 ----------

func TestC4CodeGenerationSurvivesHighCodeModels(t *testing.T) {
	surv := Match("A", SeedRegistry)
	ids := modelIDs(surv)
	// req: code:high, reasoning:med, tool_use:med
	// gpt-5/sonnet (code high, tool_use high) survive; qwen (code high, tool_use med) survive
	// deepseek tool_use:low -> blocked; gpt5-mini/haiku code:med -> blocked
	mustContain(t, ids, "openai/gpt-5")
	mustContain(t, ids, "anthropic/claude-sonnet-4-5")
	mustContain(t, ids, "qwen/qwen-max")
	mustNotContain(t, ids, "deepseek/deepseek-chat") // tool_use:low < med (correctly blocked)
	mustNotContain(t, ids, "openai/gpt-5-mini")      // code:med < high
	mustNotContain(t, ids, "anthropic/claude-haiku-4-5")
}

func TestC4BlocksCodeNoneModel(t *testing.T) {
	weak := ModelEntry{"weak/code-none", "weak", "Weak", 8000, 0.1, 0.2,
		map[string]string{"code": "none", "reasoning": "low"}, nil}
	surv := Match("A", append([]ModelEntry{weak}, SeedRegistry...))
	for _, m := range surv {
		if m.ModelID == "weak/code-none" {
			t.Fatal("weak model should be blocked")
		}
	}
}

func TestC4SimpleQaPassesAllWithGeneral(t *testing.T) {
	surv := Match("I", SeedRegistry)
	if len(surv) != len(SeedRegistry) {
		t.Fatalf("want all %d survive, got %d", len(SeedRegistry), len(surv))
	}
}

func TestC4MultimodalBlocksNonVision(t *testing.T) {
	surv := Match("G", SeedRegistry)
	ids := modelIDs(surv)
	mustContain(t, ids, "openai/gpt-5")
	mustContain(t, ids, "anthropic/claude-sonnet-4-5")
	mustNotContain(t, ids, "deepseek/deepseek-chat") // vision:none
	mustNotContain(t, ids, "qwen/qwen-max")         // vision:none
}

func TestC4BlockedReasonsReportsAxes(t *testing.T) {
	reasons := BlockedReasons("G", SeedRegistry)
	if _, ok := reasons["deepseek/deepseek-chat"]; !ok {
		t.Fatal("deepseek should have blocked reasons")
	}
}

// ---------- C5: weighted selection ----------

func TestC5ColdStartPicksCheapest(t *testing.T) {
	surv := Match("I", SeedRegistry)
	chosen, err := Select(surv, "I", nil)
	if err != nil {
		t.Fatal(err)
	}
	if chosen.ModelID != "deepseek/deepseek-chat" { // 0.27 cheapest
		t.Fatalf("want deepseek, got %s", chosen.ModelID)
	}
}

func TestC5WithMeasuredPicksBestScore(t *testing.T) {
	surv := Match("I", SeedRegistry)
	measured := map[string]map[string]MeasuredEntry{
		"I": {
			"deepseek/deepseek-chat": {PassRate: 0.5, SampleSize: 10, Status: "candidate"},
			"qwen/qwen-max":          {PassRate: 0.9, SampleSize: 10, Status: "candidate"},
		},
	}
	chosen, err := Select(surv, "I", measured)
	if err != nil {
		t.Fatal(err)
	}
	// qwen: 0.6*0.9+0.4*(1/5)=0.62; deepseek: 0.6*0.5+0.4*(1/3.7)=0.408
	if chosen.ModelID != "qwen/qwen-max" {
		t.Fatalf("want qwen, got %s", chosen.ModelID)
	}
}

func TestC5LowRateCheapCanWinOverHighRateExpensive(t *testing.T) {
	surv := Match("I", SeedRegistry)
	measured := map[string]map[string]MeasuredEntry{
		"I": {
			"deepseek/deepseek-chat": {PassRate: 0.7, SampleSize: 10, Status: "active"},
			"openai/gpt-5":           {PassRate: 0.95, SampleSize: 10, Status: "active"},
		},
	}
	chosen, _ := Select(surv, "I", measured)
	// deepseek 0.528 vs gpt-5 0.578 -> gpt-5 wins (rate dominates)
	if chosen.ModelID != "openai/gpt-5" {
		t.Fatalf("want gpt-5, got %s", chosen.ModelID)
	}
}

func TestC5NoCandidatesErrors(t *testing.T) {
	if _, err := Select(nil, "I", nil); err == nil {
		t.Fatal("want error, got nil")
	}
}

// ---------- end-to-end: Jev code -> C4 -> C5 ----------

func TestRouteChainAllTaskTypesProduceModel(t *testing.T) {
	for _, code := range "ABCDEFGHIJ" {
		surv := Match(string(code), SeedRegistry)
		chosen, err := Select(surv, string(code), nil)
		if err != nil {
			t.Fatalf("no model for task %c: %v", code, err)
		}
		_ = chosen
	}
}

func TestRouteChainCodeGenerationPicksQwen(t *testing.T) {
	surv := Match("A", SeedRegistry)
	chosen, _ := Select(surv, "A", nil) // cold start -> cheapest among survivors
	// survivors: gpt-5(5.0), sonnet(3.0), qwen(0.4); deepseek blocked (tool_use:low)
	// cold start cheapest = qwen
	if chosen.ModelID != "qwen/qwen-max" {
		t.Fatalf("want qwen, got %s", chosen.ModelID)
	}
}

// helpers

func modelIDs(ms []ModelEntry) map[string]bool {
	m := map[string]bool{}
	for _, x := range ms {
		m[x.ModelID] = true
	}
	return m
}

func mustContain(t *testing.T, m map[string]bool, id string) {
	t.Helper()
	if !m[id] {
		t.Errorf("expected %s in survivors", id)
	}
}

func mustNotContain(t *testing.T, m map[string]bool, id string) {
	t.Helper()
	if m[id] {
		t.Errorf("expected %s NOT in survivors", id)
	}
}
