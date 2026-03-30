package accountapi

import (
	"encoding/json"
	"testing"
)

func TestTelegramMessageRepliesToBot_usesNumericBotID(t *testing.T) {
	otherBot := int64(999001)
	ourBot := int64(777002)
	raw := json.RawMessage(`{
		"message_id": 1,
		"date": 1,
		"text": "hi",
		"chat": {"id": 1, "type": "supergroup"},
		"from": {"id": 100, "is_bot": false, "first_name": "U"},
		"reply_to_message": {
			"message_id": 9,
			"date": 1,
			"text": "old",
			"chat": {"id": 1, "type": "supergroup"},
			"from": {"id": ` + jsonInt(otherBot) + `, "is_bot": true, "first_name": "Other"}
		}
	}`)
	var msg telegramMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if telegramMessageRepliesToBot(&msg, ourBot) {
		t.Fatal("reply to another bot should not count")
	}
	msg.ReplyToMessage.From.ID = ourBot
	if !telegramMessageRepliesToBot(&msg, ourBot) {
		t.Fatal("reply to this bot should count")
	}
}

func TestTelegramMessageRepliesToBot_requiresBotIDWhenUnset(t *testing.T) {
	raw := json.RawMessage(`{
		"message_id": 1,
		"date": 1,
		"text": "hi",
		"chat": {"id": 1, "type": "supergroup"},
		"from": {"id": 100, "is_bot": false},
		"reply_to_message": {
			"message_id": 9,
			"date": 1,
			"text": "old",
			"chat": {"id": 1, "type": "supergroup"},
			"from": {"id": 50, "is_bot": true}
		}
	}`)
	var msg telegramMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if telegramMessageRepliesToBot(&msg, 0) {
		t.Fatal("reply should not count when telegram_bot_user_id is missing")
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestTelegramUserAllowed_emptyAllowlistOpens(t *testing.T) {
	t.Helper()
	if !telegramUserAllowed(nil, "5957030043") {
		t.Fatal("nil allowlist should allow any user id")
	}
	if !telegramUserAllowed([]string{}, "123") {
		t.Fatal("empty allowlist should allow")
	}
	if telegramUserAllowed([]string{"0"}, "5957030043") {
		t.Fatal("\"0\" is a literal Telegram user id, not a wildcard")
	}
	if !telegramUserAllowed([]string{"5957030043"}, "5957030043") {
		t.Fatal("explicit id should match")
	}
}
