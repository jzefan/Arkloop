package accountapi

import (
	"encoding/json"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

	"arkloop/services/api/internal/auth"
	"arkloop/services/shared/weixinclient"
)

const weixinBaseURL = "https://ilinkai.weixin.qq.com"

// WeixinQRStore 内存缓存二维码会话
type WeixinQRStore struct {
	mu     sync.Mutex
	qrData map[string]*WeixinQRSession
}

// WeixinQRSession 单次二维码登录会话
type WeixinQRSession struct {
	Qrcode    string
	CreatedAt time.Time
}

var weixinQRStore = &WeixinQRStore{qrData: make(map[string]*WeixinQRSession)}

func (s *WeixinQRStore) set(qrcode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.qrData[qrcode] = &WeixinQRSession{
		Qrcode:    qrcode,
		CreatedAt: time.Now(),
	}
}

// WeixinDeps holds dependencies for Weixin API handlers.
type WeixinDeps struct {
	AuthService *auth.Service
}

// RegisterWeixinRoutes adds /v1/weixin/* endpoints to the mux.
func RegisterWeixinRoutes(mux *nethttp.ServeMux, deps WeixinDeps) {
	mux.HandleFunc("GET /v1/weixin/qrcode", weixinHandler(deps.AuthService, deps.handleWeixinQRCode))
	mux.HandleFunc("GET /v1/weixin/qrcode-status", weixinHandler(deps.AuthService, deps.handleWeixinQRCodeStatus))
}

func (d WeixinDeps) handleWeixinQRCode(w nethttp.ResponseWriter, r *nethttp.Request) {
	client := weixinclient.NewClient(weixinBaseURL, "", nil)

	resp, err := client.GetBotQRCode(r.Context())
	if err != nil {
		writeWeixinJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	weixinQRStore.set(resp.Qrcode)

	writeWeixinJSON(w, nethttp.StatusOK, map[string]string{
		"qrcode":             resp.Qrcode,
		"qrcode_img_content": resp.QrcodeImgContent,
	})
}

func (d WeixinDeps) handleWeixinQRCodeStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	qrcode := strings.TrimSpace(r.URL.Query().Get("qrcode"))
	if qrcode == "" {
		writeWeixinJSON(w, nethttp.StatusBadRequest, map[string]string{"error": "qrcode is required"})
		return
	}

	client := weixinclient.NewClient(weixinBaseURL, "", nil)

	resp, err := client.GetQRCodeStatus(r.Context(), qrcode)
	if err != nil {
		writeWeixinJSON(w, nethttp.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	result := map[string]string{"status": resp.Status}
	if resp.Status == "confirmed" {
		result["bot_token"] = resp.BotToken
		result["baseurl"] = resp.BaseURL
	}

	writeWeixinJSON(w, nethttp.StatusOK, result)
}

func weixinHandler(authService *auth.Service, handler nethttp.HandlerFunc) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if authService != nil {
			token := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
			if _, err := authService.VerifyAccessTokenForActor(r.Context(), token); err != nil {
				w.WriteHeader(nethttp.StatusUnauthorized)
				return
			}
		}
		handler(w, r)
	}
}

func writeWeixinJSON(w nethttp.ResponseWriter, code int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(raw)
}
