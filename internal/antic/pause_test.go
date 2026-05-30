//go:build unix

package antic

import (
	"os"
	"testing"
)

func TestPauseBuildsWithSafePattern(t *testing.T) {
	a, err := newPause(map[string]any{"match": "zzz-nonexistent-service-zzz"})
	if err != nil {
		t.Fatalf("newPause: %v", err)
	}
	if got := a.Name(); got != "pause" {
		t.Errorf("name = %q, want pause", got)
	}
}

func TestPauseRefusesEmptyMatch(t *testing.T) {
	if _, err := newPause(map[string]any{"match": ""}); err == nil {
		t.Error("expected error for empty match")
	}
}

// match: 8080 arrives from the parser as an int; pause must accept it.
func TestPauseAcceptsNumericMatch(t *testing.T) {
	if _, err := newPause(map[string]any{"match": 8080}); err != nil {
		t.Fatalf("newPause with numeric match: %v", err)
	}
}

// The self-freeze guard is mandatory: a pattern matching antics' own command
// line would SIGSTOP the very process that must later thaw everything.
func TestPauseRefusesToMatchItself(t *testing.T) {
	if _, err := newPause(map[string]any{"match": os.Args[0]}); err == nil {
		t.Error("expected refusal when pattern matches antics' own command line")
	}
}

// Restore is pkill -CONT against a guaranteed-nonexistent pattern: a harmless
// no-op (pkill exit 1 swallowed), so it's safe before any Commit and repeatable.
func TestPauseRestoreIsAlwaysSafe(t *testing.T) {
	a, err := newPause(map[string]any{"match": "zzz-nonexistent-service-zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("Restore before Commit: %v", err)
	}
	if err := a.Restore(); err != nil {
		t.Errorf("second Restore: %v", err)
	}
}

func TestPauseRecoveryThaws(t *testing.T) {
	a, _ := newPause(map[string]any{"match": "my-api"})
	acts := a.(*pauseAntic).Recovery()
	if len(acts) != 1 {
		t.Fatalf("Recovery returned %d actions, want 1", len(acts))
	}
	want := []string{"pkill", "-CONT", "-f", "my-api"}
	if len(acts[0].Argv) != len(want) {
		t.Fatalf("recovery argv = %v, want %v", acts[0].Argv, want)
	}
	for i := range want {
		if acts[0].Argv[i] != want[i] {
			t.Errorf("recovery argv = %v, want %v", acts[0].Argv, want)
		}
	}
}
