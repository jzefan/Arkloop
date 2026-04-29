package accountapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"testing"
)

func TestFeishuURLVerificationChallenge(t *testing.T) {
	cfg := feishuChannelConfig{VerificationToken: "verify-token"}
	payload := feishuWebhookPayload{
		Type:      "url_verification",
		Token:     "verify-token",
		Challenge: "challenge-value",
	}
	challenge, ok, err := feishuURLVerificationChallenge(payload, cfg)
	if err != nil {
		t.Fatalf("url verification: %v", err)
	}
	if !ok {
		t.Fatal("expected url verification payload")
	}
	if challenge != "challenge-value" {
		t.Fatalf("challenge = %q", challenge)
	}
}

func TestFeishuDecodeWebhookPayloadRejectsBadSignature(t *testing.T) {
	cfg := feishuChannelConfig{EncryptKey: "encrypt-key"}
	header := nethttp.Header{}
	header.Set("x-lark-request-timestamp", "1700000000")
	header.Set("x-lark-request-nonce", "nonce")
	header.Set("x-lark-signature", "bad-signature")

	_, err := decodeFeishuWebhookPayload([]byte(`{"type":"event_callback"}`), cfg, header)
	if !errors.Is(err, errFeishuInvalidSignature) {
		t.Fatalf("err = %v, want invalid signature", err)
	}
}

func TestParseFeishuIncomingMessageAndGating(t *testing.T) {
	cfg := feishuChannelConfig{
		AppID:           "cli_x",
		AllowedUserIDs:  []string{"ou_allowed"},
		AllowedChatIDs:  []string{"oc_allowed"},
		BotOpenID:       "ou_bot",
		BotName:         "Arkloop",
		TriggerKeywords: []string{"ark"},
	}
	raw := []byte(`{
		"schema":"2.0",
		"header":{"event_type":"im.message.receive_v1","token":"token"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_allowed","user_id":"user_1","union_id":"union_1"}},
			"message":{
				"message_id":"om_1",
				"chat_id":"oc_allowed",
				"chat_type":"group",
				"message_type":"text",
				"content":"{\"text\":\"Hi Ark\"}",
				"mentions":[{"key":"@_user_1","id":{"open_id":"ou_bot"},"name":"Arkloop"}]
			}
		}
	}`)
	var payload feishuWebhookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	incoming, ok := parseFeishuIncomingMessage(payload, cfg, raw)
	if !ok {
		t.Fatal("expected incoming message")
	}
	if incoming.Text != "Hi Ark" {
		t.Fatalf("text = %q", incoming.Text)
	}
	if !incoming.MentionsBot {
		t.Fatal("expected bot mention")
	}
	if !incoming.MatchesKeyword {
		t.Fatal("expected keyword match")
	}
	if !incoming.ShouldCreateRun() {
		t.Fatal("expected dispatch trigger")
	}
	if !feishuInboundAllowed(cfg, incoming) {
		t.Fatal("expected allowed inbound")
	}
}

func TestDecryptFeishuPayload(t *testing.T) {
	plain := []byte(`{"type":"url_verification","challenge":"ok"}`)
	encrypted := encryptFeishuPayloadForTest(t, plain, "encrypt-key")
	got, err := decryptFeishuPayload(encrypted, "encrypt-key")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plain = %s", string(got))
	}
}

func encryptFeishuPayloadForTest(t *testing.T, plain []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	padded := pkcs7PadForTest(plain, aes.BlockSize)
	iv := []byte("1234567890abcdef")
	out := make([]byte, len(iv)+len(padded))
	copy(out, iv)
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(out[len(iv):], padded)
	return base64.StdEncoding.EncodeToString(out)
}

func pkcs7PadForTest(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}
