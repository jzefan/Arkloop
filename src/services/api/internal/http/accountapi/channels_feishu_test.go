package accountapi

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFeishuWebhookSignature(t *testing.T) {
	raw := []byte(`{"schema":"2.0"}`)
	key := "encrypt-key"
	req := httptest.NewRequest("POST", "/v1/channels/feishu/id/webhook", bytes.NewReader(raw))
	now := time.Unix(1700000000, 0)
	signFeishuRequestForTest(req, raw, key, now)

	if !verifyFeishuSignatureAt(req, raw, key, now) {
		t.Fatal("expected signature to match")
	}
	if verifyFeishuSignatureAt(req, raw, key, now.Add(feishuSignatureTolerance+time.Second)) {
		t.Fatal("expected stale signature to fail")
	}
	req.Header.Set("X-Lark-Signature", "bad")
	if verifyFeishuSignatureAt(req, raw, key, now) {
		t.Fatal("expected bad signature to fail")
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
		t.Fatalf("plain = %s, want %s", got, plain)
	}
}

func TestDecodeFeishuWebhookPayloadCanReadChallengeWithoutSignature(t *testing.T) {
	plain := []byte(`{"type":"url_verification","challenge":"ok"}`)
	raw := []byte(`{"encrypt":"` + encryptFeishuPayloadForTest(t, plain, "encrypt-key") + `"}`)
	req := httptest.NewRequest("POST", "/v1/channels/feishu/id/webhook", bytes.NewReader(raw))
	got, err := decodeFeishuWebhookPayload(req, raw, "encrypt-key", false)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plain = %s, want %s", got, plain)
	}
}

func TestDecodeFeishuWebhookPayloadRequiresSignatureForEvents(t *testing.T) {
	plain := []byte(`{"schema":"2.0","header":{"event_type":"im.message.receive_v1"}}`)
	raw := []byte(`{"encrypt":"` + encryptFeishuPayloadForTest(t, plain, "encrypt-key") + `"}`)
	req := httptest.NewRequest("POST", "/v1/channels/feishu/id/webhook", bytes.NewReader(raw))
	if _, err := decodeFeishuWebhookPayload(req, raw, "encrypt-key", true); err == nil {
		t.Fatal("expected missing signature to fail")
	}
	signFeishuRequestForTest(req, raw, "encrypt-key", time.Now())
	got, err := decodeFeishuWebhookPayload(req, raw, "encrypt-key", true)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("plain = %s, want %s", got, plain)
	}
}

func TestFeishuWebhookAuthRequiresTokenAndEncryptKey(t *testing.T) {
	if feishuWebhookAuthConfigured(feishuChannelSecret{VerificationToken: "token"}) {
		t.Fatal("expected token without encrypt_key to fail")
	}
	if feishuWebhookAuthConfigured(feishuChannelSecret{EncryptKey: "key"}) {
		t.Fatal("expected encrypt_key without token to fail")
	}
	if !feishuWebhookAuthConfigured(feishuChannelSecret{VerificationToken: "token", EncryptKey: "key"}) {
		t.Fatal("expected token and encrypt_key to pass")
	}
}

func TestFeishuURLVerificationToken(t *testing.T) {
	envelope := feishuWebhookEnvelope{
		Type:      "url_verification",
		Token:     "verify-token",
		Challenge: "challenge-value",
	}
	if !feishuTokenMatches(envelope, "verify-token") {
		t.Fatal("expected token match")
	}
	if feishuTokenMatches(envelope, "other") {
		t.Fatal("expected token mismatch")
	}
}

func TestNormalizeFeishuConfigStripsWebhookSecrets(t *testing.T) {
	raw := json.RawMessage(`{"app_id":"cli_app","encrypt_key":"enc","verification_token":"verify","allowed_user_ids":["ou_1"]}`)
	normalized, cfg, err := normalizeFeishuChannelConfig(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.EncryptKey != "" || cfg.VerificationToken != "" {
		t.Fatalf("expected secrets stripped from config, got %#v", cfg)
	}
	if strings.Contains(string(normalized), "encrypt_key") || strings.Contains(string(normalized), "verification_token") {
		t.Fatalf("normalized config leaked secrets: %s", normalized)
	}
	sanitized := sanitizeFeishuConfigForResponse(json.RawMessage(`{"encrypt_key":"enc","verification_token":"verify","other":1}`))
	if strings.Contains(string(sanitized), "encrypt_key") || strings.Contains(string(sanitized), "verification_token") {
		t.Fatalf("response config leaked secrets: %s", sanitized)
	}
	patch, err := feishuSecretPatchFromConfig(raw)
	if err != nil {
		t.Fatalf("secret patch: %v", err)
	}
	secret := applyFeishuSecretPatch(feishuChannelSecret{AppSecret: "app-secret"}, patch)
	if secret.EncryptKey != "enc" || secret.VerificationToken != "verify" {
		t.Fatalf("secret patch not applied: %#v", secret)
	}
}

func TestNormalizeFeishuConfigDerivesAllowAllUsers(t *testing.T) {
	raw := json.RawMessage(`{"app_id":"cli_app","allow_all_users":true,"allowed_user_ids":["ou_1"]}`)
	normalized, cfg, err := normalizeFeishuChannelConfig(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.AllowAllUsers {
		t.Fatal("expected allow_all_users to be false when allowlist is present")
	}
	if strings.Contains(string(normalized), `"allow_all_users":true`) {
		t.Fatalf("normalized config kept stale allow_all_users: %s", normalized)
	}

	_, openCfg, err := normalizeFeishuChannelConfig(json.RawMessage(`{"app_id":"cli_app"}`))
	if err != nil {
		t.Fatalf("normalize open config: %v", err)
	}
	if !openCfg.AllowAllUsers {
		t.Fatal("expected allow_all_users to be true without allowlists")
	}
}

func TestNormalizeFeishuIncomingTextMentionAndKeyword(t *testing.T) {
	event := feishuMessageEvent{
		Sender: feishuSender{SenderID: feishuSenderID{OpenID: "ou_sender"}, SenderType: "user"},
		Message: feishuMessage{
			MessageID:   "om_1",
			ChatID:      "oc_1",
			ChatType:    "group",
			MessageType: "text",
			Content:     `{"text":"Arkloop 请总结"}`,
			Mentions: []feishuMention{{
				Name: "Arkloop",
			}},
		},
	}
	event.Message.Mentions[0].ID.OpenID = "ou_bot"
	cfg := feishuChannelConfig{
		BotOpenID:       "ou_bot",
		TriggerKeywords: []string{"arkloop"},
		AllowedUserIDs:  []string{"ou_sender"},
	}
	incoming, err := normalizeFeishuIncoming(event, cfg)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if incoming.ConversationType != "group" || !incoming.MentionsBot || incoming.Text != "Arkloop 请总结" {
		t.Fatalf("incoming: %#v", incoming)
	}
	if !feishuIncomingAllowed(cfg, incoming) {
		t.Fatal("expected sender to be allowed")
	}
	if !feishuMessageMatchesKeyword(incoming.Text, cfg.TriggerKeywords) {
		t.Fatal("expected keyword match")
	}
}

func TestFeishuIncomingFromSelf(t *testing.T) {
	cfg := feishuChannelConfig{BotOpenID: "ou_bot", BotUserID: "u_bot"}
	if !feishuIncomingFromSelf(cfg, feishuIncomingMessage{SenderType: "bot", SenderOpenID: "ou_any"}) {
		t.Fatal("expected bot sender_type to be ignored")
	}
	if !feishuIncomingFromSelf(cfg, feishuIncomingMessage{SenderOpenID: "ou_bot"}) {
		t.Fatal("expected matching bot open_id to be ignored")
	}
	if !feishuIncomingFromSelf(cfg, feishuIncomingMessage{SenderUserID: "u_bot"}) {
		t.Fatal("expected matching bot user_id to be ignored")
	}
	if feishuIncomingFromSelf(cfg, feishuIncomingMessage{SenderType: "user", SenderOpenID: "ou_user"}) {
		t.Fatal("expected user event to be accepted")
	}
}

func TestExtractFeishuPostText(t *testing.T) {
	raw := map[string]any{
		"title": "标题",
		"content": [][]map[string]string{{
			{"tag": "text", "text": "第一段"},
			{"tag": "at", "user_name": "用户"},
		}},
	}
	data, _ := json.Marshal(raw)
	got := extractFeishuMessageText("post", string(data))
	if !strings.Contains(got, "标题") || !strings.Contains(got, "第一段@用户") {
		t.Fatalf("unexpected post text: %q", got)
	}
}

func encryptFeishuPayloadForTest(t *testing.T, plain []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte("1234567890abcdef")
	padded := pkcs7PadForTest(plain, aes.BlockSize)
	out := make([]byte, len(iv)+len(padded))
	copy(out, iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out[len(iv):], padded)
	return base64.StdEncoding.EncodeToString(out)
}

func signFeishuRequestForTest(req *http.Request, raw []byte, key string, now time.Time) {
	req.Header.Set("X-Lark-Request-Timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("X-Lark-Request-Nonce", "nonce")
	sum := sha256.Sum256([]byte(strconv.FormatInt(now.Unix(), 10) + "nonce" + key + string(raw)))
	req.Header.Set("X-Lark-Signature", hex.EncodeToString(sum[:]))
}

func pkcs7PadForTest(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	out := append([]byte{}, data...)
	for i := 0; i < pad; i++ {
		out = append(out, byte(pad))
	}
	return out
}
