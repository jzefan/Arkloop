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
func NewInjectionScanMiddleware(
	scanner *security.RegexScanner,
	auditor *security.SecurityAuditor,
	configResolver sharedconfig.Resolver,
) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if scanner == nil {
			security.ScanTotal.WithLabelValues("skipped").Inc()
			return next(ctx, rc)
		}

		regexEnabled := resolveEnabled(configResolver, "security.injection_scan.regex_enabled", true)
		if !regexEnabled {
			return next(ctx, rc)
		}

		var allDetections []security.ScanResult

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
					security.DetectionTotal.WithLabelValues(r.Category).Inc()
				}
				allDetections = append(allDetections, results...)
			}
		}

		if len(allDetections) > 0 {
			security.ScanTotal.WithLabelValues("detected").Inc()
			auditor.EmitInjectionDetected(ctx, rc.Run.ID, rc.Run.AccountID, rc.UserID, allDetections)
		} else {
			security.ScanTotal.WithLabelValues("clean").Inc()
		}

		return next(ctx, rc)
	}
}
