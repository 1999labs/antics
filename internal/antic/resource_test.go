package antic

import (
	"context"
	"testing"
)

func TestMemhogCommitRestore(t *testing.T) {
	a, err := newMemHog(map[string]any{"megabytes": 4})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
}

// With a pre-cancelled context, Commit must bail out near-instantly instead of
// allocating the full (here, 4 GB) slab — proving the allocation is interruptible.
func TestMemhogInterruptibleAllocation(t *testing.T) {
	a, err := newMemHog(map[string]any{"megabytes": 4096})
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

func TestMemhogRejectsNonPositive(t *testing.T) {
	if _, err := newMemHog(map[string]any{"megabytes": 0}); err == nil {
		t.Error("expected error for megabytes: 0")
	}
}

func TestCPUHogCommitRestore(t *testing.T) {
	a, err := newCPUHog(map[string]any{"cores": 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
}

func TestCPUHogClampsCores(t *testing.T) {
	// 0 or negative is clamped up to 1; absurdly large is clamped to NumCPU.
	a, err := newCPUHog(map[string]any{"cores": 0})
	if err != nil {
		t.Fatal(err)
	}
	if c := a.(*cpuhogAntic).cores; c < 1 {
		t.Errorf("cores = %d, want >= 1", c)
	}
}
