package antic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIohogCommitWritesAndRestoreDeletes(t *testing.T) {
	dir := t.TempDir()
	a, err := newIohog(map[string]any{"dir": dir, "megabytes": 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	path := filepath.Join(dir, "antics-iohog.tmp")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scratch file missing after commit: %v", err)
	}
	// Give the goroutine a moment, then confirm the file never exceeds the
	// configured bound (it thrashes a fixed size; it must not fill the disk).
	max := int64(2 * 1024 * 1024)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(path); err == nil && fi.Size() > max {
			t.Fatalf("scratch file grew to %d, want <= %d", fi.Size(), max)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("scratch file still present after restore: err=%v", err)
	}
}

func TestIohogRestoreIsAlwaysSafe(t *testing.T) {
	dir := t.TempDir()
	a, err := newIohog(map[string]any{"dir": dir, "megabytes": 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("Restore before Commit: %v", err)
	}
	if err := a.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("first Restore: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("second Restore: %v", err)
	}
}

// A pre-cancelled context must stop the I/O goroutine promptly.
func TestIohogStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	a, err := newIohog(map[string]any{"dir": dir, "megabytes": 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Commit(ctx); err != nil {
		t.Fatalf("Commit with cancelled ctx: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

func TestIohogDefaultsDir(t *testing.T) {
	a, err := newIohog(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if dir := filepath.Dir(a.(*iohogAntic).path); dir != os.TempDir() {
		t.Errorf("default dir = %q, want %q", dir, os.TempDir())
	}
	b, err := newIohog(map[string]any{"dir": ""})
	if err != nil {
		t.Fatal(err)
	}
	if dir := filepath.Dir(b.(*iohogAntic).path); dir != os.TempDir() {
		t.Errorf("empty dir = %q, want fallback to %q", dir, os.TempDir())
	}
}

func TestIohogRecoveryDeletesScratch(t *testing.T) {
	a, _ := newIohog(map[string]any{"dir": "/tmp", "megabytes": 1})
	acts := a.(*iohogAntic).Recovery()
	if len(acts) != 1 || acts[0].Path != filepath.Join("/tmp", "antics-iohog.tmp") {
		t.Errorf("recovery action = %+v, want a delete of the scratch file", acts)
	}
}
