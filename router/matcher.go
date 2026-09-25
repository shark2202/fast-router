package router

// C4: capability matcher — block-disqualify models whose declared capability
// does not meet the task type's requirement.
//
// task_code -> capability_requirement (from task_types) -> filter registry.
// Survivors go to the C5 selector. This is the 声明-level 挡死 layer (design
// consensus Q4): a model that declares code:none is blocked for code_generation
// regardless of cost or measured performance — prevents Type III routing.

// Match returns models whose declared capability_vector meets the task's requirement.
func Match(taskCode string, registry []ModelEntry) []ModelEntry {
	tt, ok := ByCode[taskCode]
	if !ok {
		return nil
	}
	req := tt.CapabilityRequirement
	var survivors []ModelEntry
	for _, m := range registry {
		if MeetsRequirement(m.CapabilityVector, req) {
			survivors = append(survivors, m)
		}
	}
	return survivors
}

// BlockedReasons reports which requirement axes each blocked model failed (for logging).
func BlockedReasons(taskCode string, registry []ModelEntry) map[string][]string {
	tt, ok := ByCode[taskCode]
	if !ok {
		return nil
	}
	req := tt.CapabilityRequirement
	reasons := map[string][]string{}
	for _, m := range registry {
		var failed []string
		for axis, required := range req {
			declared, ok := m.CapabilityVector[axis]
			if !ok {
				failed = append(failed, axis+":missing")
			} else if levelOrder[declared] < levelOrder[required] {
				failed = append(failed, axis+":"+declared+"<"+required)
			}
		}
		if len(failed) > 0 {
			reasons[m.ModelID] = failed
		}
	}
	return reasons
}
