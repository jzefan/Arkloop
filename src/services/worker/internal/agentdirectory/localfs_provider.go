package agentdirectory

import (
	"context"
	"os"
	"path/filepath"
)

// LocalFSProvider 从本地文件系统读取 AWD 文件，用于 Desktop localshell。
type LocalFSProvider struct {
	homeDirFunc func() string
}

func NewLocalFSProvider(homeDirFunc func() string) *LocalFSProvider {
	return &LocalFSProvider{homeDirFunc: homeDirFunc}
}

func (p *LocalFSProvider) Load(_ context.Context, _ string) (*Content, error) {
	base := p.homeDirFunc()
	_ = os.MkdirAll(base, 0o755)
	content := &Content{WorkDirPath: base}

	files := map[string]*string{
		"SOUL.md":   &content.Soul,
		"AGENTS.md": &content.Instructions,
		"MEMORY.md": &content.Memory,
		"USER.md":   &content.User,
	}
	for name, ptr := range files {
		data, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			continue
		}
		*ptr = string(data)
	}
	return content, nil
}
