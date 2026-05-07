package tools

import "strings"

var generativeUIBootstrapTools = map[string]struct{}{
	"visualize_read_me":   {},
	"artifact_guidelines": {},
}

var productHelpTools = map[string]struct{}{
	"arkloop_help": {},
}

// readClassTools are read-class tools that have built-in pagination (offset/limit/lines).
// The system compression layer must not further truncate their output,
// as that would defeat the model's deliberate use of pagination parameters.
var readClassTools = map[string]struct{}{
	"read":          {},
	"read_file":     {},
	"notebook_read": {},
	"memory_read":   {},
}

func IsGenerativeUIBootstrapTool(toolName string) bool {
	_, ok := generativeUIBootstrapTools[strings.TrimSpace(toolName)]
	return ok
}

func ShouldBypassResultCompression(toolName string) bool {
	name := strings.TrimSpace(toolName)
	if _, ok := generativeUIBootstrapTools[name]; ok {
		return true
	}
	if _, ok := productHelpTools[name]; ok {
		return true
	}
	if _, ok := readClassTools[name]; ok {
		return true
	}
	return false
}

func ShouldBypassResultSummarization(toolName string) bool {
	return ShouldBypassResultCompression(toolName)
}
