package security

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// PatternDef 外部传入的模式定义
type PatternDef struct {
	ID       string        `yaml:"id"`
	Category string        `yaml:"category"`
	Severity string        `yaml:"severity"`
	Patterns []string      `yaml:"patterns"`
	Rules    []PatternRule `yaml:"rules"`
}

// PatternRule 表示携带独立严重级别的单条规则。
type PatternRule struct {
	Severity string `yaml:"severity"`
	Pattern  string `yaml:"pattern"`
}

// ScanResult 表示一次模式匹配的结果
type ScanResult struct {
	Matched     bool
	PatternID   string
	Category    string
	Severity    string // "critical", "high", "medium", "low"
	MatchedText string
}

// compiledPattern 是编译后的正则模式
type compiledPattern struct {
	id       string
	category string
	severity string
	re       *regexp.Regexp
}

// RegexScanner 基于正则的注入检测扫描器
type RegexScanner struct {
	mu       sync.RWMutex
	patterns []compiledPattern
}

var (
	wordTokenRe       = regexp.MustCompile(`[A-Za-z]{4,}`)
	typoglycemiaWords = []string{"ignore", "bypass", "override", "reveal", "jailbreak", "system", "prompt", "disregard"}
)

// NewRegexScanner 从模式定义创建扫描器，编译所有正则
func NewRegexScanner(defs []PatternDef) (*RegexScanner, error) {
	compiled, err := compilePatterns(defs)
	if err != nil {
		return nil, err
	}
	return &RegexScanner{patterns: compiled}, nil
}

// Scan 扫描文本，返回所有匹配结果（并发安全）。
// 先做 Unicode 预处理防止字符混淆绕过，再遍历全部 pattern 类别收集匹配。
func (s *RegexScanner) Scan(text string) []ScanResult {
	s.mu.RLock()
	patterns := s.patterns
	s.mu.RUnlock()

	cleaned := sanitizeInput(text)

	var results []ScanResult
	for _, p := range patterns {
		for _, m := range p.re.FindAllString(cleaned, -1) {
			results = append(results, ScanResult{
				Matched:     true,
				PatternID:   p.id,
				Category:    p.category,
				Severity:    p.severity,
				MatchedText: m,
			})
		}
	}
	results = append(results, detectTypoglycemia(text)...)
	return results
}

// Reload 热更新模式库，替换已编译的正则集合
func (s *RegexScanner) Reload(defs []PatternDef) error {
	compiled, err := compilePatterns(defs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.patterns = compiled
	s.mu.Unlock()
	return nil
}

// compilePatterns 将模式定义编译为正则集合
func compilePatterns(defs []PatternDef) ([]compiledPattern, error) {
	var compiled []compiledPattern
	for _, def := range defs {
		if len(def.Rules) > 0 {
			for i, rule := range def.Rules {
				re, err := regexp.Compile(rule.Pattern)
				if err != nil {
					return nil, fmt.Errorf("pattern %s[%d]: %w", def.ID, i, err)
				}
				severity := strings.TrimSpace(rule.Severity)
				if severity == "" {
					severity = def.Severity
				}
				compiled = append(compiled, compiledPattern{
					id:       def.ID,
					category: def.Category,
					severity: severity,
					re:       re,
				})
			}
			continue
		}
		for i, raw := range def.Patterns {
			re, err := regexp.Compile(raw)
			if err != nil {
				return nil, fmt.Errorf("pattern %s[%d]: %w", def.ID, i, err)
			}
			compiled = append(compiled, compiledPattern{
				id:       def.ID,
				category: def.Category,
				severity: def.Severity,
				re:       re,
			})
		}
	}
	return compiled, nil
}

func detectTypoglycemia(text string) []ScanResult {
	words := wordTokenRe.FindAllString(strings.ToLower(text), -1)
	if len(words) == 0 {
		return nil
	}

	results := make([]ScanResult, 0, len(words))
	seen := map[string]struct{}{}
	for _, word := range words {
		for _, keyword := range typoglycemiaWords {
			if !isTypoglycemia(word, keyword) {
				continue
			}
			key := keyword + "|" + word
			if _, ok := seen[key]; ok {
				break
			}
			seen[key] = struct{}{}
			results = append(results, ScanResult{
				Matched:     true,
				PatternID:   "typoglycemia",
				Category:    "typoglycemia",
				Severity:    "high",
				MatchedText: word,
			})
			break
		}
	}
	return results
}

func isTypoglycemia(word, target string) bool {
	if len(word) != len(target) || len(word) < 4 {
		return false
	}
	if word == target {
		return false
	}
	if word[0] != target[0] || word[len(word)-1] != target[len(target)-1] {
		return false
	}

	wordInner := []rune(word[1 : len(word)-1])
	targetInner := []rune(target[1 : len(target)-1])
	sort.Slice(wordInner, func(i, j int) bool { return wordInner[i] < wordInner[j] })
	sort.Slice(targetInner, func(i, j int) bool { return targetInner[i] < targetInner[j] })
	return string(wordInner) == string(targetInner)
}
