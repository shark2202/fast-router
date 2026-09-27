// Package router: C7 verdict feedback — append-only collection of routing
// outcomes (design consensus v0.1, AC8), plus the C8/C9 data foundation:
// the measured matrix (per task_type × model success rates) and empty-layer
// detection (task types that never get routed).
//
// Boundary: C8 proper (LLM-scout label proposals + independent statistical
// review) and C9 proper (task-type set deltas with rollback) consume this
// data through a manual/future process — deliberately not automated here.
package router

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// VerdictEvent: one routing outcome. Immutable once recorded (I-closure in the
// design consensus: verdicts are historical facts).
type VerdictEvent struct {
	TS       int64  `json:"ts"`        // unix ms
	Session  string `json:"session"`   // session key (hash)
	TaskTurn string `json:"task_turn"` // "new" | "continue"
	TaskCode string `json:"task_code"` // A-J ("" for hint/inherit-unknown routes)
	ModelID  string `json:"model_id"`
	Upstream string `json:"upstream"`
	Via      string `json:"route_via"` // "jev" | "hint" | "inherit" | "strong-hint"
	Status   int    `json:"status"`    // upstream HTTP status (0 = connect fail)
	Outcome  string `json:"outcome"`   // "ok" | "upstream_error" | "connect_error"
}

// VerdictStore: in-memory tail + append-only JSONL persistence.
type VerdictStore struct {
	mu     sync.Mutex
	path   string
	events []VerdictEvent // tail, cap memoryTail
}

const memoryTail = 1000

// NewVerdictStore opens (and best-effort reloads) a verdict JSONL file.
// A nil-safe empty store when path is "".
func NewVerdictStore(path string) *VerdictStore {
	s := &VerdictStore{path: path}
	if path == "" {
		return s
	}
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range splitLines(data) {
			var e VerdictEvent
			if json.Unmarshal(line, &e) == nil {
				s.events = append(s.events, e)
			}
		}
		if over := len(s.events) - memoryTail; over > 0 {
			s.events = s.events[over:]
		}
	}
	return s
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				out = append(out, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

// Record appends an event (JSONL + memory tail). Safe on a nil store.
func (s *VerdictStore) Record(e VerdictEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if over := len(s.events) - memoryTail; over > 0 {
		s.events = s.events[over:]
	}
	if s.path != "" {
		if f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			json.NewEncoder(f).Encode(e)
			f.Close()
		}
	}
}

// Recent returns up to n newest events.
func (s *VerdictStore) Recent(n int) []VerdictEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.events) {
		n = len(s.events)
	}
	out := make([]VerdictEvent, n)
	copy(out, s.events[len(s.events)-n:])
	// newest last → newest first for display
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// MeasuredCell: aggregated outcome per (task_code, model_id).
type MeasuredCell struct {
	TaskCode string  `json:"task_code"`
	ModelID  string  `json:"model_id"`
	Count    int     `json:"count"`
	OK       int     `json:"ok"`
	Err      int     `json:"err"`
	OKRate   float64 `json:"ok_rate"`
}

// MeasuredReport: the C8 calibration input + C9 empty-layer signal.
type MeasuredReport struct {
	Total       int            `json:"total"`
	Cells       []MeasuredCell `json:"cells"`
	TaskCounts  map[string]int `json:"task_counts"`  // decisions per task code
	EmptyLayers []string       `json:"empty_layers"` // seed types never routed (C9)
}

// Measured aggregates recorded verdicts into the measured matrix.
func (s *VerdictStore) Measured() MeasuredReport {
	rep := MeasuredReport{TaskCounts: map[string]int{}}
	if s == nil {
		return rep
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	type key struct{ task, model string }
	cells := map[key]*MeasuredCell{}
	for _, e := range s.events {
		rep.Total++
		if e.TaskCode != "" {
			rep.TaskCounts[e.TaskCode]++
		}
		k := key{e.TaskCode, e.ModelID}
		c, ok := cells[k]
		if !ok {
			c = &MeasuredCell{TaskCode: e.TaskCode, ModelID: e.ModelID}
			cells[k] = c
		}
		c.Count++
		switch e.Outcome {
		case "ok":
			c.OK++
		case "upstream_error", "connect_error":
			c.Err++
		}
	}
	for _, c := range cells {
		if c.Count > 0 {
			c.OKRate = float64(c.OK) / float64(c.Count)
		}
		rep.Cells = append(rep.Cells, *c)
	}
	sort.Slice(rep.Cells, func(i, j int) bool {
		if rep.Cells[i].TaskCode != rep.Cells[j].TaskCode {
			return rep.Cells[i].TaskCode < rep.Cells[j].TaskCode
		}
		return rep.Cells[i].Count > rep.Cells[j].Count
	})
	// C9: seed task types with zero routing decisions
	for _, t := range SeedTaskTypes {
		if rep.TaskCounts[t.Code] == 0 {
			rep.EmptyLayers = append(rep.EmptyLayers, t.Code)
		}
	}
	return rep
}
