package accountapi

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	"arkloop/services/api/internal/data"
	httpkit "arkloop/services/api/internal/http/httpkit"
	"arkloop/services/api/internal/observability"
	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/pgnotify"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const feishuRemoteRequestTimeout = 10 * time.Second

var (
	errFeishuInvalidSignature = errors.New("invalid feishu signature")
	errFeishuMissingEncryptKey = errors.New("feishu encrypt_key required")
)

type feishuChannelConfig struct {
	AppID             string   `json:"app_id"`
	Domain            string   `json:"domain,omitempty"`
	EncryptKey        string   `json:"encrypt_key,omitempty"`
	VerificationToken string   `json:"verification_token,omitempty"`
	AllowedUserIDs    []string `json:"allowed_user_ids,omitempty"`
	AllowedChatIDs    []string `json:"allowed_chat_ids,omitempty"`
	AllowAllUsers     bool     `json:"allow_all_users,omitempty"`
	DefaultModel      string   `json:"default_model,omitempty"`
	BotOpenID         string   `json:"bot_open_id,omitempty"`
	BotUserID         string   `json:"bot_user_id,omitempty"`
	BotName           string   `json:"bot_name,omitempty"`
	TriggerKeywords   []string `json:"trigger_keywords,omitempty"`
}

type feishuWebhookPayload struct {
	Type      string              `json:"type,omitempty"`
	Token     string              `json:"token,omitempty"`
	Challenge string              `json:"challenge,omitempty"`
	Header    feishuEventHeader   `json:"header,omitempty"`
	Event     feishuEventEnvelope  `json:"event,omitempty"`
	Schema    string              `json:"schema,omitempty"`
}

type feishuEventHeader struct {
	EventID    string `json:"event_id,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	CreateTime string `json:"create_time,omitempty"`
	Token      string `json:"token,omitempty"`
	AppID      string `json:"app_id,omitempty"`
	TenantKey  string `json:"tenant_key,omitempty"`
}

type feishuEventEnvelope struct {
	Sender  feishuEventSender  `json:"sender,omitempty"`
	Message feishuEventMessage `json:"message,omitempty"`
}

type feishuEventSender struct {
	SenderID   feishuSubjectID `json:"sender_id,omitempty"`
	SenderType string          `json:"sender_type,omitempty"`
	TenantKey  string          `json:"tenant_key,omitempty"`
}

type feishuSubjectID struct {
	OpenID  string `json:"open_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	UnionID string `json:"union_id,omitempty"`
}

type feishuEventMessage struct {
	MessageID   string          `json:"message_id,omitempty"`
	RootID      string          `json:"root_id,omitempty"`
	ParentID    string          `json:"parent_id,omitempty"`
	CreateTime  string          `json:"create_time,omitempty"`
	ChatID      string          `json:"chat_id,omitempty"`
	ChatType    string          `json:"chat_type,omitempty"`
	MessageType string          `json:"message_type,omitempty"`
	Content     string          `json:"content,omitempty"`
	Mentions    []feishuMention `json:"mentions,omitempty"`
}

type feishuMention struct {
	Key       string          `json:"key,omitempty"`
	ID        feishuSubjectID `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	TenantKey string          `json:"tenant_key,omitempty"`
}

type feishuIncomingMessage struct {
	PlatformChatID   string
	PlatformMsgID    string
	SenderOpenID     string
	SenderUserID     string
	SenderUnionID    string
	SenderSubjectID  string
	ChatType         string
	MessageType      string
	Text             string
	MentionsBot      bool
	MentionsAll      bool
	MatchesKeyword   bool
	ConversationType string
	RawPayload       json.RawMessage
}

func (m feishuIncomingMessage) IsPrivate() bool {
	return m.ConversationType == "private"
}

func (m feishuIncomingMessage) ShouldCreateRun() bool {
	return m.IsPrivate() || m.MentionsBot || m.MentionsAll || m.MatchesKeyword
}

func normalizeFeishuChannelConfigJSON(raw json.RawMessage) (json.RawMessage, *feishuChannelConfig, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, nil, fmt.Errorf("config_json must be a valid JSON object")
	}
	if generic == nil {
		generic = map[string]any{}
	}
	var cfg feishuChannelConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, fmt.Errorf("config_json must be a valid JSON object")
	}
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	if cfg.AppID == "" {
		return nil, nil, fmt.Errorf("feishu config app_id must not be empty")
	}
	cfg.Domain = strings.TrimSpace(strings.ToLower(cfg.Domain))
	if cfg.Domain == "" {
		cfg.Domain = "feishu"
	}
	cfg.EncryptKey = strings.TrimSpace(cfg.EncryptKey)
	cfg.VerificationToken = strings.TrimSpace(cfg.VerificationToken)
	cfg.AllowedUserIDs = normalizeFeishuStringList(cfg.AllowedUserIDs, false)
	cfg.AllowedChatIDs = normalizeFeishuStringList(cfg.AllowedChatIDs, false)
	if len(cfg.AllowedUserIDs) == 0 && len(cfg.AllowedChatIDs) == 0 {
		cfg.AllowAllUsers = true
	}
	cfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	cfg.BotOpenID = strings.TrimSpace(cfg.BotOpenID)
	cfg.BotUserID = strings.TrimSpace(cfg.BotUserID)
	cfg.BotName = strings.TrimSpace(cfg.BotName)
	cfg.TriggerKeywords = normalizeFeishuStringList(cfg.TriggerKeywords, true)

	normalized, err := json.Marshal(cfg)
	if err != nil {
		return nil, nil, err
	}
	return normalized, &cfg, nil
}

func resolveFeishuChannelConfig(raw json.RawMessage) (feishuChannelConfig, error) {
	_, cfg, err := normalizeFeishuChannelConfigJSON(raw)
	if err != nil {
		return feishuChannelConfig{}, err
	}
	if cfg == nil {
		return feishuChannelConfig{}, nil
	}
	return *cfg, nil
}

func mergeFeishuChannelConfigJSONPatch(existing, patch json.RawMessage) (json.RawMessage, error) {
	if len(patch) == 0 {
		normalized, _, err := normalizeFeishuChannelConfigJSON(existing)
		return normalized, err
	}
	ex := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &ex); err != nil {
			return nil, fmt.Errorf("config_json must be a valid JSON object")
		}
	}
	if ex == nil {
		ex = map[string]any{}
	}
	patchMap := map[string]any{}
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return nil, fmt.Errorf("config_json must be a valid JSON object")
	}
	for k, v := range patchMap {
		ex[k] = v
	}
	merged, err := json.Marshal(ex)
	if err != nil {
		return nil, err
	}
	normalized, _, err := normalizeFeishuChannelConfigJSON(merged)
	return normalized, err
}

func normalizeFeishuStringList(values []string, lower bool) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			cleaned := strings.TrimSpace(item)
			if lower {
				cleaned = strings.ToLower(cleaned)
			}
			if cleaned == "" {
				continue
			}
			if _, ok := seen[cleaned]; ok {
				continue
			}
			seen[cleaned] = struct{}{}
			out = append(out, cleaned)
		}
	}
	return out
}

func mustValidateFeishuActivation(ctx context.Context, accountID uuid.UUID, personasRepo *data.PersonasRepository, personaID *uuid.UUID, configJSON json.RawMessage) (*data.Persona, string, feishuChannelConfig, error) {
	if personaID == nil || *personaID == uuid.Nil {
		return nil, "", feishuChannelConfig{}, fmt.Errorf("feishu channel requires persona_id before activation")
	}
	persona, err := personasRepo.GetByIDForAccount(ctx, accountID, *personaID)
	if err != nil {
		return nil, "", feishuChannelConfig{}, err
	}
	if persona == nil || !persona.IsActive {
		return nil, "", feishuChannelConfig{}, fmt.Errorf("persona not found or inactive")
	}
	if persona.ProjectID == nil || *persona.ProjectID == uuid.Nil {
		return nil, "", feishuChannelConfig{}, fmt.Errorf("feishu channel persona must belong to a project")
	}
	cfg, err := resolveFeishuChannelConfig(configJSON)
	if err != nil {
		return nil, "", feishuChannelConfig{}, err
	}
	return persona, buildPersonaRef(*persona), cfg, nil
}

func mergeFeishuBotProfile(raw json.RawMessage, info feishuBotInfo) (json.RawMessage, bool, error) {
	cfg, err := resolveFeishuChannelConfig(raw)
	if err != nil {
		return nil, false, err
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, false, fmt.Errorf("config_json must be a valid JSON object")
	}
	if generic == nil {
		generic = map[string]any{}
	}
	changed := false
	if cfg.BotOpenID == "" && strings.TrimSpace(info.OpenID) != "" {
		generic["bot_open_id"] = strings.TrimSpace(info.OpenID)
		changed = true
	}
	if cfg.BotUserID == "" && strings.TrimSpace(info.UserID) != "" {
		generic["bot_user_id"] = strings.TrimSpace(info.UserID)
		changed = true
	}
	if cfg.BotName == "" && strings.TrimSpace(info.Name) != "" {
		generic["bot_name"] = strings.TrimSpace(info.Name)
		changed = true
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(generic)
	if err != nil {
		return nil, false, err
	}
	normalized, _, err := normalizeFeishuChannelConfigJSON(out)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

type feishuBotInfo struct {
	OpenID string
	UserID string
	Name   string
}

func verifyFeishuChannelBotInfo(ctx context.Context, cfg feishuChannelConfig, appSecret string) (feishuBotInfo, error) {
	baseURL, err := feishuOpenAPIBaseURL(cfg.Domain)
	if err != nil {
		return feishuBotInfo{}, err
	}
	client := nethttp.DefaultClient
	token, err := fetchFeishuTenantAccessToken(ctx, client, baseURL, cfg.AppID, appSecret)
	if err != nil {
		return feishuBotInfo{}, err
	}
	return fetchFeishuBotInfo(ctx, client, baseURL, token)
}

func feishuOpenAPIBaseURL(domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	switch domain {
	case "", "feishu":
		return "https://open.feishu.cn", nil
	case "larksuite", "lark":
		return "https://open.larksuite.com", nil
	default:
		u, err := url.Parse(domain)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", fmt.Errorf("invalid feishu domain")
		}
		return strings.TrimRight(u.String(), "/"), nil
	}
}

func fetchFeishuTenantAccessToken(ctx context.Context, client *nethttp.Client, baseURL, appID, appSecret string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     strings.TrimSpace(appID),
		"app_secret": strings.TrimSpace(appSecret),
	})
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodPost, baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("feishu http %d", resp.StatusCode)
	}
	var envelope struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	if envelope.Code != 0 {
		return "", fmt.Errorf("feishu api error: code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	if strings.TrimSpace(envelope.TenantAccessToken) == "" {
		return "", fmt.Errorf("feishu tenant access token empty")
	}
	return strings.TrimSpace(envelope.TenantAccessToken), nil
}

func fetchFeishuBotInfo(ctx context.Context, client *nethttp.Client, baseURL, token string) (feishuBotInfo, error) {
	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, baseURL+"/open-apis/bot/v3/info", nil)
	if err != nil {
		return feishuBotInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	resp, err := client.Do(req)
	if err != nil {
		return feishuBotInfo{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return feishuBotInfo{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return feishuBotInfo{}, fmt.Errorf("feishu http %d", resp.StatusCode)
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Bot  struct {
			OpenID  string `json:"open_id"`
			UserID  string `json:"user_id"`
			Name    string `json:"app_name"`
			BotName string `json:"bot_name"`
		} `json:"bot"`
		Data struct {
			Bot struct {
				OpenID  string `json:"open_id"`
				UserID  string `json:"user_id"`
				Name    string `json:"app_name"`
				BotName string `json:"bot_name"`
			} `json:"bot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return feishuBotInfo{}, err
	}
	if envelope.Code != 0 {
		return feishuBotInfo{}, fmt.Errorf("feishu api error: code=%d msg=%s", envelope.Code, envelope.Msg)
	}
	info := feishuBotInfo{
		OpenID: firstNonEmptySelector(envelope.Bot.OpenID, envelope.Data.Bot.OpenID),
		UserID: firstNonEmptySelector(envelope.Bot.UserID, envelope.Data.Bot.UserID),
		Name:   firstNonEmptySelector(envelope.Bot.Name, envelope.Bot.BotName, envelope.Data.Bot.Name, envelope.Data.Bot.BotName),
	}
	if info.OpenID == "" && info.UserID == "" {
		return feishuBotInfo{}, fmt.Errorf("feishu bot info empty")
	}
	return info, nil
}

type feishuConnector struct {
	channelsRepo            *data.ChannelsRepository
	channelIdentitiesRepo   *data.ChannelIdentitiesRepository
	channelDMThreadsRepo    *data.ChannelDMThreadsRepository
	channelGroupThreadsRepo *data.ChannelGroupThreadsRepository
	channelReceiptsRepo     *data.ChannelMessageReceiptsRepository
	channelLedgerRepo       *data.ChannelMessageLedgerRepository
	personasRepo            *data.PersonasRepository
	threadRepo              *data.ThreadRepository
	messageRepo             *data.MessageRepository
	runEventRepo            *data.RunEventRepository
	jobRepo                 *data.JobRepository
	pool                    data.DB
	inputNotify             func(ctx context.Context, runID uuid.UUID)
}

func feishuWebhookEntry(
	channelsRepo *data.ChannelsRepository,
	channelIdentitiesRepo *data.ChannelIdentitiesRepository,
	channelDMThreadsRepo *data.ChannelDMThreadsRepository,
	channelGroupThreadsRepo *data.ChannelGroupThreadsRepository,
	channelReceiptsRepo *data.ChannelMessageReceiptsRepository,
	secretsRepo *data.SecretsRepository,
	personasRepo *data.PersonasRepository,
	threadRepo *data.ThreadRepository,
	messageRepo *data.MessageRepository,
	runEventRepo *data.RunEventRepository,
	jobRepo *data.JobRepository,
	pool data.DB,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	var channelLedgerRepo *data.ChannelMessageLedgerRepository
	if pool != nil {
		repo, err := data.NewChannelMessageLedgerRepository(pool)
		if err != nil {
			panic(err)
		}
		channelLedgerRepo = repo
	}
	connector := feishuConnector{
		channelsRepo:            channelsRepo,
		channelIdentitiesRepo:   channelIdentitiesRepo,
		channelDMThreadsRepo:    channelDMThreadsRepo,
		channelGroupThreadsRepo: channelGroupThreadsRepo,
		channelReceiptsRepo:     channelReceiptsRepo,
		channelLedgerRepo:       channelLedgerRepo,
		personasRepo:            personasRepo,
		threadRepo:              threadRepo,
		messageRepo:             messageRepo,
		runEventRepo:            runEventRepo,
		jobRepo:                 jobRepo,
		pool:                    pool,
		inputNotify: func(ctx context.Context, runID uuid.UUID) {
			if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgnotify.ChannelRunInput, runID.String()); err != nil {
				slog.Warn("feishu_active_run_notify_failed", "run_id", runID.String(), "error", err)
			}
		},
	}
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		if r.Method != nethttp.MethodPost {
			httpkit.WriteMethodNotAllowed(w, r)
			return
		}
		if channelsRepo == nil || channelIdentitiesRepo == nil || channelDMThreadsRepo == nil || channelGroupThreadsRepo == nil ||
			channelReceiptsRepo == nil || secretsRepo == nil || personasRepo == nil || threadRepo == nil || messageRepo == nil ||
			runEventRepo == nil || jobRepo == nil || pool == nil {
			httpkit.WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}
		channelID, ok := parseFeishuWebhookChannelID(r.URL.Path)
		if !ok {
			httpkit.WriteNotFound(w, r)
			return
		}
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			httpkit.WriteError(w, nethttp.StatusBadRequest, "validation.error", "invalid feishu payload", traceID, nil)
			return
		}
		ch, err := channelsRepo.GetByID(r.Context(), channelID)
		if err != nil {
			httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if ch == nil || ch.ChannelType != "feishu" {
			httpkit.WriteNotFound(w, r)
			return
		}
		if !ch.IsActive {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}
		cfg, err := resolveFeishuChannelConfig(ch.ConfigJSON)
		if err != nil {
			httpkit.WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", err.Error(), traceID, nil)
			return
		}
		payloadBytes, err := decodeFeishuWebhookPayload(rawBody, cfg, r.Header)
		if err != nil {
			status := nethttp.StatusBadRequest
			if errors.Is(err, errFeishuInvalidSignature) || errors.Is(err, errFeishuMissingEncryptKey) {
				status = nethttp.StatusUnauthorized
			}
			httpkit.WriteError(w, status, "channels.invalid_signature", "invalid feishu signature", traceID, nil)
			return
		}
		var payload feishuWebhookPayload
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			httpkit.WriteError(w, nethttp.StatusBadRequest, "validation.error", "invalid feishu payload", traceID, nil)
			return
		}
		if challenge, ok, err := feishuURLVerificationChallenge(payload, cfg); ok || err != nil {
			if err != nil {
				httpkit.WriteError(w, nethttp.StatusUnauthorized, "channels.invalid_token", "invalid feishu token", traceID, nil)
				return
			}
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]string{"challenge": challenge})
			return
		}
		if !feishuEventTokenValid(payload, cfg) {
			httpkit.WriteError(w, nethttp.StatusUnauthorized, "channels.invalid_token", "invalid feishu token", traceID, nil)
			return
		}
		if payload.Header.EventType != "im.message.receive_v1" {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}
		incoming, ok := parseFeishuIncomingMessage(payload, cfg, payloadBytes)
		if !ok || !feishuInboundAllowed(cfg, incoming) || !incoming.ShouldCreateRun() {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}
		if err := connector.HandleIncoming(r.Context(), traceID, *ch, cfg, incoming); err != nil {
			httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
	}
}

func parseFeishuWebhookChannelID(path string) (uuid.UUID, bool) {
	tail := strings.TrimPrefix(path, "/v1/channels/feishu/")
	tail = strings.TrimSuffix(tail, "/webhook")
	tail = strings.Trim(tail, "/")
	if tail == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(tail)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func decodeFeishuWebhookPayload(rawBody []byte, cfg feishuChannelConfig, header nethttp.Header) ([]byte, error) {
	var encrypted struct {
		Encrypt string `json:"encrypt"`
	}
	_ = json.Unmarshal(rawBody, &encrypted)
	hasEncrypt := strings.TrimSpace(encrypted.Encrypt) != ""
	if strings.TrimSpace(cfg.EncryptKey) != "" {
		if !verifyFeishuSignature(header.Get("x-lark-request-timestamp"), header.Get("x-lark-request-nonce"), header.Get("x-lark-signature"), cfg.EncryptKey, rawBody) {
			return nil, errFeishuInvalidSignature
		}
	}
	if !hasEncrypt {
		return rawBody, nil
	}
	if strings.TrimSpace(cfg.EncryptKey) == "" {
		return nil, errFeishuMissingEncryptKey
	}
	return decryptFeishuPayload(encrypted.Encrypt, cfg.EncryptKey)
}

func verifyFeishuSignature(timestamp, nonce, signature, encryptKey string, rawBody []byte) bool {
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)
	signature = strings.TrimSpace(strings.ToLower(signature))
	if timestamp == "" || nonce == "" || signature == "" || strings.TrimSpace(encryptKey) == "" {
		return false
	}
	sum := sha256.Sum256([]byte(timestamp + nonce + strings.TrimSpace(encryptKey) + string(rawBody)))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

func decryptFeishuPayload(encrypted, encryptKey string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encrypted))
	if err != nil {
		return nil, err
	}
	if len(ciphertext) <= aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid feishu encrypted payload")
	}
	key := sha256.Sum256([]byte(strings.TrimSpace(encryptKey)))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ciphertext)-aes.BlockSize)
	mode := cipher.NewCBCDecrypter(block, ciphertext[:aes.BlockSize])
	mode.CryptBlocks(plain, ciphertext[aes.BlockSize:])
	return pkcs7Unpad(plain, aes.BlockSize)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid pkcs7 payload")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	for _, b := range data[len(data)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("invalid pkcs7 padding")
		}
	}
	return data[:len(data)-padding], nil
}

func feishuURLVerificationChallenge(payload feishuWebhookPayload, cfg feishuChannelConfig) (string, bool, error) {
	if strings.TrimSpace(payload.Type) != "url_verification" {
		return "", false, nil
	}
	if strings.TrimSpace(cfg.VerificationToken) != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(payload.Token)), []byte(strings.TrimSpace(cfg.VerificationToken))) != 1 {
		return "", true, fmt.Errorf("invalid feishu token")
	}
	return strings.TrimSpace(payload.Challenge), true, nil
}

func feishuEventTokenValid(payload feishuWebhookPayload, cfg feishuChannelConfig) bool {
	token := firstNonEmptySelector(payload.Header.Token, payload.Token)
	if strings.TrimSpace(cfg.VerificationToken) == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(strings.TrimSpace(cfg.VerificationToken))) == 1
}

func parseFeishuIncomingMessage(payload feishuWebhookPayload, cfg feishuChannelConfig, raw []byte) (feishuIncomingMessage, bool) {
	msg := payload.Event.Message
	text, ok := feishuExtractMessageText(msg.MessageType, msg.Content)
	if !ok {
		return feishuIncomingMessage{}, false
	}
	chatType := strings.TrimSpace(strings.ToLower(msg.ChatType))
	conversationType := "group"
	if chatType == "p2p" || chatType == "private" {
		conversationType = "private"
	}
	incoming := feishuIncomingMessage{
		PlatformChatID:   strings.TrimSpace(msg.ChatID),
		PlatformMsgID:    strings.TrimSpace(msg.MessageID),
		SenderOpenID:     strings.TrimSpace(payload.Event.Sender.SenderID.OpenID),
		SenderUserID:     strings.TrimSpace(payload.Event.Sender.SenderID.UserID),
		SenderUnionID:    strings.TrimSpace(payload.Event.Sender.SenderID.UnionID),
		ChatType:         chatType,
		MessageType:      strings.TrimSpace(strings.ToLower(msg.MessageType)),
		Text:             strings.TrimSpace(text),
		ConversationType: conversationType,
		RawPayload:       append(json.RawMessage(nil), raw...),
	}
	incoming.SenderSubjectID = firstNonEmptySelector(incoming.SenderOpenID, incoming.SenderUserID, incoming.SenderUnionID)
	incoming.MentionsBot, incoming.MentionsAll = feishuMentionsTargetBot(msg.Mentions, cfg)
	incoming.MatchesKeyword = feishuMessageMatchesKeyword(incoming.Text, cfg.TriggerKeywords)
	return incoming, incoming.PlatformChatID != "" && incoming.PlatformMsgID != "" && incoming.SenderSubjectID != "" && incoming.Text != ""
}

func feishuExtractMessageText(messageType, content string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(messageType)) {
	case "text":
		var body struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return "", false
		}
		return strings.TrimSpace(body.Text), strings.TrimSpace(body.Text) != ""
	case "post":
		var body struct {
			Title   string             `json:"title"`
			Content [][]feishuPostItem `json:"content"`
		}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return "", false
		}
		var lines []string
		if title := strings.TrimSpace(body.Title); title != "" {
			lines = append(lines, title)
		}
		for _, row := range body.Content {
			var parts []string
			for _, item := range row {
				if text := strings.TrimSpace(firstNonEmptySelector(item.Text, item.Name)); text != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				lines = append(lines, strings.Join(parts, ""))
			}
		}
		text := strings.TrimSpace(strings.Join(lines, "\n"))
		return text, text != ""
	default:
		return "", false
	}
}

type feishuPostItem struct {
	Tag  string `json:"tag,omitempty"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

func feishuMentionsTargetBot(mentions []feishuMention, cfg feishuChannelConfig) (bool, bool) {
	for _, mention := range mentions {
		if feishuMentionIsAll(mention) {
			return false, true
		}
		if target := strings.TrimSpace(cfg.BotOpenID); target != "" && strings.TrimSpace(mention.ID.OpenID) == target {
			return true, false
		}
		if target := strings.TrimSpace(cfg.BotUserID); target != "" && strings.TrimSpace(mention.ID.UserID) == target {
			return true, false
		}
		if name := strings.TrimSpace(cfg.BotName); name != "" && strings.TrimSpace(mention.Name) == name {
			return true, false
		}
	}
	return false, false
}

func feishuMentionIsAll(mention feishuMention) bool {
	key := strings.ToLower(strings.TrimSpace(mention.Key))
	name := strings.ToLower(strings.TrimSpace(mention.Name))
	return key == "@all" || key == "all" || name == "all" || name == "所有人" ||
		strings.EqualFold(strings.TrimSpace(mention.ID.OpenID), "all") ||
		strings.EqualFold(strings.TrimSpace(mention.ID.UserID), "all")
}

func feishuMessageMatchesKeyword(text string, keywords []string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	for _, kw := range keywords {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw != "" && strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func feishuInboundAllowed(cfg feishuChannelConfig, incoming feishuIncomingMessage) bool {
	if len(cfg.AllowedChatIDs) > 0 && !containsString(cfg.AllowedChatIDs, incoming.PlatformChatID) {
		return false
	}
	if cfg.AllowAllUsers {
		return true
	}
	if containsString(cfg.AllowedUserIDs, incoming.SenderOpenID) ||
		containsString(cfg.AllowedUserIDs, incoming.SenderUserID) ||
		containsString(cfg.AllowedUserIDs, incoming.SenderUnionID) {
		return true
	}
	if len(cfg.AllowedChatIDs) > 0 && containsString(cfg.AllowedChatIDs, incoming.PlatformChatID) {
		return true
	}
	return false
}

func (c feishuConnector) HandleIncoming(ctx context.Context, traceID string, ch data.Channel, cfg feishuChannelConfig, incoming feishuIncomingMessage) error {
	persona, personaRef, err := c.resolveFeishuPersona(ctx, ch)
	if err != nil {
		return err
	}
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	commitTx := func() error {
		return tx.Commit(ctx)
	}
	received, err := c.channelReceiptsRepo.WithTx(tx).Record(ctx, ch.ID, incoming.PlatformChatID, incoming.PlatformMsgID)
	if err != nil {
		return err
	}
	if !received {
		return commitTx()
	}
	identity, err := c.channelIdentitiesRepo.WithTx(tx).Upsert(ctx, "feishu", incoming.SenderSubjectID, nil, nil, feishuIdentityMetadata(incoming))
	if err != nil {
		return err
	}
	if !incoming.IsPrivate() {
		if _, err := c.channelIdentitiesRepo.WithTx(tx).Upsert(ctx, "feishu", incoming.PlatformChatID, nil, nil, nil); err != nil {
			return err
		}
	}
	if c.channelLedgerRepo != nil {
		ledgerMeta, _ := json.Marshal(map[string]any{
			"source":            "feishu",
			"conversation_type": incoming.ConversationType,
		})
		if _, err := c.channelLedgerRepo.WithTx(tx).Record(ctx, data.ChannelMessageLedgerRecordInput{
			ChannelID:               ch.ID,
			ChannelType:             ch.ChannelType,
			Direction:               data.ChannelMessageDirectionInbound,
			PlatformConversationID:  incoming.PlatformChatID,
			PlatformMessageID:       incoming.PlatformMsgID,
			SenderChannelIdentityID: &identity.ID,
			MetadataJSON:            ledgerMeta,
		}); err != nil {
			return err
		}
	}
	threadProjectID := derefUUID(persona.ProjectID)
	if threadProjectID == uuid.Nil {
		return fmt.Errorf("cannot resolve project for persona %s", persona.ID)
	}
	threadID, err := c.resolveFeishuThreadID(ctx, tx, ch, persona.ID, threadProjectID, identity, incoming)
	if err != nil {
		return err
	}
	content, err := messagecontent.Normalize(messagecontent.FromText(incoming.Text).Parts)
	if err != nil {
		return err
	}
	contentJSON, err := content.JSON()
	if err != nil {
		return err
	}
	metadataJSON, _ := json.Marshal(map[string]any{
		"source":              "feishu",
		"channel_identity_id": identity.ID.String(),
		"platform_chat_id":    incoming.PlatformChatID,
		"platform_message_id": incoming.PlatformMsgID,
		"platform_user_id":    incoming.SenderSubjectID,
		"chat_type":           incoming.ConversationType,
		"mentions_bot":        incoming.MentionsBot,
		"mentions_all":        incoming.MentionsAll,
	})
	if _, err := c.messageRepo.WithTx(tx).CreateStructuredWithMetadata(ctx, ch.AccountID, threadID, "user", incoming.Text, contentJSON, metadataJSON, identity.UserID); err != nil {
		return err
	}
	runRepoTx := c.runEventRepo.WithTx(tx)
	if err := runRepoTx.LockThreadRow(ctx, threadID); err != nil {
		return err
	}
	activeRun, err := runRepoTx.GetActiveRootRunForThread(ctx, threadID)
	if err != nil {
		return err
	}
	if activeRun != nil {
		delivered, err := c.deliverToActiveRun(ctx, runRepoTx, activeRun, incoming.Text, traceID)
		if err != nil {
			return err
		}
		if delivered {
			if err := commitTx(); err != nil {
				return err
			}
			c.notifyInput(ctx, activeRun.ID)
			return nil
		}
	}
	channelDelivery := buildFeishuChannelDeliveryPayload(ch.ID, identity.ID, incoming)
	runData := buildChannelRunStartedData(personaRef, cfg.DefaultModel, "", channelDelivery)
	run, _, err := runRepoTx.CreateRunWithStartedEvent(ctx, ch.AccountID, threadID, identity.UserID, "run.started", runData)
	if err != nil {
		return err
	}
	jobPayload := map[string]any{
		"source":           "feishu",
		"channel_delivery": channelDelivery,
	}
	if _, err := c.jobRepo.WithTx(tx).EnqueueRun(ctx, ch.AccountID, run.ID, traceID, data.RunExecuteJobType, jobPayload, nil); err != nil {
		return err
	}
	return commitTx()
}

func feishuIdentityMetadata(incoming feishuIncomingMessage) json.RawMessage {
	meta, _ := json.Marshal(map[string]any{
		"open_id":  incoming.SenderOpenID,
		"user_id":  incoming.SenderUserID,
		"union_id": incoming.SenderUnionID,
	})
	return meta
}

func (c feishuConnector) resolveFeishuPersona(ctx context.Context, ch data.Channel) (*data.Persona, string, error) {
	if ch.PersonaID == nil || *ch.PersonaID == uuid.Nil {
		return nil, "", fmt.Errorf("feishu channel requires persona_id")
	}
	persona, err := c.personasRepo.GetByIDForAccount(ctx, ch.AccountID, *ch.PersonaID)
	if err != nil {
		return nil, "", err
	}
	if persona == nil || !persona.IsActive {
		return nil, "", fmt.Errorf("persona not found or inactive")
	}
	return persona, buildPersonaRef(*persona), nil
}

func (c feishuConnector) resolveFeishuThreadID(ctx context.Context, tx pgx.Tx, ch data.Channel, personaID, projectID uuid.UUID, identity data.ChannelIdentity, incoming feishuIncomingMessage) (uuid.UUID, error) {
	threadRepoTx := c.threadRepo.WithTx(tx)
	if incoming.IsPrivate() {
		dmRepo := c.channelDMThreadsRepo.WithTx(tx)
		threadMap, err := dmRepo.GetByBinding(ctx, ch.ID, identity.ID, personaID, "")
		if err != nil {
			return uuid.Nil, err
		}
		if threadMap != nil {
			if existing, _ := threadRepoTx.GetByID(ctx, threadMap.ThreadID); existing != nil {
				return threadMap.ThreadID, nil
			}
			_ = dmRepo.DeleteByBinding(ctx, ch.ID, identity.ID, personaID, "")
		}
		thread, err := threadRepoTx.Create(ctx, ch.AccountID, identity.UserID, projectID, nil, false)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := dmRepo.Create(ctx, ch.ID, identity.ID, personaID, "", thread.ID); err != nil {
			return uuid.Nil, err
		}
		return thread.ID, nil
	}
	groupRepo := c.channelGroupThreadsRepo.WithTx(tx)
	threadMap, err := groupRepo.GetByBinding(ctx, ch.ID, incoming.PlatformChatID, personaID)
	if err != nil {
		return uuid.Nil, err
	}
	if threadMap != nil {
		if existing, _ := threadRepoTx.GetByID(ctx, threadMap.ThreadID); existing != nil {
			return threadMap.ThreadID, nil
		}
		_ = groupRepo.DeleteByBinding(ctx, ch.ID, incoming.PlatformChatID, personaID)
	}
	thread, err := threadRepoTx.Create(ctx, ch.AccountID, nil, projectID, nil, false)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := groupRepo.Create(ctx, ch.ID, incoming.PlatformChatID, personaID, thread.ID); err != nil {
		return uuid.Nil, err
	}
	return thread.ID, nil
}

func (c feishuConnector) deliverToActiveRun(ctx context.Context, repo *data.RunEventRepository, run *data.Run, content, traceID string) (bool, error) {
	if run == nil || strings.TrimSpace(content) == "" {
		return false, nil
	}
	if _, err := repo.ProvideInput(ctx, run.ID, content, traceID); err != nil {
		var notActive data.RunNotActiveError
		if errors.As(err, &notActive) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c feishuConnector) notifyInput(ctx context.Context, runID uuid.UUID) {
	if c.inputNotify == nil || runID == uuid.Nil {
		return
	}
	c.inputNotify(ctx, runID)
}

func buildFeishuChannelDeliveryPayload(channelID uuid.UUID, channelIdentityID uuid.UUID, incoming feishuIncomingMessage) map[string]any {
	return map[string]any{
		"channel_id":   channelID.String(),
		"channel_type": "feishu",
		"conversation_ref": map[string]any{
			"target": incoming.PlatformChatID,
		},
		"inbound_message_ref": map[string]any{
			"message_id": incoming.PlatformMsgID,
		},
		"trigger_message_ref": map[string]any{
			"message_id": incoming.PlatformMsgID,
		},
		"platform_chat_id":           incoming.PlatformChatID,
		"platform_message_id":        incoming.PlatformMsgID,
		"sender_channel_identity_id": channelIdentityID.String(),
		"conversation_type":          incoming.ConversationType,
	}
}
