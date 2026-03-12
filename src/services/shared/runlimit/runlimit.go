package runlimit

import "time"

const KeyPrefix = "arkloop:org:active_runs:"

const defaultTTL = 24 * time.Hour

// Key 根据 orgID 字符串构建 Redis key。
func Key(orgID string) string {
	return KeyPrefix + orgID
}
