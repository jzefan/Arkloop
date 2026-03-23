package rollout

import (
	"encoding/json"
	"time"
)

type RolloutItem struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// payload types
type RunMeta struct {
	RunID       string `json:"run_id"`
	SubAgentID  string `json:"sub_agent_id"`
	ParentRunID string `json:"parent_run_id"`
	RootRunID   string `json:"root_run_id"`
	AccountID   string `json:"account_id"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type TurnStart struct {
	TurnIndex int    `json:"turn_index"`
	Model     string `json:"model"`
	Cwd       string `json:"cwd"`
}

type AssistantMessage struct {
	Content   string          `json:"content,omitempty"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	CallID string          `json:"call_id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
}

type ToolResult struct {
	CallID string          `json:"call_id"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type TurnEnd struct {
	TurnIndex               int    `json:"turn_index"`
	LastAssistantMessageRef string `json:"last_assistant_message_ref,omitempty"`
}

type RunEnd struct {
	FinalStatus string `json:"final_status"`
	OutputRef   string `json:"output_ref,omitempty"`
}

type ReconstructedState struct {
	Messages    []json.RawMessage // 重建的 assistant message 序列
	Breakpoint  *Breakpoint       // 断点位置
	FinalStatus string
}

type Breakpoint struct {
	TurnIndex    int    `json:"turn_index"`
	LastToolCall string `json:"last_tool_call_id,omitempty"`
}
