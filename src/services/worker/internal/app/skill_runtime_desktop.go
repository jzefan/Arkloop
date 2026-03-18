//go:build desktop

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arkloop/services/shared/desktop"
	"arkloop/services/shared/objectstore"
	"arkloop/services/shared/skillstore"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/pipeline"
	"github.com/google/uuid"
)

func desktopSkillResolver(db data.DesktopDB) pipeline.SkillResolver {
	if db == nil {
		return nil
	}
	repo := data.NewSkillsRepository(db)
	return func(ctx context.Context, accountID uuid.UUID, profileRef, workspaceRef string) ([]skillstore.ResolvedSkill, error) {
		return repo.ResolveEnabledSkills(ctx, accountID, profileRef, workspaceRef)
	}
}

func desktopSkillLayoutResolver(useVM bool) pipeline.SkillLayoutResolver {
	return func(_ context.Context, rc *pipeline.RunContext) (skillstore.PathLayout, error) {
		return desktopSkillLayout(useVM, rc.Run.ID)
	}
}

func desktopSkillLayout(useVM bool, runID uuid.UUID) (skillstore.PathLayout, error) {
	if useVM {
		return skillstore.DefaultPathLayout(), nil
	}
	root, err := desktopSkillRuntimeRoot(runID)
	if err != nil {
		return skillstore.PathLayout{}, err
	}
	return skillstore.PathLayout{
		MountRoot: filepath.Join(root, "files"),
		IndexPath: filepath.Join(root, "enabled-skills.json"),
	}, nil
}

func desktopSkillPreparer(useVM bool) pipeline.SkillPreparer {
	if useVM {
		return nil
	}
	return prepareDesktopHostSkills
}

func desktopSkillRuntimeRoot(runID uuid.UUID) (string, error) {
	if runID == uuid.Nil {
		return "", fmt.Errorf("run_id must not be empty")
	}
	dataDir, err := desktop.ResolveDataDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "runtime", "skills", runID.String()), nil
}

func cleanupDesktopSkillRuntime(runID uuid.UUID) error {
	root, err := desktopSkillRuntimeRoot(runID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove desktop skill runtime: %w", err)
	}
	return nil
}

func prepareDesktopHostSkills(ctx context.Context, skills []skillstore.ResolvedSkill, layout skillstore.PathLayout) error {
	store, err := openDesktopSkillStore(ctx)
	if err != nil {
		return err
	}
	layout = skillstore.NormalizePathLayout(layout)
	runtimeRoot := filepath.Dir(layout.IndexPath)
	if err := os.RemoveAll(runtimeRoot); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset desktop skill runtime: %w", err)
	}
	if err := os.MkdirAll(layout.MountRoot, 0o755); err != nil {
		return fmt.Errorf("create desktop skills dir: %w", err)
	}
	for _, item := range skills {
		bundleRef := strings.TrimSpace(item.BundleRef)
		if bundleRef == "" {
			return fmt.Errorf("skill %s@%s bundle_ref is empty", item.SkillKey, item.Version)
		}
		encoded, err := store.Get(ctx, bundleRef)
		if err != nil {
			return fmt.Errorf("load skill bundle %s@%s: %w", item.SkillKey, item.Version, err)
		}
		bundle, err := skillstore.DecodeBundle(encoded)
		if err != nil {
			return fmt.Errorf("decode skill bundle %s@%s: %w", item.SkillKey, item.Version, err)
		}
		targetRoot := layout.MountPath(item.SkillKey, item.Version)
		if err := writeDesktopSkillBundle(targetRoot, bundle); err != nil {
			return err
		}
	}
	indexJSON, err := skillstore.BuildIndex(skills)
	if err != nil {
		return fmt.Errorf("build desktop skill index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(layout.IndexPath), 0o755); err != nil {
		return fmt.Errorf("create desktop skill index dir: %w", err)
	}
	if err := atomicWriteDesktopFile(layout.IndexPath, indexJSON, 0o644); err != nil {
		return fmt.Errorf("write desktop skill index: %w", err)
	}
	return nil
}

func openDesktopSkillStore(ctx context.Context) (objectstore.Store, error) {
	dataDir, err := desktop.ResolveDataDir("")
	if err != nil {
		return nil, err
	}
	return objectstore.NewFilesystemOpener(desktop.StorageRoot(dataDir)).Open(ctx, objectstore.SkillStoreBucket)
}

func writeDesktopSkillBundle(root string, bundle skillstore.BundleImage) error {
	if err := os.RemoveAll(root); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset desktop skill dir %s: %w", root, err)
	}
	for _, file := range bundle.Files {
		targetPath, err := desktopSkillTargetPath(root, file.Path)
		if err != nil {
			return err
		}
		if file.IsDir {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create desktop skill dir %s: %w", targetPath, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("create desktop skill parent %s: %w", targetPath, err)
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := atomicWriteDesktopFile(targetPath, file.Data, mode); err != nil {
			return fmt.Errorf("write desktop skill file %s: %w", targetPath, err)
		}
	}
	return nil
}

func desktopSkillTargetPath(root, relativePath string) (string, error) {
	root = filepath.Clean(root)
	target := filepath.Join(root, filepath.FromSlash(relativePath))
	target = filepath.Clean(target)
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", fmt.Errorf("desktop skill path escapes root: %s", relativePath)
	}
	return target, nil
}

func atomicWriteDesktopFile(targetPath string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), ".arkloop-skill-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, targetPath)
}
