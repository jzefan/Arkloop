package pipeline

import (
	"strings"
	"testing"

	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
)

func TestCompactThreadMessages_trimCount(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "a"}}},
		{Role: "assistant", Content: []llm.TextPart{{Text: "b"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "c"}}},
	}
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	cfg := ContextCompactSettings{Enabled: true, MaxMessages: 2}
	out, outIDs, dropped := CompactThreadMessages(msgs, ids, cfg, nil)
	if dropped != 1 || len(out) != 2 {
		t.Fatalf("expected drop 1, len 2, got dropped=%d len=%d", dropped, len(out))
	}
	if out[0].Role != "assistant" || outIDs[0] != ids[1] {
		t.Fatalf("unexpected suffix start: %#v", out[0].Role)
	}
}

func TestCompactThreadMessages_userTokenBudget(t *testing.T) {
	long := strings.Repeat("a", 600) // 150 近似 tokens，与尾部 user 合计超预算
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: long}}},
		{Role: "assistant", Content: []llm.TextPart{{Text: "ok"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "tail"}}},
	}
	cfg := ContextCompactSettings{Enabled: true, MaxUserMessageTokens: 120}
	out, _, dropped := CompactThreadMessages(msgs, nil, cfg, nil)
	if dropped == 0 || len(out) == len(msgs) {
		t.Fatalf("expected head dropped, dropped=%d len=%d", dropped, len(out))
	}
	if out[len(out)-1].Role != "user" {
		t.Fatal("expected tail preserved")
	}
}

func TestContextCompactHasActiveBudget(t *testing.T) {
	if ContextCompactHasActiveBudget(ContextCompactSettings{Enabled: true}) {
		t.Fatal("expected false when all budgets zero")
	}
	if !ContextCompactHasActiveBudget(ContextCompactSettings{Enabled: true, MaxMessages: 1}) {
		t.Fatal("expected true")
	}
}

func TestTrimPrefixMessagesForCompactLLM_keepsNewestUnderCap(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: strings.Repeat("x", 8000)}}},
		{Role: "user", Content: []llm.TextPart{{Text: "tail-marker"}}},
	}
	out := TrimPrefixMessagesForCompactLLM(enc, msgs, 80)
	if len(out) != 1 {
		t.Fatalf("expected single message kept, got %d", len(out))
	}
	if messageText(out[0]) != "tail-marker" {
		t.Fatalf("expected newest segment kept")
	}
}

// ---------------------------------------------------------------------------
// computeTailKeepByTokenBudget
// ---------------------------------------------------------------------------

func TestComputeTailKeepByTokenBudget_AllShortMessages(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := make([]llm.Message, 10)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Content: []llm.TextPart{{Text: "hi"}}}
	}
	got := computeTailKeepByTokenBudget(enc, msgs, 100000, 0)
	if got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestComputeTailKeepByTokenBudget_AllShortMessages_MaxMessagesLimit(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := make([]llm.Message, 10)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Content: []llm.TextPart{{Text: "hi"}}}
	}
	got := computeTailKeepByTokenBudget(enc, msgs, 100000, 5)
	if got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestComputeTailKeepByTokenBudget_MixedSizes(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("x", 40000)
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: huge}}},
		{Role: "user", Content: []llm.TextPart{{Text: huge}}},
		{Role: "user", Content: []llm.TextPart{{Text: huge}}},
		{Role: "user", Content: []llm.TextPart{{Text: "hi"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "hi"}}},
	}
	got := computeTailKeepByTokenBudget(enc, msgs, 5000, 0)
	if got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}

func TestComputeTailKeepByTokenBudget_SingleHugeMessage(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: strings.Repeat("a", 200000)}}},
	}
	got := computeTailKeepByTokenBudget(enc, msgs, 100, 0)
	if got != 1 {
		t.Fatalf("expected 1 (at-least-one guarantee), got %d", got)
	}
}

func TestComputeTailKeepByTokenBudget_EmptyMessages(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	got := computeTailKeepByTokenBudget(enc, nil, 1000, 0)
	if got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestComputeTailKeepByTokenBudget_ZeroBudget(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "a"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "b"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "c"}}},
	}
	got := computeTailKeepByTokenBudget(enc, msgs, 0, 0)
	if got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// truncateLargeTailMessages
// ---------------------------------------------------------------------------

func TestTruncateLargeTailMessages_NoTruncation(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "short"}}},
		{Role: "assistant", Content: []llm.TextPart{{Text: "reply"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "also short"}}},
	}
	out := truncateLargeTailMessages(enc, msgs)
	for i, m := range out {
		if messageText(m) != messageText(msgs[i]) {
			t.Fatalf("msg[%d] changed unexpectedly", i)
		}
	}
}

func TestTruncateLargeTailMessages_TruncatesOldLargeUser(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 40000) // ~10K tokens
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: large}}},
		{Role: "assistant", Content: []llm.TextPart{{Text: "ok"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "hi"}}},
	}
	out := truncateLargeTailMessages(enc, msgs)
	if !strings.Contains(messageText(out[0]), "[... content truncated") {
		t.Fatal("expected first user message to be truncated")
	}
	if messageText(out[2]) != "hi" {
		t.Fatal("last user message should be untouched")
	}
}

func TestTruncateLargeTailMessages_SkipsLastUser(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 40000)
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "small"}}},
		{Role: "user", Content: []llm.TextPart{{Text: large}}},
	}
	out := truncateLargeTailMessages(enc, msgs)
	if messageText(out[1]) != large {
		t.Fatal("last user message must not be truncated")
	}
}

func TestTruncateLargeTailMessages_SkipsAssistant(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 40000)
	msgs := []llm.Message{
		{Role: "assistant", Content: []llm.TextPart{{Text: large}}},
	}
	out := truncateLargeTailMessages(enc, msgs)
	if messageText(out[0]) != large {
		t.Fatal("assistant messages must not be truncated")
	}
}

func TestTruncateLargeTailMessages_OriginalUnmodified(t *testing.T) {
	enc, err := tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", 40000)
	msgs := []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: large}}},
		{Role: "user", Content: []llm.TextPart{{Text: "tail"}}},
	}
	origText := messageText(msgs[0])
	_ = truncateLargeTailMessages(enc, msgs)
	if messageText(msgs[0]) != origText {
		t.Fatal("original slice must not be modified")
	}
}
