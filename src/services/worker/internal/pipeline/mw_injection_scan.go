package pipeline

import (
	"context"
	"log/slog"
	"strings"

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/worker/internal/security"
)

// NewInjectionScanMiddleware 在 Pipeline 中执行注入扫描。
// scanner 为 nil 时整个 middleware 为 no-op。
func NewInjectionScanMiddleware(scanner *security.RegexScanner, configResolver sharedconfig.Resolver) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if scanner == nil {
			return next(ctx, rc)
		}

		regexEnabled := resolveEnabled(configResolver, "security.injection_scan.regex_enabled", true)
		if !regexEnabled {
			return next(ctx, rc)
		}

		for _, msg := range rc.Messages {
			if msg.Role != "user" {
				continue
			}
			for _, part := range msg.Content {
				text := strings.TrimSpace(part.Text)
				if text == "" {
					continue
				}
				results := scanner.Scan(text)
				for _, r := range results {
					slog.WarnContext(ctx, "injection pattern detected",
						"run_id", rc.Run.ID,
						"pattern_id", r.PatternID,
						"category", r.Category,
						"severity", r.Severity,
					)
				}
			}
		}

		return next(ctx, rc)
	}
}
