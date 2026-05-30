package antic

import (
	"os"
	"testing"
)

func TestKillBuildsWithSafePattern(t *testing.T) {
	a, err := newKill(map[string]any{"match": "zzz-nonexistent-service-zzz", "every": "5s"})
	if err != nil {
		t.Fatalf("newKill: %v", err)
	}
	if got := a.Name(); got != "kill" {
		t.Errorf("name = %q, want kill", got)
	}
}

// match: 8080 arrives from the parser as an int; the kill antic must accept it.
func TestKillAcceptsNumericMatch(t *testing.T) {
	if _, err := newKill(map[string]any{"match": 8080}); err != nil {
		t.Fatalf("newKill with numeric match: %v", err)
	}
}

func TestKillRefusesEmptyMatch(t *testing.T) {
	if _, err := newKill(map[string]any{"match": ""}); err == nil {
		t.Error("expected error for empty match")
	}
}

// A pattern that matches antics' own command line must be refused, so the kill
// antic can never take down the process responsible for cleaning up.
func TestKillRefusesToMatchItself(t *testing.T) {
	if _, err := newKill(map[string]any{"match": os.Args[0]}); err == nil {
		t.Error("expected refusal when pattern matches antics' own command line")
	}
}
