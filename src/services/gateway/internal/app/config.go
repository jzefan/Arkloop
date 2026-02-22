package app

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"arkloop/services/gateway/internal/ratelimit"
)

const (
	gatewayAddrEnv     = "ARKLOOP_GATEWAY_ADDR"
	gatewayUpstreamEnv = "ARKLOOP_GATEWAY_UPSTREAM"
	redisURLEnv        = "ARKLOOP_REDIS_URL"
	jwtSecretEnv       = "ARKLOOP_AUTH_JWT_SECRET"

	defaultAddr     = "0.0.0.0:8000"
	defaultUpstream = "http://127.0.0.1:8001"
)

type Config struct {
	Addr      string
	Upstream  string
	RedisURL  string
	JWTSecret string
	RateLimit ratelimit.Config
}

func DefaultConfig() Config {
	return Config{
		Addr:      defaultAddr,
		Upstream:  defaultUpstream,
		RateLimit: ratelimit.DefaultConfig(),
	}
}

func LoadConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()

	if raw := strings.TrimSpace(os.Getenv(gatewayAddrEnv)); raw != "" {
		cfg.Addr = raw
	}
	if raw := strings.TrimSpace(os.Getenv(gatewayUpstreamEnv)); raw != "" {
		cfg.Upstream = raw
	}
	cfg.RedisURL = strings.TrimSpace(os.Getenv(redisURLEnv))
	cfg.JWTSecret = strings.TrimSpace(os.Getenv(jwtSecretEnv))

	rlCfg, err := ratelimit.LoadConfigFromEnv()
	if err != nil {
		return Config{}, fmt.Errorf("ratelimit config: %w", err)
	}
	cfg.RateLimit = rlCfg

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("addr must not be empty")
	}
	if _, err := net.ResolveTCPAddr("tcp", c.Addr); err != nil {
		return fmt.Errorf("addr invalid: %w", err)
	}

	if strings.TrimSpace(c.Upstream) == "" {
		return fmt.Errorf("upstream must not be empty")
	}
	u, err := url.Parse(c.Upstream)
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("upstream must be a valid URL with host: %s", c.Upstream)
	}
	return nil
}
