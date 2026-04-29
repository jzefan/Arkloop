package agentdirectory

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	content.ExtraFiles = loadExtraMarkdownFiles(base, files)
	return content, nil
}

func loadExtraMarkdownFiles(base string, canonical map[string]*string) []FileContent {
	extras := []FileContent{}
	_ = filepath.WalkDir(base, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			switch name {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}
		rel, err := filepath.Rel(base, filePath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := canonical[rel]; ok {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		extras = append(extras, FileContent{Path: rel, Content: string(data)})
		return nil
	})
	sort.Slice(extras, func(i, j int) bool { return extras[i].Path < extras[j].Path })
	return extras
}
