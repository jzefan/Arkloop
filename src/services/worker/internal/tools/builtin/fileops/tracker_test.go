package fileops

import "testing"

func TestFileTrackerIsRunScoped(t *testing.T) {
	tracker := NewFileTracker()
	runA := "run-a"
	runB := "run-b"
	key := TrackingKey("/workspace", "./notes/todo.md")

	tracker.RecordReadForRun(runA, key)

	if !tracker.HasBeenReadForRun(runA, TrackingKey("/workspace", "notes/todo.md")) {
		t.Fatal("expected run-a to see its read")
	}
	if tracker.HasBeenReadForRun(runB, TrackingKey("/workspace", "notes/todo.md")) {
		t.Fatal("did not expect run-b to inherit run-a state")
	}
}

func TestFileTrackerCleanupRun(t *testing.T) {
	tracker := NewFileTracker()
	key := TrackingKey("/workspace", "doc.txt")
	tracker.RecordReadForRun("run-a", key)
	tracker.RecordReadForRun("run-b", key)

	tracker.CleanupRun("run-a")

	if tracker.HasBeenReadForRun("run-a", key) {
		t.Fatal("expected run-a state to be removed")
	}
	if !tracker.HasBeenReadForRun("run-b", key) {
		t.Fatal("expected run-b state to remain")
	}
}

func TestTrackingKeyNormalizesRelativePaths(t *testing.T) {
	base := "/workspace"
	a := TrackingKey(base, "./src/../src/app.txt")
	b := TrackingKey(base, "src/app.txt")

	if a != b {
		t.Fatalf("expected normalized keys to match, got %q vs %q", a, b)
	}
}
