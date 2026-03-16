package fileops

import (
	"sync"
	"time"
)

// FileTracker records per-path read/write timestamps within a single run.
// Used by edit and write_file for safety checks (must read before edit).
type FileTracker struct {
	mu      sync.RWMutex
	records map[string]fileRecord
}

type fileRecord struct {
	readTime  time.Time
	writeTime time.Time
}

func NewFileTracker() *FileTracker {
	return &FileTracker{records: make(map[string]fileRecord)}
}

func (t *FileTracker) RecordRead(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[path]
	r.readTime = time.Now()
	t.records[path] = r
}

func (t *FileTracker) RecordWrite(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[path]
	r.writeTime = time.Now()
	t.records[path] = r
}

func (t *FileTracker) LastReadTime(path string) time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.records[path].readTime
}

func (t *FileTracker) HasBeenRead(path string) bool {
	return !t.LastReadTime(path).IsZero()
}
