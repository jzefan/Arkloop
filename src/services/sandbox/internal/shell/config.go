package shell

import (
	"time"

	sandboxskills "arkloop/services/sandbox/internal/skills"
)

type Config struct {
	RestoreTTL         time.Duration
	GovernanceInterval time.Duration
	SkillOverlay       *sandboxskills.OverlayManager
}

func DefaultConfig() Config {
	return Config{
		RestoreTTL:         7 * 24 * time.Hour,
		GovernanceInterval: time.Minute,
	}
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()
	if cfg.RestoreTTL < 0 {
		cfg.RestoreTTL = defaults.RestoreTTL
	}
	if cfg.GovernanceInterval <= 0 {
		cfg.GovernanceInterval = defaults.GovernanceInterval
	}
	return cfg
}
