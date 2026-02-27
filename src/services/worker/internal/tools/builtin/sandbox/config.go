package sandbox

import (
	"os"
	"strings"
)

const sandboxBaseURLEnv = "ARKLOOP_SANDBOX_BASE_URL"

func BaseURLFromEnv() string {
	raw := strings.TrimSpace(os.Getenv(sandboxBaseURLEnv))
	return strings.TrimRight(raw, "/")
}
