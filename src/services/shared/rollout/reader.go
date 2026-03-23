package rollout

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"

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
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(string(scanner.Bytes()))
		if line == "" {
			continue
		}
		var item RolloutItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// Reconstruct 反向扫描 RolloutItem 列表，重建断点状态和消息序列。
// 从尾到头扫描，找到 run_end 和最后一个 assistant_message，构造 ReconstructedState。
func (r *Reader) Reconstruct(items []RolloutItem) *ReconstructedState {
	state := &ReconstructedState{}
	// 反向遍历
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		switch item.Type {
		case "run_end":
			var payload RunEnd
			if json.Unmarshal(item.Payload, &payload) == nil {
				state.FinalStatus = payload.FinalStatus
			}
		case "assistant_message":
			state.Messages = append([]json.RawMessage{item.Payload}, state.Messages...)
		case "tool_call":
			if state.Breakpoint == nil {
				var tc ToolCall
				if json.Unmarshal(item.Payload, &tc) == nil {
					state.Breakpoint = &Breakpoint{LastToolCall: tc.CallID}
				}
			}
		}
	}
	return state
}
