package openviking

import (
	"os"
	"strings"

	"arkloop/services/worker/internal/memory"
)

const (
	envBaseURL    = "ARKLOOP_OPENVIKING_BASE_URL"
	envRootAPIKey = "ARKLOOP_OPENVIKING_ROOT_API_KEY"
)

// Config 保存 OpenViking HTTP 客户端连接参数。
type Config struct {
	BaseURL    string
	RootAPIKey string
}

// Enabled 仅当 BaseURL 非空时视为启用。
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != ""
}

// LoadConfigFromEnv 从环境变量加载配置，供本地开发或无 DB 时使用。
func LoadConfigFromEnv() Config {
	return Config{
		BaseURL:    strings.TrimSpace(os.Getenv(envBaseURL)),
		RootAPIKey: strings.TrimSpace(os.Getenv(envRootAPIKey)),
	}
}

// NewProvider 根据配置返回 MemoryProvider；cfg 未启用时返回 nil（调用方应跳过 Memory）。
func NewProvider(cfg Config) memory.MemoryProvider {
	if !cfg.Enabled() {
		return nil
	}
	return newClient(cfg.BaseURL, cfg.RootAPIKey)
}
