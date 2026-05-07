package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RequestStats 保存 LLM request 的上下文分解统计。
type RequestStats struct {
	SystemBytes          int
	ToolsBytes           int
	MessagesBytes        int
	AbstractRequestBytes int
	ImagePartCount       int
	Base64ImageBytes     int
	RoleBytes            map[string]int
	ToolSchemaBytesMap   map[string]int

	StablePrefixHash    string
	SessionPrefixHash   string
	VolatileTailHash    string
	ToolSchemaHash      string
	StablePrefixBytes   int
	SessionPrefixBytes  int
	VolatileTailBytes   int
	CacheCandidateBytes int
}

// ComputeRequestStats 从 Request 计算上下文分解统计。
func ComputeRequestStats(req Request) RequestStats {
	stats := RequestStats{
		RoleBytes:          make(map[string]int),
		ToolSchemaBytesMap: make(map[string]int),
	}

	toolSchemaPayload := buildToolSchemaPayload(req.Tools, stats.ToolSchemaBytesMap, &stats.ToolsBytes, &stats.CacheCandidateBytes)
	stats.ToolSchemaHash = hashText(toolSchemaPayload)

	for _, msg := range req.Messages {
		b, _ := json.Marshal(msg.ToJSON())
		msgBytes := len(b)
		stats.MessagesBytes += msgBytes
		stats.RoleBytes[msg.Role] += msgBytes
		if msg.Role == "system" {
			stats.SystemBytes += msgBytes
		}
		for _, part := range msg.Content {
			if part.Kind() != "image" {
				continue
			}
			stats.ImagePartCount++
			if size, err := modelInputImageBase64Size(part); err == nil {
				stats.Base64ImageBytes += size
			} else if len(part.Data) > 0 {
				stats.Base64ImageBytes += base64EncodedLen(len(part.Data))
			}
		}
	}

	stablePayload, sessionPayload, volatilePayload, stableBytes, sessionBytes, volatileBytes, cacheCandidateFromPlan := buildPromptBuckets(req)
	stats.StablePrefixBytes = stableBytes
	stats.SessionPrefixBytes = sessionBytes
	stats.VolatileTailBytes = volatileBytes
	stats.CacheCandidateBytes += cacheCandidateFromPlan

	// 向后兼容：stable_prefix_hash 仍带 tool schema。
	stats.StablePrefixHash = hashText(stablePayload + "|" + toolSchemaPayload)
	stats.SessionPrefixHash = hashText(sessionPayload)
	stats.VolatileTailHash = hashText(volatilePayload)

	stats.AbstractRequestBytes = EstimateRequestJSONBytes(req)
	return stats
}

func base64EncodedLen(n int) int {
	return (n + 2) / 3 * 4
}

func EstimateRequestJSONBytes(req Request) int {
	raw, err := json.Marshal(req.ToJSON())
	if err != nil {
		return 0
	}
	return len(raw)
}

func buildToolSchemaPayload(
	tools []ToolSpec,
	toolSchemaBytes map[string]int,
	totalBytes *int,
	cacheCandidateBytes *int,
) string {
	type toolHashEntry struct {
		Name string
		JSON string
	}
	entries := make([]toolHashEntry, 0, len(tools))
	for _, tool := range tools {
		b, _ := json.Marshal(tool.ToJSON())
		schemaBytes := len(b)
		*totalBytes += schemaBytes
		toolSchemaBytes[tool.Name] = schemaBytes
		if hasWriteCacheHint(tool.CacheHint, nil) {
			*cacheCandidateBytes += schemaBytes
		}
		entries = append(entries, toolHashEntry{
			Name: strings.TrimSpace(tool.Name),
			JSON: string(b),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	var payload strings.Builder
	for _, entry := range entries {
		payload.WriteString(entry.Name)
		payload.WriteString("=")
		payload.WriteString(entry.JSON)
		payload.WriteString("\n")
	}
	return payload.String()
}

func buildPromptBuckets(req Request) (stablePayload string, sessionPayload string, volatilePayload string, stableBytes int, sessionBytes int, volatileBytes int, cacheCandidateBytes int) {
	var stableBuilder strings.Builder
	var sessionBuilder strings.Builder
	var volatileBuilder strings.Builder

	if req.PromptPlan != nil && (len(req.PromptPlan.SystemBlocks) > 0 || len(req.PromptPlan.MessageBlocks) > 0) {
		for _, block := range req.PromptPlan.SystemBlocks {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			blockBytes := len([]byte(text))
			switch normalizeStability(block.Stability) {
			case CacheStabilitySessionPrefix:
				sessionBuilder.WriteString(text)
				sessionBuilder.WriteString("\n")
				sessionBytes += blockBytes
			case CacheStabilityVolatileTail:
				volatileBuilder.WriteString(text)
				volatileBuilder.WriteString("\n")
				volatileBytes += blockBytes
			default:
				stableBuilder.WriteString(text)
				stableBuilder.WriteString("\n")
				stableBytes += blockBytes
			}
			if block.CacheEligible {
				cacheCandidateBytes += blockBytes
			}
		}
		for _, block := range req.PromptPlan.MessageBlocks {
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			blockBytes := len([]byte(text))
			switch normalizeStability(block.Stability) {
			case CacheStabilitySessionPrefix:
				sessionBuilder.WriteString(text)
				sessionBuilder.WriteString("\n")
				sessionBytes += blockBytes
			case CacheStabilityVolatileTail:
				volatileBuilder.WriteString(text)
				volatileBuilder.WriteString("\n")
				volatileBytes += blockBytes
			default:
				stableBuilder.WriteString(text)
				stableBuilder.WriteString("\n")
				stableBytes += blockBytes
			}
			if block.CacheEligible {
				cacheCandidateBytes += blockBytes
			}
		}
	} else {
		for _, msg := range req.Messages {
			if msg.Role != "system" {
				continue
			}
			for _, part := range msg.Content {
				text := strings.TrimSpace(PartPromptText(part))
				if text == "" {
					continue
				}
				stableBuilder.WriteString(text)
				stableBuilder.WriteString("\n")
				stableBytes += len([]byte(text))
				if hasWriteCacheHint(part.CacheHint, part.CacheControl) {
					cacheCandidateBytes += len([]byte(text))
				}
			}
		}
	}

	// 在 messages 中继续统计显式 cache hint（包括非 system 内容）。
	for _, msg := range req.Messages {
		for _, part := range msg.Content {
			if !hasWriteCacheHint(part.CacheHint, part.CacheControl) {
				continue
			}
			text := strings.TrimSpace(PartPromptText(part))
			if text == "" {
				continue
			}
			cacheCandidateBytes += len([]byte(text))
		}
	}

	return stableBuilder.String(), sessionBuilder.String(), volatileBuilder.String(), stableBytes, sessionBytes, volatileBytes, cacheCandidateBytes
}

func normalizeStability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CacheStabilitySessionPrefix:
		return CacheStabilitySessionPrefix
	case CacheStabilityVolatileTail:
		return CacheStabilityVolatileTail
	default:
		return CacheStabilityStablePrefix
	}
}

func hasWriteCacheHint(hint *CacheHint, legacyCacheControl *string) bool {
	if hint != nil {
		if strings.EqualFold(strings.TrimSpace(hint.Action), CacheHintActionWrite) {
			return true
		}
	}
	return legacyCacheControl != nil && strings.TrimSpace(*legacyCacheControl) != ""
}

func hashText(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:8])
}
