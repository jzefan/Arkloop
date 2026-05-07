package telegrambot

import (
	"strings"
	"testing"
)

func TestFormatAssistantMarkdownAsHTML_Bold(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("前置 **粗体** 后置")
	if !strings.Contains(s, "<b>粗体</b>") || !strings.Contains(s, "前置") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_EscapesThenBold(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML(`x **a & b** y`)
	if !strings.Contains(s, "&amp;") || !strings.Contains(s, "<b>a &amp; b</b>") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_LinkHTTPSOnly(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML(`[t](https://ex.com/a?x=1)`)
	if !strings.Contains(s, `<a href="https://ex.com/a?x=1">`) || !strings.Contains(s, "</a>") {
		t.Fatalf("got %q", s)
	}
	s2 := FormatAssistantMarkdownAsHTML(`[t](javascript:alert(1))`)
	if strings.Contains(s2, "<a href") {
		t.Fatalf("expected no link, got %q", s2)
	}
}

func TestFormatAssistantMarkdownAsHTML_InlineCode(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("run `go test` now")
	if !strings.Contains(s, "<code>go test</code>") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_Fenced(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("```\na < b\n```")
	if !strings.Contains(s, "<pre>") || !strings.Contains(s, "a &lt; b") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_FenceNoInlineMarkdown(t *testing.T) {
	t.Parallel()
	// 围栏须处于行首（CommonMark）；否则 ``` 不会开启代码块。
	s := FormatAssistantMarkdownAsHTML("x\n```\n**no**\n# h\n```\ny")
	if !strings.Contains(s, "<pre>") || strings.Contains(s, "<b>no</b>") || strings.Contains(s, "<b>h</b>") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_BoldAndLink(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML(`**a** [u](https://x.test/)`)
	if !strings.Contains(s, "<b>a</b>") || !strings.Contains(s, `href="https://x.test/"`) {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_ListWithItalic(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("- row *it* end")
	if !strings.Contains(s, "•") || !strings.Contains(s, "<i>it</i>") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_Blockquote(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("> q1\n> q2\nplain")
	if !strings.Contains(s, "<blockquote>") || !strings.Contains(s, "q1") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_Strike(t *testing.T) {
	t.Parallel()
	s := FormatAssistantMarkdownAsHTML("~~x~~")
	if !strings.Contains(s, "<s>") || !strings.Contains(s, "x") {
		t.Fatalf("got %q", s)
	}
}

func TestFormatAssistantMarkdownAsHTML_TableAsPre(t *testing.T) {
	t.Parallel()
	in := "| a | b |\n| --- | --- |\n| 1 | 2 |\n"
	s := FormatAssistantMarkdownAsHTML(in)
	if !strings.Contains(s, "<pre>") || (!strings.Contains(s, "a") && !strings.Contains(s, "1")) {
		t.Fatalf("got %q", s)
	}
}
