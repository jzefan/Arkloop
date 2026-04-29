package pipeline

import (
	"context"
	"fmt"
	"strings"

	"arkloop/services/worker/internal/routing"
)

// NewModelIdentityMiddleware injects a <model_identity> segment into the system prompt
// so the LLM knows which provider, model, and capabilities it is running on.
// Must run after RoutingMiddleware (rc.SelectedRoute is required).
func NewModelIdentityMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc.SelectedRoute != nil {
			rc.UpsertPromptSegment(PromptSegment{
				Name:          "runtime.model_identity",
				Target:        PromptTargetSystemPrefix,
				Role:          "system",
				Text:          buildModelIdentityBlock(rc),
				Stability:     PromptStabilitySessionPrefix,
				CacheEligible: true,
			})
		}
		return next(ctx, rc)
	}
}

func buildModelIdentityBlock(rc *RunContext) string {
	selected := rc.SelectedRoute
	caps := routing.SelectedRouteModelCapabilities(selected)

	var sb strings.Builder
	sb.WriteString("<model_identity>\n")

	sb.WriteString("Provider: " + selected.Credential.Name + "\n")
	sb.WriteString("Model: " + selected.Route.Model + "\n")

	if caps.ContextLength > 0 {
		sb.WriteString("Context Window: " + fmt.Sprintf("%d", caps.ContextLength) + " tokens\n")
	}
	if caps.MaxOutputTokens > 0 {
		sb.WriteString("Max Output Tokens: " + fmt.Sprintf("%d", caps.MaxOutputTokens) + " tokens\n")
	}

	inputMods := ensureTextModality(caps.InputModalities)
	outputMods := ensureTextModality(caps.OutputModalities)
	sb.WriteString("Input Modalities: " + strings.Join(inputMods, ", ") + "\n")
	sb.WriteString("Output Modalities: " + strings.Join(outputMods, ", ") + "\n")

	if rc.Temperature != nil {
		sb.WriteString(fmt.Sprintf("Temperature: %.2f\n", *rc.Temperature))
	}

	if rc.AgentConfig != nil && rc.AgentConfig.ReasoningMode != "" {
		sb.WriteString("Reasoning Mode: " + rc.AgentConfig.ReasoningMode + "\n")
	}

	sb.WriteString("</model_identity>")
	return sb.String()
}

func ensureTextModality(mods []string) []string {
	for _, m := range mods {
		if m == "text" {
			return mods
		}
	}
	return append([]string{"text"}, mods...)
}
