package router

import (
	"errors"
	"math"
)

// C5: weighted selector — from C4 survivors, pick the best upstream+model by
// measured_matrix + cost.
//
// Cold start (no measured data for this task across candidates): fall back to
// cheapest input cost (G-R3 cold-start 降级). Once C8 回填 populates measured
// pass_rates, selection is weighted:
//   score = WMeasured * pass_rate + WCost * cost_score
// where cost_score = 1/(1 + 10*input_cost) (cheaper -> higher, bounded in (0,1)).

const (
	WMeasured = 0.6
	WCost     = 0.4
)

// Select picks the best model from C4 survivors.
//   candidates: C4 survivors (non-empty).
//   taskCode:   task type code (A..J) for measured-matrix lookup.
//   measured:   map[taskCode]map[modelID]MeasuredEntry (cold-start nil/empty OK).
// Returns the chosen ModelEntry, or error if no candidates.
func Select(candidates []ModelEntry, taskCode string, measured map[string]map[string]MeasuredEntry) (ModelEntry, error) {
	if len(candidates) == 0 {
		return ModelEntry{}, errors.New("no candidates after blocking — widen registry or relax requirement")
	}
	// Is there any measured data for THIS task among candidates?
	hasMeasured := false
	for _, m := range candidates {
		if rate := measuredRate(m, taskCode, measured); rate != nil {
			hasMeasured = true
			break
		}
	}
	if !hasMeasured {
		// G-R3 cold start: cheapest input cost (声明-level 挡死 already done by C4)
		chosen := candidates[0]
		for _, m := range candidates[1:] {
			if m.InputCostPer1k < chosen.InputCostPer1k {
				chosen = m
			}
		}
		return chosen, nil
	}
	// weighted
	chosen := candidates[0]
	bestScore := score(candidates[0], taskCode, measured)
	for _, m := range candidates[1:] {
		s := score(m, taskCode, measured)
		if s > bestScore {
			bestScore = s
			chosen = m
		}
	}
	return chosen, nil
}

func score(m ModelEntry, taskCode string, measured map[string]map[string]MeasuredEntry) float64 {
	var rate float64
	if e := measuredRate(m, taskCode, measured); e != nil {
		rate = e.PassRate
	}
	cost := m.InputCostPer1k
	if cost <= 0 {
		cost = 1.0
	}
	costScore := 1.0 / (1.0 + 10.0*cost)
	return WMeasured*rate + WCost*costScore
}

func measuredRate(m ModelEntry, taskCode string, measured map[string]map[string]MeasuredEntry) *MeasuredEntry {
	if measured == nil {
		return nil
	}
	taskMap, ok := measured[taskCode]
	if !ok {
		return nil
	}
	e, ok := taskMap[m.ModelID]
	if !ok {
		return nil
	}
	return &e
}

// compile-time guard: ensure math is used (costScore uses no math fn now, but
// keep import for future latency term). Avoid unused import error.
var _ = math.Sqrt
