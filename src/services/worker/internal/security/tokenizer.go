package security

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"
)

const metaspaceReplacement = '▁'

// Tokenizer 从 HuggingFace tokenizer.json 加载，支持 Unigram 和 BPE。
type Tokenizer struct {
	tokenToID   map[string]int64
	scores      map[string]float64 // unigram scores
	added       map[string]int64
	maxTokenLen int
	clsID       int64
	sepID       int64
	padID       int64
	unkID       int64
	maxLen      int
}

type tokenizerJSON struct {
	Model struct {
		Type  string          `json:"type"`
		Vocab json.RawMessage `json:"vocab"`
		UNKId *int            `json:"unk_id"`
	} `json:"model"`
	AddedTokens []struct {
		Content string `json:"content"`
		ID      int    `json:"id"`
	} `json:"added_tokens"`
}

// LoadTokenizer 从 HuggingFace tokenizer.json 加载 tokenizer。
func LoadTokenizer(path string, maxLen int) (*Tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer.json: %w", err)
	}

	var raw tokenizerJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}

	if maxLen <= 0 {
		maxLen = 512
	}

	t := &Tokenizer{
		tokenToID: make(map[string]int64),
		scores:    make(map[string]float64),
		added:     make(map[string]int64),
		maxLen:    maxLen,
	}

	switch raw.Model.Type {
	case "Unigram":
		if err := t.parseUnigramVocab(raw.Model.Vocab); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported tokenizer type: %s", raw.Model.Type)
	}

	for _, tok := range raw.AddedTokens {
		t.added[tok.Content] = int64(tok.ID)
		switch tok.Content {
		case "[CLS]":
			t.clsID = int64(tok.ID)
		case "[SEP]":
			t.sepID = int64(tok.ID)
		case "[PAD]":
			t.padID = int64(tok.ID)
		case "[UNK]":
			t.unkID = int64(tok.ID)
		}
	}

	return t, nil
}

func (t *Tokenizer) parseUnigramVocab(raw json.RawMessage) error {
	var entries [][]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("parse unigram vocab: %w", err)
	}

	for i, entry := range entries {
		if len(entry) < 2 {
			continue
		}
		var token string
		var score float64
		if err := json.Unmarshal(entry[0], &token); err != nil {
			continue
		}
		if err := json.Unmarshal(entry[1], &score); err != nil {
			continue
		}
		t.tokenToID[token] = int64(i)
		t.scores[token] = score
		if tl := utf8.RuneCountInString(token); tl > t.maxTokenLen {
			t.maxTokenLen = tl
		}
	}
	return nil
}

// Encode 将文本编码为 token ID 序列 (input_ids, attention_mask)。
func (t *Tokenizer) Encode(text string) ([]int64, []int64) {
	pieces := t.preTokenize(text)
	var tokens []string
	for _, piece := range pieces {
		tokens = append(tokens, t.segment(piece)...)
	}

	maxTokens := t.maxLen - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	ids := make([]int64, 0, len(tokens)+2)
	ids = append(ids, t.clsID)
	for _, tok := range tokens {
		if id, ok := t.tokenToID[tok]; ok {
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
	for len(ids) < t.maxLen {
		ids = append(ids, t.padID)
		mask = append(mask, 0)
	}
	return ids, mask
}

// preTokenize 执行 Metaspace 预分词：
// 将空格替换为 ▁，在文本开头也添加 ▁。
func (t *Tokenizer) preTokenize(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var buf strings.Builder
	buf.WriteRune(metaspaceReplacement)

	for _, r := range text {
		if r == ' ' {
			buf.WriteRune(metaspaceReplacement)
		} else {
			buf.WriteRune(r)
		}
	}
	return []string{buf.String()}
}

// segment 使用 Viterbi 算法对单个 piece 做 Unigram 分词。
func (t *Tokenizer) segment(text string) []string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return nil
	}

	const negInf = -1e18
	// best[i] = 从位置 i 到末尾的最优分词分数
	best := make([]float64, n+1)
	// backPtr[i] = 从位置 i 开始的最优 token 长度（rune 数）
	backPtr := make([]int, n+1)
	for i := range best {
		best[i] = negInf
	}
	best[n] = 0

	maxTokLen := t.maxTokenLen
	if maxTokLen <= 0 {
		maxTokLen = 64
	}

	// 从右向左 DP
	for i := n - 1; i >= 0; i-- {
		limit := n - i
		if limit > maxTokLen {
			limit = maxTokLen
		}
		for l := 1; l <= limit; l++ {
			tok := string(runes[i : i+l])
			score, ok := t.scores[tok]
			if !ok {
				if l == 1 {
					// 单字符 fallback，给一个很低的分数
					score = -100.0
				} else {
					continue
				}
			}
			total := score + best[i+l]
			if total > best[i] {
				best[i] = total
				backPtr[i] = l
			}
		}
		// 如果没有任何匹配（不应该发生，因为有单字符 fallback）
		if best[i] == negInf {
			best[i] = -200.0 + best[i+1]
			backPtr[i] = 1
		}
	}

	var tokens []string
	for pos := 0; pos < n; {
		l := backPtr[pos]
		if l <= 0 {
			l = 1
		}
		tok := string(runes[pos : pos+l])
		if _, ok := t.tokenToID[tok]; ok {
			tokens = append(tokens, tok)
		} else {
			// 分解为单字符
			for _, r := range tok {
				tokens = append(tokens, string(r))
			}
		}
		pos += l
	}
	return tokens
}

// VocabSize 返回词表大小。
func (t *Tokenizer) VocabSize() int {
	return len(t.tokenToID) + len(t.added)
}

// softmaxFloat64 对 float64 slice 做 softmax。
func softmaxFloat64(x []float64) []float64 {
	max := x[0]
	for _, v := range x[1:] {
		if v > max {
			max = v
		}
	}
	sum := 0.0
	out := make([]float64, len(x))
	for i, v := range x {
		out[i] = math.Exp(v - max)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}
