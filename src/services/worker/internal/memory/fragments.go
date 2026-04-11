package memory

import "context"

// MemoryFragment 是 Arkloop 内部用于 snapshot/impression 的统一颗粒记忆表示。
type MemoryFragment struct {
	ID          string
	URI         string
	Title       string
	Content     string
	Abstract    string
	Score       float64
	Labels      []string
	RecordedAt  string
	IsEphemeral bool
}

// MemoryFragmentSource 提供 snapshot/impression 所需的颗粒记忆列表。
type MemoryFragmentSource interface {
	ListFragments(ctx context.Context, ident MemoryIdentity, limit int) ([]MemoryFragment, error)
}
