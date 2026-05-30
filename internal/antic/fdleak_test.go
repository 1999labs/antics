package antic

import (
	"context"
	"testing"
)

func TestFdleakDefaultCount(t *testing.T) {
	a, err := newFdleak(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if c := a.(*fdleakAntic).count; c != 256 {
		t.Errorf("default count = %d, want 256", c)
	}
}

func TestFdleakRejectsNonPositive(t *testing.T) {
	if _, err := newFdleak(map[string]any{"count": 0}); err == nil {
		t.Error("expected error for count: 0")
	}
}

// A huge count must be clamped so the leak never starves antics' own fds.
func TestFdleakCapsHugeCount(t *testing.T) {
	a, err := newFdleak(map[string]any{"count": 10_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if c := a.(*fdleakAntic).count; c != maxFDLeak {
		t.Errorf("count = %d, want clamped to %d", c, maxFDLeak)
	}
}

func TestFdleakCommitHoldsThenRestoreCloses(t *testing.T) {
	a, err := newFdleak(map[string]any{"count": 8})
	if err != nil {
		t.Fatal(err)
	}
	fd := a.(*fdleakAntic)
	if err := fd.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := len(fd.held); got != 8 {
		t.Fatalf("held = %d, want 8", got)
	}
	if err := fd.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if fd.held != nil {
		t.Error("held should be nil after Restore")
	}
	// Second Restore must be a safe no-op (no double-close).
	if err := fd.Restore(); err != nil {
		t.Errorf("second Restore: %v", err)
	}
}

func TestFdleakRestoreBeforeCommitSafe(t *testing.T) {
	a, err := newFdleak(map[string]any{"count": 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("Restore before Commit: %v", err)
	}
}
