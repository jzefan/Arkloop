package telegrambot

import (
	"errors"
	"strings"
	"testing"
)

func TestIsTelegramEntityParseError(t *testing.T) {
	t.Parallel()
	if IsTelegramEntityParseError(nil) {
		t.Fatal("nil")
	}
	if !IsTelegramEntityParseError(errors.New(`Bad Request: can't parse entities`)) {
		t.Fatal("expected entities error")
	}
	if IsTelegramEntityParseError(errors.New("network")) {
		t.Fatal("unexpected")
	}
}

func TestStripTelegramHTMLToPlain(t *testing.T) {
	t.Parallel()
	s := StripTelegramHTMLToPlain(`a <b>b</b> c <a href="https://x">lbl</a>`)
	if !strings.Contains(s, "a") || !strings.Contains(s, "b") || !strings.Contains(s, "lbl") {
		t.Fatalf("got %q", s)
	}
}
