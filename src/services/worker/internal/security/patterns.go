package security

// DefaultPatterns 返回编译时内置的默认正则模式
func DefaultPatterns() []PatternDef {
	return []PatternDef{
		{
			ID:       "system_override",
			Category: "instruction_override",
			Severity: "high",
			Patterns: []string{
				`(?i)ignore\s+(all\s+)?(previous|prior|above|earlier|preceding)\s+instructions?`,
				`(?i)forget\s+(all\s+)?(your\s+)?(previous|prior)?\s*instructions?`,
				`(?i)disregard\s+(all\s+)?(previous|prior|above|earlier)?\s*(instructions?|rules?|guidelines?)`,
				`(?i)you\s+are\s+(now\s+)?(a|an|the)\s+`,
				`(?i)new\s+(instructions?|rules?):\s*`,
				`(?i)system\s*:\s*you\s+(must|should|will)`,
				`(?i)override\s+(all\s+)?(previous|prior|system)?\s*(instructions?|rules?|prompts?)`,
				`(?i)from\s+now\s+on[,.]?\s+(you|respond|act|behave|only)`,
				`(?i)(reveal|output|show|print|display)\s+(the\s+)?(original|system|internal|hidden)\s+.{0,20}(prompt|instructions?|rules?)`,
			},
		},
		{
			ID:       "role_hijack",
			Category: "role_manipulation",
			Severity: "high",
			Patterns: []string{
				`(?i)<\/?system>`,
				`(?i)\[SYSTEM\]`,
				`(?i)ADMIN\s*MODE`,
				`(?i)developer\s+mode\s+(enabled|on|activated)`,
				`(?i)jailbreak`,
				`(?i)DAN\s+mode`,
				`(?i)you\s+are\s+(the\s+)?(system\s+prompt|instruction)\s+(generator|creator|writer)`,
				`(?i)pretend\s+(you\s+are|to\s+be)\s+(a\s+)?(different|new|another)`,
				`(?i)act\s+as\s+(if|though)\s+you\s+(have\s+)?no\s+(restrictions?|rules?|limits?)`,
			},
		},
		{
			ID:       "exfiltration",
			Category: "data_exfiltration",
			Severity: "critical",
			Patterns: []string{
				`(?i)send\s+(all|this|the)\s+(data|info|content|conversation)\s+to`,
				`(?i)forward\s+(all|this|the)\s+.{0,30}\s+to\s+https?://`,
				`(?i)encode\s+(and\s+)?send`,
				`(?i)base64\s+encode\s+.{0,30}\s+(and\s+)?(send|post|fetch)`,
			},
		},
		{
			ID:       "hidden_instruction",
			Category: "hidden_content",
			Severity: "medium",
			Patterns: []string{
				`<!--\s*(SYSTEM|INSTRUCTION|ADMIN|IGNORE)`,
				`\x00`,
				`(?i)\[hidden\]`,
				`(?i)invisible\s+instruction`,
			},
		},
	}
}
