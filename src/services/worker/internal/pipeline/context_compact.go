package pipeline

import (
	"strings"

	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
)

// ContextCompactSettings 来自平台配置，供 ContextCompactMiddleware 使用。
type ContextCompactSettings struct {
	Enabled bool

	// MaxMessages 尾部最多保留多少条消息；0 表示不按条数收缩。
	MaxMessages int

	MaxUserMessageTokens int
	MaxTotalTextTokens   int
	MaxUserTextBytes     int
	MaxTotalTextBytes    int

	PersistEnabled             bool
	PersistTriggerApproxTokens int
	PersistKeepLastMessages    int
}

func approxTokensFromText(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

func messageText(m llm.Message) string {
	var b strings.Builder
	for _, p := range m.Content {
		b.WriteString(llm.PartPromptText(p))
	}
	return b.String()
}

func countUserTokens(msgs []llm.Message, start int) int {
	n := 0
	for i := start; i < len(msgs); i++ {
		if msgs[i].Role == "user" {
			n += approxTokensFromText(messageText(msgs[i]))
		}
	}
	return n
}

func countTotalTokens(msgs []llm.Message, start int) int {
	n := 0
	for i := start; i < len(msgs); i++ {
		n += approxTokensFromText(messageText(msgs[i]))
	}
	return n
}

func countUserBytes(msgs []llm.Message, start int) int {
	n := 0
	for i := start; i < len(msgs); i++ {
		if msgs[i].Role == "user" {
			n += len(messageText(msgs[i]))
		}
	}
	return n
}

func countTotalBytes(msgs []llm.Message, start int) int {
	n := 0
	for i := start; i < len(msgs); i++ {
		n += len(messageText(msgs[i]))
	}
	return n
}

// stabilizeCompactStart 在「尾部条数上限」与「不以孤立 tool 开头」之间收敛切口。
func stabilizeCompactStart(msgs []llm.Message, start int, maxMessages int) int {
	if len(msgs) == 0 {
		return 0
	}
	maxIter := len(msgs) + 8
	for iter := 0; iter < maxIter; iter++ {
		for start > 0 && start < len(msgs) && msgs[start].Role == "tool" {
			start--
		}
		if maxMessages <= 0 || len(msgs)-start <= maxMessages {
			break
		}
		start++
		if start >= len(msgs) {
			start = len(msgs) - 1
			break
		}
	}
	for start < len(msgs)-1 && msgs[start].Role == "tool" {
		start++
	}
	return start
}

func budgetOK(msgs []llm.Message, start int, cfg ContextCompactSettings) bool {
	if cfg.MaxMessages > 0 && len(msgs)-start > cfg.MaxMessages {
		return false
	}
	if cfg.MaxUserMessageTokens > 0 && countUserTokens(msgs, start) > cfg.MaxUserMessageTokens {
		return false
	}
	if cfg.MaxTotalTextTokens > 0 && countTotalTokens(msgs, start) > cfg.MaxTotalTextTokens {
		return false
	}
	if cfg.MaxUserTextBytes > 0 && countUserBytes(msgs, start) > cfg.MaxUserTextBytes {
		return false
	}
	if cfg.MaxTotalTextBytes > 0 && countTotalBytes(msgs, start) > cfg.MaxTotalTextBytes {
		return false
	}
	return true
}

// CompactThreadMessages 从头部裁掉消息直到满足预算；保证切口不以孤立的 tool 开头（尽力左扩）。
// ids 若与 msgs 等长则同步裁切；否则 ids 原样截断或置 nil。
func CompactThreadMessages(msgs []llm.Message, ids []uuid.UUID, cfg ContextCompactSettings) ([]llm.Message, []uuid.UUID, int) {
	if len(msgs) == 0 {
		return msgs, ids, 0
	}
	start := 0
	if cfg.MaxMessages > 0 && len(msgs) > cfg.MaxMessages {
		start = len(msgs) - cfg.MaxMessages
	}
	start = stabilizeCompactStart(msgs, start, cfg.MaxMessages)
	for start < len(msgs) && !budgetOK(msgs, start, cfg) {
		start++
		start = stabilizeCompactStart(msgs, start, cfg.MaxMessages)
	}
	if start <= 0 {
		return msgs, alignIDs(ids, len(msgs)), 0
	}
	out := make([]llm.Message, len(msgs)-start)
	copy(out, msgs[start:])
	var outIDs []uuid.UUID
	if len(ids) == len(msgs) {
		outIDs = make([]uuid.UUID, len(ids)-start)
		copy(outIDs, ids[start:])
	}
	return out, outIDs, start
}

func alignIDs(ids []uuid.UUID, n int) []uuid.UUID {
	if len(ids) == n {
		return ids
	}
	return nil
}

// ContextCompactHasActiveBudget enabled 为真且至少有一项预算大于 0。
func ContextCompactHasActiveBudget(cfg ContextCompactSettings) bool {
	if !cfg.Enabled {
		return false
	}
	return cfg.MaxMessages > 0 ||
		cfg.MaxUserMessageTokens > 0 ||
		cfg.MaxTotalTextTokens > 0 ||
		cfg.MaxUserTextBytes > 0 ||
		cfg.MaxTotalTextBytes > 0
}
