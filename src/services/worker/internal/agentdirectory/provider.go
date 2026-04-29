package agentdirectory

import "context"

type FileContent struct {
	Path    string
	Content string
}

// Content 从 agent work directory 读取的文件内容，空字符串表示文件不存在。
type Content struct {
	Soul         string // SOUL.md
	Instructions string // AGENTS.md
	Memory       string // MEMORY.md
	User         string // USER.md
	ExtraFiles   []FileContent
	WorkDirPath  string // AWD 路径，注入到 system prompt
}

// Provider 读取 agent work directory 内容。
type Provider interface {
	Load(ctx context.Context, profileRef string) (*Content, error)
}
