package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/redis/go-redis/v9"
)

const (
	settingGatewayIPMode              = "gateway.ip_mode"
	settingGatewayTrustedCIDRs        = "gateway.trusted_cidrs"
	settingGatewayRiskRejectThreshold = "gateway.risk_reject_threshold"

	// gatewayConfigRedisKey 与 gateway 服务约定的 Redis key
	gatewayConfigRedisKey = "arkloop:gateway:config"
)

var validIPModes = map[string]struct{}{
	"direct":        {},
	"cloudflare":    {},
	"trusted_proxy": {},
}

type gatewayConfigResponse struct {
	IPMode              string   `json:"ip_mode"`
	TrustedCIDRs        []string `json:"trusted_cidrs"`
	RiskRejectThreshold int      `json:"risk_reject_threshold"`
}

type updateGatewayConfigRequest struct {
	IPMode              string   `json:"ip_mode"`
	TrustedCIDRs        []string `json:"trusted_cidrs"`
	RiskRejectThreshold *int     `json:"risk_reject_threshold"`
}

func adminGatewayConfigEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	settingsRepo *data.PlatformSettingsRepository,
	apiKeysRepo *data.APIKeysRepository,
	rdb *redis.Client,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		switch r.Method {
		case nethttp.MethodGet:
			cfg, err := loadGatewayConfig(r.Context(), settingsRepo)
			if err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			writeJSON(w, traceID, nethttp.StatusOK, cfg)

		case nethttp.MethodPut:
			var body updateGatewayConfigRequest
			if err := decodeJSON(r, &body); err != nil {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
				return
			}

			if err := validateGatewayConfig(body); err != nil {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", err.Error(), traceID, nil)
				return
			}

			if err := saveGatewayConfig(r.Context(), settingsRepo, body); err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}

			// 同步写入 Redis，让 Gateway 30s 内热更新
			if rdb != nil {
				_ = pushGatewayConfigToRedis(r.Context(), rdb, body)
			}

			cfg, err := loadGatewayConfig(r.Context(), settingsRepo)
			if err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			writeJSON(w, traceID, nethttp.StatusOK, cfg)

		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func validateGatewayConfig(body updateGatewayConfigRequest) error {
	if body.IPMode != "" {
		if _, ok := validIPModes[body.IPMode]; !ok {
			return fmt.Errorf("ip_mode must be one of: direct, cloudflare, trusted_proxy")
		}
	}

	for _, cidr := range body.TrustedCIDRs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid CIDR %q: %s", cidr, err.Error())
		}
	}

	if body.RiskRejectThreshold != nil {
		v := *body.RiskRejectThreshold
		if v < 0 || v > 100 {
			return fmt.Errorf("risk_reject_threshold must be 0-100")
		}
	}

	return nil
}

func saveGatewayConfig(ctx context.Context, settingsRepo *data.PlatformSettingsRepository, body updateGatewayConfigRequest) error {
	if body.IPMode != "" {
		if _, err := settingsRepo.Set(ctx, settingGatewayIPMode, body.IPMode); err != nil {
			return err
		}
	}

	cidrs := filterCIDRs(body.TrustedCIDRs)
	encoded, _ := json.Marshal(cidrs)
	if _, err := settingsRepo.Set(ctx, settingGatewayTrustedCIDRs, string(encoded)); err != nil {
		return err
	}

	if body.RiskRejectThreshold != nil {
		val := fmt.Sprintf("%d", *body.RiskRejectThreshold)
		if _, err := settingsRepo.Set(ctx, settingGatewayRiskRejectThreshold, val); err != nil {
			return err
		}
	}

	return nil
}

func loadGatewayConfig(ctx context.Context, settingsRepo *data.PlatformSettingsRepository) (*gatewayConfigResponse, error) {
	cfg := &gatewayConfigResponse{
		IPMode:       "direct",
		TrustedCIDRs: []string{},
	}

	if s, err := settingsRepo.Get(ctx, settingGatewayIPMode); err != nil {
		return nil, err
	} else if s != nil {
		cfg.IPMode = s.Value
	}

	if s, err := settingsRepo.Get(ctx, settingGatewayTrustedCIDRs); err != nil {
		return nil, err
	} else if s != nil {
		var cidrs []string
		if err := json.Unmarshal([]byte(s.Value), &cidrs); err == nil {
			cfg.TrustedCIDRs = cidrs
		}
	}

	if s, err := settingsRepo.Get(ctx, settingGatewayRiskRejectThreshold); err != nil {
		return nil, err
	} else if s != nil {
		var v int
		if _, err := fmt.Sscanf(s.Value, "%d", &v); err == nil {
			cfg.RiskRejectThreshold = v
		}
	}

	return cfg, nil
}

// gatewayRedisPayload 与 gateway 服务的 gatewayDynamicConfig 结构对应。
type gatewayRedisPayload struct {
	IPMode              string   `json:"ip_mode,omitempty"`
	TrustedCIDRs        []string `json:"trusted_cidrs,omitempty"`
	RiskRejectThreshold int      `json:"risk_reject_threshold,omitempty"`
}

func pushGatewayConfigToRedis(ctx context.Context, rdb *redis.Client, body updateGatewayConfigRequest) error {
	payload := gatewayRedisPayload{
		IPMode:       body.IPMode,
		TrustedCIDRs: filterCIDRs(body.TrustedCIDRs),
	}
	if body.RiskRejectThreshold != nil {
		payload.RiskRejectThreshold = *body.RiskRejectThreshold
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, gatewayConfigRedisKey, raw, 0).Err()
}

func filterCIDRs(cidrs []string) []string {
	result := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		if s := strings.TrimSpace(c); s != "" {
			result = append(result, s)
		}
	}
	return result
}
