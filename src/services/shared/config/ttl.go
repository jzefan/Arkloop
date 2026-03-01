package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const cacheTTLSecondsEnv = "ARKLOOP_CONFIG_CACHE_TTL_SECONDS"

func CacheTTLFromEnv() time.Duration {
	raw, ok := os.LookupEnv(cacheTTLSecondsEnv)
	if !ok {
		return 60 * time.Second
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 60 * time.Second
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 60 * time.Second
	}
	if seconds == 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
