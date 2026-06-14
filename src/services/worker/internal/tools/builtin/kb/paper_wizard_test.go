package kb

import (
	"strings"
	"testing"

	"arkloop/services/worker/internal/tools"
)

func TestPaperWizardWidgetEmbedsKBsAndChapters(t *testing.T) {
	kbs := []wizardKB{
		{
			ID:   "kb-1",
			Name: "妇产科学",
			Mode: "standalone",
			Chapters: []wizardChapter{
				{ID: "kp-1", Name: "第一章 女性生殖系统"},
				{ID: "kp-2", Name: "第二章 妊娠生理"},
			},
		},
		{
			ID:       "kb-2",
			Name:     "内科学",
			Mode:     "exam",
			Chapters: []wizardChapter{{ID: "kp-3", Name: "心血管"}},
		},
	}
	html := paperWizardWidget(kbs)

	mustContain := []string{
		// step scaffolding
		"第 1/3 步", "第 2/3 步", "第 3/3 步",
		"全选",
		// A1-A4 type labels
		"A1", "A2", "A3", "A4", "单句型最佳选择题", "病例串型最佳选择题",
		// global difficulty radios
		"name=\"pwdiff\"",
		// the final structured command marker the widget posts back
		"【智能组卷·开始生成】", "pattern_tag", "保存到考试系统",
		// baked data must be present (ids + names)
		"kb-1", "kb-2", "kp-1", "kp-3", "妇产科学", "心血管",
		// interaction hooks
		"data-kb=", "data-kp=", "data-act=\"submit\"", "sendPrompt",
	}
	for _, want := range mustContain {
		if !strings.Contains(html, want) {
			t.Errorf("paperWizardWidget output missing %q", want)
		}
	}

	// The JSON data must be injected (placeholder fully replaced).
	if strings.Contains(html, "/*__PW_DATA__*/null") {
		t.Errorf("paperWizardWidget left the data placeholder unreplaced")
	}
}

func TestPaperWizardWidgetEmptyState(t *testing.T) {
	html := paperWizardWidget(nil)
	if !strings.Contains(html, "没有可用于组卷的课程资料知识库") {
		t.Errorf("empty wizard should explain there are no usable knowledge bases, got: %s", html)
	}
	if strings.Contains(html, "data-act=\"submit\"") {
		t.Errorf("empty wizard should not render the generate button")
	}
}

func TestWizardChaptersFromResultToleratesShapes(t *testing.T) {
	// provider returns []map[string]any with id + display_name aliases
	res := tools.ExecutionResult{ResultJSON: map[string]any{"items": []map[string]any{
		{"id": "a", "display_name": "甲"},
		{"id": "b", "name": "乙"},
		{"code": "no-id"}, // dropped: missing id
	}}}
	got := wizardChaptersFromResult(res)
	if len(got) != 2 {
		t.Fatalf("expected 2 chapters, got %d (%+v)", len(got), got)
	}
	if got[0].ID != "a" || got[0].Name != "甲" {
		t.Errorf("display_name alias not used: %+v", got[0])
	}
	if got[1].ID != "b" || got[1].Name != "乙" {
		t.Errorf("name field not used: %+v", got[1])
	}
}
