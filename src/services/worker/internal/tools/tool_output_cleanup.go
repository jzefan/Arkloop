package tools

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools/builtin/fileops"
)

const (
	defaultToolOutputThreadRetention = 30 * 24 * time.Hour
	toolOutputCleanupInterval        = 12 * time.Hour
)

// CleanupToolOutputThread removes one persisted tool-output thread scope from store.
func CleanupToolOutputThread(ctx context.Context, store objectstore.Store, threadID string) {
	if threadID == "" {
		return
	}
	if !validToolCallIDRe.MatchString(threadID) {
		slog.Warn("tool output cleanup skipped: invalid thread_id", "thread_id", threadID)
		return
	}
	if store == nil {
		return
	}
	objects, err := store.ListPrefix(ctx, fileops.ToolOutputObjectPrefix(threadID))
	if err != nil {
		slog.Warn("tool output cleanup list failed", "thread_id", threadID, "error", err.Error())
		return
	}
	for _, item := range objects {
		if delErr := store.Delete(ctx, item.Key); delErr != nil {
			slog.Warn("tool output cleanup delete failed", "thread_id", threadID, "key", item.Key, "error", delErr.Error())
		}
	}
}

// CleanupExpiredToolOutputThreads deletes thread scopes whose latest object update is older than retention.
func CleanupExpiredToolOutputThreads(ctx context.Context, store objectstore.Store, now time.Time, retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}
	if store == nil {
		return 0, nil
	}
	objects, err := store.ListPrefix(ctx, fileops.ToolOutputObjectPrefix(""))
	if err != nil {
		return 0, err
	}

	cutoff := now.Add(-retention)
	threadLatest := map[string]time.Time{}
	for _, item := range objects {
		threadID := fileops.ThreadIDFromToolOutputObjectKey(item.Key)
		if threadID == "" || !validToolCallIDRe.MatchString(threadID) {
			continue
		}
		updatedAt := parseToolOutputUpdatedAt(item.Metadata["updated_at"])
		if updatedAt.IsZero() {
			continue
		}
		if latest, ok := threadLatest[threadID]; !ok || updatedAt.After(latest) {
			threadLatest[threadID] = updatedAt
		}
	}
	deleted := 0
	for threadID, latest := range threadLatest {
		if latest.After(cutoff) {
			continue
		}
		CleanupToolOutputThread(ctx, store, threadID)
		deleted++
	}
	return deleted, nil
}

func StartToolOutputCleanupLoop(ctx context.Context, store objectstore.Store) {
	if ctx == nil || store == nil {
		return
	}
	runToolOutputCleanup(ctx, store, time.Now().UTC())
	go func() {
		ticker := time.NewTicker(toolOutputCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				runToolOutputCleanup(ctx, store, now.UTC())
			}
		}
	}()
}

func runToolOutputCleanup(ctx context.Context, store objectstore.Store, now time.Time) {
	deleted, err := CleanupExpiredToolOutputThreads(ctx, store, now, defaultToolOutputThreadRetention)
	if err != nil {
		slog.Warn("tool output cleanup failed", "error", err.Error())
		return
	}
	if deleted > 0 {
		slog.Info("tool output cleanup deleted expired threads", "deleted", deleted)
	}
}

func parseToolOutputUpdatedAt(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
