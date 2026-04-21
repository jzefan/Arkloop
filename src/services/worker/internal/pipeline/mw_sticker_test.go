package pipeline

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/personas"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPrepareStickerDeliveryOutputs_OnlySplitsWhenPlaceholderExists(t *testing.T) {
	clean, segments := prepareStickerDeliveryOutputs([]string{"hello world"})
	if len(clean) != 1 || clean[0] != "hello world" {
		t.Fatalf("unexpected clean outputs: %#v", clean)
	}
	if len(segments) != 0 {
		t.Fatalf("expected no segments, got %#v", segments)
	}
}

func TestPrepareStickerDeliveryOutputs_ParsesStickerSequence(t *testing.T) {
	clean, segments := prepareStickerDeliveryOutputs([]string{"hi [sticker:abc] there"})
	if len(clean) != 1 || clean[0] != "hi there" {
		t.Fatalf("unexpected clean outputs: %#v", clean)
	}
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %#v", segments)
	}
	if segments[0].Kind != "text" || segments[0].Text != "hi" {
		t.Fatalf("unexpected first segment: %#v", segments[0])
	}
	if segments[1].Kind != "sticker" || segments[1].StickerID != "abc" {
		t.Fatalf("unexpected sticker segment: %#v", segments[1])
	}
	if segments[2].Kind != "text" || segments[2].Text != "there" {
		t.Fatalf("unexpected last segment: %#v", segments[2])
	}
}

func TestParseStickerBuilderOutput(t *testing.T) {
	description, tags, ok := parseStickerBuilderOutput("描述: 无语又想笑的狗头\n标签: 狗头, 阴阳怪气, 吐槽")
	if !ok {
		t.Fatal("expected parse success")
	}
	if description != "无语又想笑的狗头" {
		t.Fatalf("unexpected description: %q", description)
	}
	if tags != "狗头, 阴阳怪气, 吐槽" {
		t.Fatalf("unexpected tags: %q", tags)
	}
}

func TestStripStickerPlaceholders_PreservesFormatting(t *testing.T) {
	got := stripStickerPlaceholders("第一行\n\n[sticker:abc]\n第三行")
	if got != "第一行\n\n第三行" {
		t.Fatalf("unexpected stripped text: %q", got)
	}
}

func TestStickerToolMiddleware_AddsToolForTelegramRuns(t *testing.T) {
	rc := &RunContext{
		Run: data.Run{AccountID: uuid.New()},
		ChannelContext: &ChannelContext{
			ChannelType: "telegram",
		},
		ToolExecutors: map[string]tools.Executor{},
		AllowlistSet:  map[string]struct{}{},
		ToolRegistry:  tools.NewRegistry(),
		PersonaDefinition: &personas.Definition{
			ToolAllowlist: []string{stickerSearchToolName},
		},
	}
	mw := NewStickerToolMiddleware(fakeStickerQueryDB{})
	if err := mw(context.Background(), rc, func(ctx context.Context, rc *RunContext) error { return nil }); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if _, ok := rc.AllowlistSet[stickerSearchToolName]; !ok {
		t.Fatalf("expected %s in allowlist", stickerSearchToolName)
	}
	if rc.ToolExecutors[stickerSearchToolName] == nil {
		t.Fatalf("expected %s executor bound", stickerSearchToolName)
	}
	if _, ok := rc.ToolRegistry.Get(stickerSearchToolName); !ok {
		t.Fatalf("expected %s registered", stickerSearchToolName)
	}
}

func TestStickerToolMiddleware_RespectsPersonaAllowlist(t *testing.T) {
	rc := &RunContext{
		Run: data.Run{AccountID: uuid.New()},
		ChannelContext: &ChannelContext{
			ChannelType: "telegram",
		},
		ToolExecutors: map[string]tools.Executor{},
		AllowlistSet:  map[string]struct{}{},
		ToolRegistry:  tools.NewRegistry(),
		PersonaDefinition: &personas.Definition{
			ToolAllowlist: []string{"read"},
		},
	}
	mw := NewStickerToolMiddleware(fakeStickerQueryDB{})
	if err := mw(context.Background(), rc, func(ctx context.Context, rc *RunContext) error { return nil }); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if _, ok := rc.AllowlistSet[stickerSearchToolName]; ok {
		t.Fatalf("did not expect %s in allowlist", stickerSearchToolName)
	}
	if rc.ToolExecutors[stickerSearchToolName] != nil {
		t.Fatalf("did not expect %s executor bound", stickerSearchToolName)
	}
}

type fakeStickerQueryDB struct{}

func (fakeStickerQueryDB) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }

func (fakeStickerQueryDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeStickerRow{}
}

type fakeStickerRow struct{}

func (fakeStickerRow) Scan(...any) error { return nil }
