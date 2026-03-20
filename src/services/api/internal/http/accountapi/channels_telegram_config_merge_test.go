package accountapi

import (
	"encoding/json"
	"testing"

	"arkloop/services/shared/telegrambot"
)

func TestMergeTelegramChannelConfigJSONPatch_preservesBotFieldsWhenPatchOmitsThem(t *testing.T) {
	t.Helper()
	existing := json.RawMessage(`{
	  "allowed_user_ids": ["1"],
	  "telegram_bot_user_id": 4242,
	  "bot_username": "my_bot",
	  "default_model": "old^m"
	}`)
	patch := json.RawMessage(`{"allowed_user_ids":["2"],"default_model":"new^m"}`)
	out, err := mergeTelegramChannelConfigJSONPatch(existing, patch)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["telegram_bot_user_id"] != float64(4242) {
		t.Fatalf("telegram_bot_user_id: %v", got["telegram_bot_user_id"])
	}
	if got["bot_username"] != "my_bot" {
		t.Fatalf("bot_username: %v", got["bot_username"])
	}
	if got["default_model"] != "new^m" {
		t.Fatalf("default_model: %v", got["default_model"])
	}
}

func TestMergeTelegramBotProfileFromGetMe_fillsMissingIDAndUsername(t *testing.T) {
	t.Helper()
	raw := json.RawMessage(`{"allowed_user_ids":["1"]}`)
	u := "chiffon_arkloop_bot"
	info := &telegrambot.BotInfo{ID: 9001, Username: &u}
	out, changed, err := mergeTelegramBotProfileFromGetMe(raw, info)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed")
	}
	cfg, err := resolveTelegramConfig("telegram", out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramBotUserID != 9001 {
		t.Fatalf("id: %d", cfg.TelegramBotUserID)
	}
	if cfg.BotUsername != "chiffon_arkloop_bot" {
		t.Fatalf("username: %q", cfg.BotUsername)
	}
}

func TestMergeTelegramBotProfileFromGetMe_fillsOnlyMissingUsername(t *testing.T) {
	t.Helper()
	raw := json.RawMessage(`{"allowed_user_ids":["1"],"telegram_bot_user_id":42}`)
	u := "only_user"
	info := &telegrambot.BotInfo{ID: 999, Username: &u}
	out, changed, err := mergeTelegramBotProfileFromGetMe(raw, info)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed")
	}
	cfg, err := resolveTelegramConfig("telegram", out)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TelegramBotUserID != 42 {
		t.Fatalf("id should stay: %d", cfg.TelegramBotUserID)
	}
	if cfg.BotUsername != "only_user" {
		t.Fatalf("username: %q", cfg.BotUsername)
	}
}

func TestMergeTelegramBotProfileFromGetMe_noOverwriteWhenPresent(t *testing.T) {
	t.Helper()
	raw := json.RawMessage(`{"allowed_user_ids":["1"],"telegram_bot_user_id":1,"bot_username":"keep_me"}`)
	u := "other"
	info := &telegrambot.BotInfo{ID: 999, Username: &u}
	out, changed, err := mergeTelegramBotProfileFromGetMe(raw, info)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected unchanged")
	}
	if string(out) != string(raw) {
		t.Fatalf("raw mutated: %s", string(out))
	}
}
