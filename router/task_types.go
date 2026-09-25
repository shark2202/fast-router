// Package router: fast-router core — Jev-style single-token task-type routing.
package router

// TaskType is a seed task type for Jev scoring (code A..J) + its capability
// requirement vector used by the matcher (C4) to block-disqualify models.
type TaskType struct {
	Code                 string
	Name                 string
	Label                string
	Description          string
	CapabilityRequirement map[string]string // axis -> "high"|"med"|"low"
}

// SeedTaskTypes: 10 seed types. Codes A..J are single ASCII tokens; the Jev
// scorer verifies the llama.cpp tokenizer encodes each as one token at runtime.
var SeedTaskTypes = []TaskType{
	{"A", "code_generation", "代码生成", "生成或修改源代码、实现功能、写函数/类/脚本。",
		map[string]string{"code": "high", "reasoning": "med", "tool_use": "med"}},
	{"B", "code_review_debug", "代码审查/调试", "审查代码、定位 bug、解释代码行为、提出修复。",
		map[string]string{"code": "high", "reasoning": "high", "long_context": "med"}},
	{"C", "reasoning_analysis", "推理/分析", "多步推理、逻辑分析、因果推断、决策推演。",
		map[string]string{"reasoning": "high", "math": "med"}},
	{"D", "long_document_processing", "长文档处理", "总结/抽取/问答长文档（超 8K token）。",
		map[string]string{"long_context": "high", "reasoning": "med"}},
	{"E", "structured_extraction", "结构化抽取", "从文本抽取结构化字段、生成 JSON/表格。",
		map[string]string{"structured_output": "high", "instruction_follow": "high"}},
	{"F", "creative_writing", "创意写作", "写文案、故事、邮件、营销文本等创意内容。",
		map[string]string{"creative": "high", "multilingual": "med"}},
	{"G", "multimodal_understanding", "多模态理解", "理解图片/截图/图表内容（vision 输入）。",
		map[string]string{"vision": "high", "reasoning": "med"}},
	{"H", "tool_agent_task", "工具调用 agent 任务", "调用外部工具/函数/API 完成的多步 agent 任务。",
		map[string]string{"tool_use": "high", "reasoning": "high", "code": "med"}},
	{"I", "simple_qa", "简单问答", "事实性问答、短回答、闲聊。",
		map[string]string{"general": "med"}},
	{"J", "multilingual_translation", "多语言翻译", "翻译文本到另一语言。",
		map[string]string{"multilingual": "high"}},
}

// ByCode: O(1) lookup after Jev scoring picks a code.
var ByCode = func() map[string]TaskType {
	m := make(map[string]TaskType, len(SeedTaskTypes))
	for _, t := range SeedTaskTypes {
		m[t.Code] = t
	}
	return m
}()

var levelOrder = map[string]int{"low": 0, "med": 1, "high": 2}

// MeetsRequirement returns true if declared meets every axis in requirement.
// Missing axis in declared = does not meet (blocked). This is the C4 挡死 logic.
func MeetsRequirement(declared, requirement map[string]string) bool {
	for axis, required := range requirement {
		d, ok := declared[axis]
		if !ok {
			return false
		}
		if levelOrder[d] < levelOrder[required] {
			return false
		}
	}
	return true
}

// CandidateDescriptions builds the candidate table for the Jev prompt.
func CandidateDescriptions() string {
	out := ""
	for _, t := range SeedTaskTypes {
		out += t.Code + ": " + t.Name + " - " + t.Description + "\n"
	}
	return out
}

// mainActionCues: explicit discrimination signals for small models (added
// after POC1 v0 showed 0.5B mis-routing broadly to B). Each cue names the
// single discriminating action.
var mainActionCues = map[string]string{
	"A": "produce new code (write/implement/generate a function/script/program)",
	"B": "inspect EXISTING code for problems (review/find bug/debug)",
	"C": "reason / analyze / compare / weigh (not code, not a given document)",
	"D": "process a GIVEN long document or report (summarize/extract/QA it)",
	"E": "output structured data (JSON / CSV / table / fields)",
	"F": "write creative text (prose / story / copy / email)",
	"G": "understand an image / screenshot / chart (vision input)",
	"H": "call external tools / calendar / API / deploy (agent task)",
	"I": "answer a simple factual question (short lookup)",
	"J": "translate between languages",
}

// PromptDescriptions builds the candidate table with MAIN ACTION cues — helps
// small models discriminate. Used by the Jev prompt.
func PromptDescriptions() string {
	out := ""
	for _, t := range SeedTaskTypes {
		cue := mainActionCues[t.Code]
		out += t.Code + ": " + t.Name + " - " + t.Description + " (MAIN ACTION: " + cue + ")\n"
	}
	return out
}
