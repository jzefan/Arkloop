package session

import "fmt"

// 支持的资源 tier 名称。
const (
	TierLite  = "lite"
	TierPro   = "pro"
	TierUltra = "ultra"
)

// ValidTier 验证 tier 值是否合法。
func ValidTier(tier string) error {
	switch tier {
	case TierLite, TierPro, TierUltra:
		return nil
	default:
		return fmt.Errorf("unknown tier %q: must be lite, pro, or ultra", tier)
	}
}
