package skillstore

import (
	"path/filepath"
	"strings"
)

// PathLayout describes where a runtime expects skill bundles and the skill index.
type PathLayout struct {
	MountRoot string
	IndexPath string
}

func DefaultPathLayout() PathLayout {
	return PathLayout{
		MountRoot: MountRoot,
		IndexPath: IndexPath,
	}
}

func NormalizePathLayout(layout PathLayout) PathLayout {
	normalized := PathLayout{
		MountRoot: strings.TrimSpace(layout.MountRoot),
		IndexPath: strings.TrimSpace(layout.IndexPath),
	}
	if normalized.MountRoot == "" {
		normalized.MountRoot = MountRoot
	}
	if normalized.IndexPath == "" {
		normalized.IndexPath = IndexPath
	}
	return normalized
}

func (l PathLayout) MountPath(skillKey, version string) string {
	layout := NormalizePathLayout(l)
	return filepath.Join(layout.MountRoot, strings.TrimSpace(skillKey)+"@"+strings.TrimSpace(version))
}
