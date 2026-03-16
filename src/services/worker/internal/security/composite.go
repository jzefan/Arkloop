package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type regexScanner interface {
	Scan(text string) []ScanResult
}

type SemanticClassifier interface {
	Classify(ctx context.Context, text string) (SemanticResult, error)
	Close()
}

var ErrInputBlocked = errors.New("security input blocked")

// CompositeScanner 组合 RegexScanner 和 SemanticScanner。
// Semantic 不可用时自动降级为纯 Regex。
type CompositeScanner struct {
	regex    regexScanner
	semantic SemanticClassifier
}

// CompositeScanResult 综合扫描结果。
type CompositeScanResult struct {
	RegexMatches       []ScanResult
	SemanticResult     *SemanticResult
	SemanticError      string
	SemanticSkipReason string
	IsInjection        bool
	Source             string // "regex", "semantic", "both", "none"
}

// CompositeScanOptions 控制组合扫描器的执行策略。
type CompositeScanOptions struct {
	RegexEnabled                bool
	SemanticEnabled             bool
	ShortCircuitOnDecisiveRegex bool
}

// NewCompositeScanner 创建组合扫描器。
// regex 为 nil 时跳过正则扫描，semantic 为 nil 时降级。
func NewCompositeScanner(regex *RegexScanner, semantic SemanticClassifier) *CompositeScanner {
	return newCompositeScanner(normalizeRegexScanner(regex), semantic)
}

// Scan 执行综合扫描。
// 对于高置信 regex 命中，可按选项跳过 semantic 以降低阻断路径延迟。
func (c *CompositeScanner) Scan(ctx context.Context, text string, opts CompositeScanOptions) CompositeScanResult {
	result := CompositeScanResult{Source: "none"}

	if opts.RegexEnabled && c.regex != nil {
		result.RegexMatches = c.regex.Scan(text)
	}

	if opts.SemanticEnabled && c.semantic != nil {
		if opts.ShortCircuitOnDecisiveRegex && hasDecisiveRegexMatch(result.RegexMatches) {
			result.SemanticSkipReason = "decisive_regex_match"
		} else {
			sr, err := c.semantic.Classify(ctx, text)
			if err != nil {
				result.SemanticError = err.Error()
				slog.Warn("semantic scan failed, falling back to regex-only", "error", err)
			} else {
				result.SemanticResult = &sr
			}
		}
	}

	regexHit := len(result.RegexMatches) > 0
	semanticHit := result.SemanticResult != nil && result.SemanticResult.IsInjection

	switch {
	case regexHit && semanticHit:
		result.IsInjection = true
		result.Source = "both"
	case regexHit:
		result.IsInjection = true
		result.Source = "regex"
	case semanticHit:
		result.IsInjection = true
		result.Source = "semantic"
	}

	return result
}

// HasSemantic 返回语义扫描器是否可用。
func (c *CompositeScanner) HasSemantic() bool {
	return c != nil && c.semantic != nil
}

// HasRegex 返回正则扫描器是否可用。
func (c *CompositeScanner) HasRegex() bool {
	return c != nil && c.regex != nil
}

// Close 释放资源。
func (c *CompositeScanner) Close() {
	if c.semantic != nil {
		c.semantic.Close()
	}
}

// String 返回扫描器状态描述。
func (c *CompositeScanner) String() string {
	if c == nil {
		return "CompositeScanner(nil)"
	}
	return fmt.Sprintf("CompositeScanner(regex=%v, semantic=%v)", c.regex != nil, c.semantic != nil)
}

func newCompositeScanner(regex regexScanner, semantic SemanticClassifier) *CompositeScanner {
	return &CompositeScanner{
		regex:    regex,
		semantic: semantic,
	}
}

func normalizeRegexScanner(scanner *RegexScanner) regexScanner {
	if scanner == nil {
		return nil
	}
	return scanner
}

func hasDecisiveRegexMatch(matches []ScanResult) bool {
	for _, match := range matches {
		switch strings.ToLower(match.Severity) {
		case "critical", "high":
			return true
		}
	}
	return false
}
