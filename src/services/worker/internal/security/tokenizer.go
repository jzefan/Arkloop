package security

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// BPETokenizer 从 HuggingFace tokenizer.json 解析的 BPE tokenizer。
type BPETokenizer struct {
	vocab   map[string]int64
	merges  []mergePair
	added   map[string]int64
	clsID   int64
	sepID   int64
	padID   int64
	unkID   int64
	maxLen  int
}

type mergePair struct {
	a, b string
}

type tokenizerJSON struct {
	Model struct {
		Type  string            `json:"type"`
		Vocab map[string]int64  `json:"vocab"`
		Merges []string         `json:"merges"`
	} `json:"model"`
	AddedTokens []struct {
		Content string `json:"content"`
		ID      int    `json:"id"`
		Special bool   `json:"special"`
	} `json:"added_tokens"`
}

// LoadTokenizer 从 HuggingFace tokenizer.json 加载 BPE tokenizer。
func LoadTokenizer(path string, maxLen int) (*BPETokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer.json: %w", err)
	}

	var raw tokenizerJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}

	if raw.Model.Type != "BPE" {
		return nil, fmt.Errorf("unsupported tokenizer type: %s", raw.Model.Type)
	}

	merges := make([]mergePair, 0, len(raw.Model.Merges))
	for _, m := range raw.Model.Merges {
		parts := strings.SplitN(m, " ", 2)
		if len(parts) != 2 {
			continue
		}
		merges = append(merges, mergePair{a: parts[0], b: parts[1]})
	}

	added := make(map[string]int64, len(raw.AddedTokens))
	var clsID, sepID, padID, unkID int64
	for _, t := range raw.AddedTokens {
		added[t.Content] = int64(t.ID)
		switch t.Content {
		case "[CLS]":
			clsID = int64(t.ID)
		case "[SEP]":
			sepID = int64(t.ID)
		case "[PAD]":
			padID = int64(t.ID)
		case "[UNK]":
			unkID = int64(t.ID)
		}
	}

	if maxLen <= 0 {
		maxLen = 512
	}

	return &BPETokenizer{
		vocab:  raw.Model.Vocab,
		merges: merges,
		added:  added,
		clsID:  clsID,
		sepID:  sepID,
		padID:  padID,
		unkID:  unkID,
		maxLen: maxLen,
	}, nil
}

// Encode 将文本编码为 token ID 序列，包含 [CLS] 和 [SEP]。
// 返回 (input_ids, attention_mask)。
func (t *BPETokenizer) Encode(text string) ([]int64, []int64) {
	tokens := t.tokenize(text)

	// [CLS] + tokens + [SEP]，截断到 maxLen
	maxTokens := t.maxLen - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	ids := make([]int64, 0, len(tokens)+2)
	ids = append(ids, t.clsID)
	for _, tok := range tokens {
		if id, ok := t.vocab[tok]; ok {
			ids = append(ids, id)
		} else if id, ok := t.added[tok]; ok {
			ids = append(ids, id)
		} else {
			ids = append(ids, t.unkID)
		}
	}
	ids = append(ids, t.sepID)

	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}

	// pad to maxLen
	for len(ids) < t.maxLen {
		ids = append(ids, t.padID)
		mask = append(mask, 0)
	}

	return ids, mask
}

// tokenize 将文本拆分为 BPE 子词。
func (t *BPETokenizer) tokenize(text string) []string {
	// pre-tokenize: 按空白分词
	words := splitWords(text)
	var result []string
	for _, word := range words {
		result = append(result, t.bpe(word)...)
	}
	return result
}

// splitWords 按空白和标点进行初步分词。
func splitWords(text string) []string {
	var words []string
	var buf strings.Builder
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if buf.Len() > 0 {
				words = append(words, buf.String())
				buf.Reset()
			}
			i += size
			continue
		}
		buf.WriteRune(r)
		i += size
	}
	if buf.Len() > 0 {
		words = append(words, buf.String())
	}
	return words
}

// bpe 对单个词应用 BPE 合并。
func (t *BPETokenizer) bpe(word string) []string {
	// 将词拆分为单字符
	symbols := make([]string, 0, len(word))
	for _, r := range word {
		symbols = append(symbols, string(r))
	}
	if len(symbols) <= 1 {
		return symbols
	}

	// 构建合并优先级 lookup
	mergeRank := make(map[string]int, len(t.merges))
	for i, m := range t.merges {
		mergeRank[m.a+" "+m.b] = i
	}

	for {
		if len(symbols) < 2 {
			break
		}

		// 找最高优先级的相邻对
		bestIdx := -1
		bestRank := len(t.merges)
		for i := 0; i < len(symbols)-1; i++ {
			key := symbols[i] + " " + symbols[i+1]
			if rank, ok := mergeRank[key]; ok && rank < bestRank {
				bestRank = rank
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}

		merged := symbols[bestIdx] + symbols[bestIdx+1]
		newSymbols := make([]string, 0, len(symbols)-1)
		newSymbols = append(newSymbols, symbols[:bestIdx]...)
		newSymbols = append(newSymbols, merged)
		if bestIdx+2 < len(symbols) {
			newSymbols = append(newSymbols, symbols[bestIdx+2:]...)
		}
		symbols = newSymbols
	}

	return symbols
}

// VocabSize 返回词表大小。
func (t *BPETokenizer) VocabSize() int {
	return len(t.vocab) + len(t.added)
}

// SortedVocab 返回按 ID 排序的词表（调试用）。
func (t *BPETokenizer) SortedVocab() []string {
	type entry struct {
		token string
		id    int64
	}
	entries := make([]entry, 0, len(t.vocab))
	for tok, id := range t.vocab {
		entries = append(entries, entry{tok, id})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.token
	}
	return result
}
