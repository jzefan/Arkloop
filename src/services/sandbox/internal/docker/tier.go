package docker

import "arkloop/services/sandbox/internal/session"

// TierResources 定义单个 tier 的 Docker 容器资源限制。
type TierResources struct {
	NanoCPUs int64 // CPU 配额，1e9 = 1 核
	MemoryMB int64 // 内存上限 (MB)
}

var tierResources = map[string]TierResources{
	session.TierLite: {NanoCPUs: 1_000_000_000, MemoryMB: 256},
	session.TierPro:  {NanoCPUs: 1_000_000_000, MemoryMB: 1024},
}

func resourcesFor(tier string) TierResources {
	if r, ok := tierResources[tier]; ok {
		return r
	}
	return tierResources[session.TierLite]
}
