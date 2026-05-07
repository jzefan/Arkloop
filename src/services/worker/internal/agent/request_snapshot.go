package agent

import "arkloop/services/worker/internal/llm"

// CacheSafeSnapshot captures cache-key-critical request components so
// future fork/subagent flows can reuse the same prefix safely.
type CacheSafeSnapshot struct {
	PersonaID       string
	BaseMessages    []llm.Message
	Model           string
	Messages        []llm.Message
	Tools           []llm.ToolSpec
	MaxOutputTokens *int
	Temperature     *float64
	ReasoningMode   string
	ToolChoice      *llm.ToolChoice
	PromptPlan      *llm.PromptPlan
}
