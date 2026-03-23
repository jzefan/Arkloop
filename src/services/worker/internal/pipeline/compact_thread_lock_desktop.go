//go:build desktop

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
)

// compactThreadCompactionAdvisoryXactLock is a no-op on desktop builds.
// SQLite doesn't support advisory locks; file locking is used instead via CompactThreadCompactionLock.
func compactThreadCompactionAdvisoryXactLock(_ context.Context, _ pgx.Tx, _ uuid.UUID) error {
	return nil
}

// CompactThreadCompactionLock acquires an exclusive file lock for the given thread.
// This ensures only one compact operation runs at a time per thread.
func CompactThreadCompactionLock(ctx context.Context, threadID uuid.UUID) (func(), error) {
	if threadID == uuid.Nil {
		return func() {}, nil
	}

	rundir := os.Getenv("ARKLOOP_RUNDIR")
	if rundir == "" {
		rundir = filepath.Join(os.TempDir(), "arkloop_compact_locks")
	}
	lockDir := filepath.Join(rundir, "compact_locks")

	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	lockFile := filepath.Join(lockDir, threadID.String()+".lock")
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Acquire exclusive flock - blocks until available
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire flock: %w", err)
	}

	cleanup := func() {
		// Close first - on POSIX, close(fd) automatically releases flock locks.
		// Then remove the lock file.
		f.Close()
		os.Remove(lockFile)
	}

	return cleanup, nil
}
