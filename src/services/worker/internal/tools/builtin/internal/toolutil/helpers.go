package toolutil

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

func SplitModelSelector(selector string) (credentialName, modelName string, exact bool) {
	parts := strings.SplitN(strings.TrimSpace(selector), "^", 2)
	if len(parts) != 2 {
		return "", strings.TrimSpace(selector), false
	}
	cred := strings.TrimSpace(parts[0])
	model := strings.TrimSpace(parts[1])
	if cred == "" || model == "" {
		return "", strings.TrimSpace(selector), false
	}
	return cred, model, true
}

func BuildArtifactKey(execCtx tools.ExecutionContext, filename string) string {
	accountID := "_anonymous"
	if execCtx.AccountID != nil {
		accountID = execCtx.AccountID.String()
	}
	return filepath.ToSlash(fmt.Sprintf("%s/%s/%s", accountID, execCtx.RunID.String(), filename))
}

func StringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

func ErrResult(errorClass, message string, started time.Time) tools.ExecutionResult {
	return ErrResultWithDetails(errorClass, message, nil, started)
}

func ErrResultWithDetails(errorClass, message string, details map[string]any, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
			Details:    CopyMap(details),
		},
		DurationMs: DurationMs(started),
	}
}

func ErrorClassForGenerateError(err error) string {
	if err == nil {
		return "tool.execution_failed"
	}
	if gatewayErr, ok := err.(llm.GatewayError); ok {
		return gatewayErr.ErrorClass
	}
	if gatewayErr, ok := err.(*llm.GatewayError); ok && gatewayErr != nil {
		return gatewayErr.ErrorClass
	}
	return "tool.execution_failed"
}

func ErrorDetailsForGenerateError(err error) map[string]any {
	if err == nil {
		return nil
	}
	if gatewayErr, ok := err.(llm.GatewayError); ok {
		return CopyMap(gatewayErr.Details)
	}
	if gatewayErr, ok := err.(*llm.GatewayError); ok && gatewayErr != nil {
		return CopyMap(gatewayErr.Details)
	}
	return nil
}

func CopyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func DurationMs(started time.Time) int {
	elapsed := time.Since(started)
	if elapsed < 0 {
		return 0
	}
	return int(elapsed / time.Millisecond)
}
