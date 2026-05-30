package antic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskfillCommitThenRestore(t *testing.T) {
	dir := t.TempDir()
	a, err := newDiskfill(map[string]any{"megabytes": 2, "dir": dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	path := filepath.Join(dir, "antics-diskfill.junk")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("junk file missing after commit: %v", err)
	}
	if want := int64(2 * 1024 * 1024); fi.Size() != want {
		t.Errorf("size = %d, want %d", fi.Size(), want)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("junk file still present after restore: err=%v", err)
	}
}

func TestDiskfillRestoreIsAlwaysSafe(t *testing.T) {
	dir := t.TempDir()
	a, err := newDiskfill(map[string]any{"megabytes": 1, "dir": dir})
	if err != nil {
		t.Fatal(err)
	}
	// Restore before any Commit must be safe (the partial/zero-commit contract).
	if err := a.Restore(); err != nil {
		t.Errorf("Restore with no file: %v", err)
	}
	if err := a.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	// And calling it twice must be safe.
	if err := a.Restore(); err != nil {
		t.Errorf("first Restore: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("second Restore: %v", err)
	}
}

func TestDiskfillRejectsNonPositive(t *testing.T) {
	if _, err := newDiskfill(map[string]any{"megabytes": 0}); err == nil {
		t.Error("expected error for megabytes: 0")
	}
}
