package pipeline

import (
	"context"
	"fmt"
	"strings"

	"arkloop/services/worker/internal/agentdirectory"
)

const (
	agentDirectorySegmentName = "agent_directory.context"
	agentDirectoryMaxChars    = 24000
)

// NewAgentDirectoryMiddleware 将 agent work directory 内容注入 system prompt。
// 在 skill_context 之后、memory injection 之前插入。
func NewAgentDirectoryMiddleware(provider agentdirectory.Provider) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc.ProfileRef == "" {
			return next(ctx, rc)
		}

		content, err := provider.Load(ctx, rc.ProfileRef)
		if err != nil || content == nil {
			return next(ctx, rc)
		}

		text := assembleAWDSegment(content)
		if text == "" {
			return next(ctx, rc)
		}

		rc.UpsertPromptSegment(PromptSegment{
			Name:          agentDirectorySegmentName,
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          text,
			Stability:     PromptStabilitySessionPrefix,
			CacheEligible: false,
		})

		return next(ctx, rc)
	}
}

func assembleAWDSegment(c *agentdirectory.Content) string {
	type fileEntry struct {
		name    string
		xmlTag  string
		content string
	}

	entries := []fileEntry{
		{"SOUL.md", "soul", c.Soul},
		{"AGENTS.md", "instructions", c.Instructions},
		{"MEMORY.md", "memory", c.Memory},
		{"USER.md", "user", c.User},
	}
	for _, file := range c.ExtraFiles {
		entries = append(entries, fileEntry{file.Path, "file", file.Content})
	}

	totalChars := 0
	for _, e := range entries {
		totalChars += len(e.content)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Your Agent Work Directory is %s.\n", c.WorkDirPath)

	if totalChars > agentDirectoryMaxChars {
		// 内容超限，降级为目录索引
		sb.WriteString("\nAvailable files:\n")
		for _, e := range entries {
			if e.content != "" {
				fmt.Fprintf(&sb, "- %s (%d chars)\n", e.name, len(e.content))
			}
		}
		return strings.TrimSpace(sb.String())
	}

	for _, e := range entries {
		if e.content == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n<%s path=\"%s\">\n%s\n</%s>", e.xmlTag, e.name, e.content, e.xmlTag)
	}

	return strings.TrimSpace(sb.String())
}
