package rollout

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"arkloop/services/shared/objectstore"

	"github.com/google/uuid"
)

// Reader 从 S3 读取并解析 JSONL rollout 文件。
type Reader struct {
	store objectstore.BlobStore
}

func NewReader(store objectstore.BlobStore) *Reader {
	return &Reader{store: store}
}

// ReadRollout 下载 S3 上的 JSONL 文件并解析为 RolloutItem 列表。
func (r *Reader) ReadRollout(ctx context.Context, runID uuid.UUID) ([]RolloutItem, error) {
	key := "run/" + runID.String() + ".jsonl"
	data, err := r.store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var items []RolloutItem
	reader := bufio.NewReader(bytes.NewReader(data))
	lineNum := 0
	for {
		chunk, readErr := reader.ReadBytes('\n')
		if len(chunk) == 0 && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("read rollout %s: %w", runID, readErr)
		}
		lineNum++
		line := bytes.TrimSpace(chunk)
		if len(line) == 0 {
			if readErr == io.EOF {
				break
			}
			continue
		}
		var item RolloutItem
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("parse rollout line %d: %w", lineNum, err)
		}
		items = append(items, item)
		if readErr == io.EOF {
			break
		}
	}
	return items, nil
}

// Reconstruct 顺序扫描 RolloutItem 列表，重建 assistant/tool 回放序列和未完成 tool call。
func (r *Reader) Reconstruct(items []RolloutItem) *ReconstructedState {
	state := &ReconstructedState{}
	pending := map[string]ToolCall{}
	pendingOrder := make([]string, 0)
	toolNames := map[string]string{}
	currentTurnIndex := 0

	for _, item := range items {
		switch item.Type {
		case "run_end":
			var payload RunEnd
			if json.Unmarshal(item.Payload, &payload) == nil {
				state.FinalStatus = payload.FinalStatus
			}
		case "turn_start":
			var payload TurnStart
			if json.Unmarshal(item.Payload, &payload) == nil {
				currentTurnIndex = payload.TurnIndex
			}
		case "assistant_message":
			var payload AssistantMessage
			if json.Unmarshal(item.Payload, &payload) != nil {
				continue
			}
			state.Messages = append(state.Messages, item.Payload)
			state.ReplayMessages = append(state.ReplayMessages, ReplayMessage{
				Role:      "assistant",
				Assistant: &payload,
			})
		case "tool_call":
			var payload ToolCall
			if json.Unmarshal(item.Payload, &payload) != nil {
				continue
			}
			pending[payload.CallID] = payload
			toolNames[payload.CallID] = payload.Name
			pendingOrder = append(pendingOrder, payload.CallID)
		case "tool_result":
			var payload ToolResult
			if json.Unmarshal(item.Payload, &payload) != nil {
				continue
			}
			delete(pending, payload.CallID)
			state.ReplayMessages = append(state.ReplayMessages, ReplayMessage{
				Role: "tool",
				Tool: &ReplayToolResult{
					CallID: payload.CallID,
					Name:   toolNames[payload.CallID],
					Output: payload.Output,
					Error:  payload.Error,
				},
			})
		case "turn_end":
			var payload TurnEnd
			if json.Unmarshal(item.Payload, &payload) == nil {
				currentTurnIndex = payload.TurnIndex
			}
		}
	}

	for _, callID := range pendingOrder {
		call, ok := pending[callID]
		if !ok {
			continue
		}
		state.PendingToolCalls = append(state.PendingToolCalls, call)
	}
	if len(state.PendingToolCalls) > 0 {
		state.Breakpoint = &Breakpoint{TurnIndex: currentTurnIndex}
	}
	return state
}
