package formatter

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintStatusJSON(t *testing.T) {
	var buf bytes.Buffer
	err := PrintStatus(&buf, OutputJSON, StatusView{
		Host:        "http://127.0.0.1:19035",
		Connected:   true,
		UserID:      "u1",
		Username:    "qq",
		AccountID:   "a1",
		WorkEnabled: true,
	})
	if err != nil {
		t.Fatalf("PrintStatus: %v", err)
	}
	if !strings.Contains(buf.String(), `"connected":true`) || !strings.Contains(buf.String(), `"host":"http://127.0.0.1:19035"`) {
		t.Fatalf("unexpected json: %s", buf.String())
	}
}

func TestPrintModelsText(t *testing.T) {
	var buf bytes.Buffer
	err := PrintModels(&buf, OutputText, []ModelView{{
		Model:        "gpt-4.1",
		ProviderName: "OpenAI",
		IsDefault:    true,
		ShowInPicker: false,
		Tags:         []string{"chat", "default"},
	}})
	if err != nil {
		t.Fatalf("PrintModels: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "MODEL") || !strings.Contains(out, "gpt-4.1") || !strings.Contains(out, "true") {
		t.Fatalf("unexpected text: %s", out)
	}
}

func TestPrintPersonasText(t *testing.T) {
	var buf bytes.Buffer
	err := PrintPersonas(&buf, OutputText, []PersonaView{{
		PersonaKey:    "search",
		SelectorName:  "Search",
		DisplayName:   "Search",
		Model:         "gpt-4.1",
		ReasoningMode: "enabled",
	}})
	if err != nil {
		t.Fatalf("PrintPersonas: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "PERSONA_KEY") || !strings.Contains(out, "search") || !strings.Contains(out, "enabled") {
		t.Fatalf("unexpected text: %s", out)
	}
}

func TestPrintSessionsJSON(t *testing.T) {
	var buf bytes.Buffer
	err := PrintSessions(&buf, OutputJSON, []SessionView{{
		ID:          "t1",
		Title:       "Hello",
		Mode:        "chat",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-02T00:00:00Z",
		ActiveRunID: "r1",
		IsPrivate:   false,
	}})
	if err != nil {
		t.Fatalf("PrintSessions: %v", err)
	}
	if !strings.Contains(buf.String(), `"id":"t1"`) || !strings.Contains(buf.String(), `"active_run_id":"r1"`) {
		t.Fatalf("unexpected json: %s", buf.String())
	}
}
