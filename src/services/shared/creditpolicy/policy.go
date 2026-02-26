package creditpolicy

import "encoding/json"

// CreditTier 描述一个积分计费区间。
// UpToTokens/UpToCostUSD 定义该区间的上限（不包含），nil 表示 catch-all。
// Multiplier 为 0 表示该区间免费，1.0 为正常计费，>1 为溢价。
type CreditTier struct {
	UpToTokens  *int64   `json:"up_to_tokens,omitempty"`
	UpToCostUSD *float64 `json:"up_to_cost_usd,omitempty"`
	Multiplier  float64  `json:"multiplier"`
}

// CreditDeductionPolicy 定义积分扣减的阶梯策略。
// Tiers 按上限升序排列，MultiplierFor 返回第一个满足条件的区间 multiplier。
type CreditDeductionPolicy struct {
	Tiers []CreditTier `json:"tiers"`
}

// DefaultPolicy 平台默认：总 token < 2000 免费，其余正常计费。
var DefaultPolicy = CreditDeductionPolicy{
	Tiers: []CreditTier{
		{UpToTokens: ptrInt64(2000), Multiplier: 0},
		{Multiplier: 1.0},
	},
}

// DefaultPolicyJSON 是 DefaultPolicy 的 JSON 序列化，用于写入 entitlement defaults。
const DefaultPolicyJSON = `{"tiers":[{"up_to_tokens":2000,"multiplier":0},{"multiplier":1}]}`

// Parse 从 JSON 字符串解析策略，解析失败或 Tiers 为空时返回 DefaultPolicy。
func Parse(raw string) CreditDeductionPolicy {
	var p CreditDeductionPolicy
	if err := json.Unmarshal([]byte(raw), &p); err != nil || len(p.Tiers) == 0 {
		return DefaultPolicy
	}
	return p
}

// MultiplierFor 按 totalTokens 和 totalCostUSD 返回适用的策略 multiplier。
// 按 Tiers 顺序遍历，匹配第一个满足上限条件的区间（OR 语义：token 或 cost 任一满足即匹配）。
// 无上限的 catch-all tier 总是匹配。Tiers 为空时返回 1.0。
func (p CreditDeductionPolicy) MultiplierFor(totalTokens int64, totalCostUSD float64) float64 {
	for _, t := range p.Tiers {
		if t.UpToTokens != nil && totalTokens < *t.UpToTokens {
			return t.Multiplier
		}
		if t.UpToCostUSD != nil && totalCostUSD < *t.UpToCostUSD {
			return t.Multiplier
		}
		// catch-all：无任何上限条件
		if t.UpToTokens == nil && t.UpToCostUSD == nil {
			return t.Multiplier
		}
	}
	return 1.0
}

func ptrInt64(v int64) *int64 { return &v }
