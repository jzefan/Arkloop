package fileops

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileTracker records per-path read/write timestamps within a single run.
// Used by edit and write_file for safety checks (must read before edit).
type FileTracker struct {
	mu      sync.RWMutex
	records map[string]map[string]fileRecord
}

type fileRecord struct {
	readTime  time.Time
	writeTime time.Time
}

func NewFileTracker() *FileTracker {
	return &FileTracker{records: make(map[string]map[string]fileRecord)}
}

func (t *FileTracker) RecordRead(path string) {
	t.RecordReadForRun("", path)
}

func (t *FileTracker) RecordReadForRun(runID string, path string) {
	runID = normalizeRunKey(runID)
	path = normalizePathKey(path)
	if path == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	runRecords := t.records[runID]
	if runRecords == nil {
		runRecords = make(map[string]fileRecord)
		t.records[runID] = runRecords
	}
	r := runRecords[path]
	r.readTime = time.Now()
	runRecords[path] = r
}

func (t *FileTracker) RecordWrite(path string) {
	t.RecordWriteForRun("", path)
}

func (t *FileTracker) RecordWriteForRun(runID string, path string) {
	runID = normalizeRunKey(runID)
	path = normalizePathKey(path)
	if path == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	runRecords := t.records[runID]
	if runRecords == nil {
		runRecords = make(map[string]fileRecord)
		t.records[runID] = runRecords
	}
	r := runRecords[path]
	r.writeTime = time.Now()
	runRecords[path] = r
}

func (t *FileTracker) LastReadTime(path string) time.Time {
	return t.LastReadTimeForRun("", path)
}

func (t *FileTracker) LastReadTimeForRun(runID string, path string) time.Time {
	runID = normalizeRunKey(runID)
	path = normalizePathKey(path)
	if path == "" {
		return time.Time{}
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.records[runID][path].readTime
}

func (t *FileTracker) HasBeenRead(path string) bool {
	return t.HasBeenReadForRun("", path)
}

func (t *FileTracker) HasBeenReadForRun(runID string, path string) bool {
	return !t.LastReadTimeForRun(runID, path).IsZero()
}

func (t *FileTracker) CleanupRun(runID string) {
	runID = normalizeRunKey(runID)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, runID)
}

func TrackingKey(workDir string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base := strings.TrimSpace(workDir)
	if base != "" && !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return normalizePathKey(path)
}

func normalizeRunKey(runID string) string {
	return strings.TrimSpace(runID)
}

func normalizePathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}
