// Package antic defines the core interface every mischief-maker implements
// and a registry so new antics can be plugged in without touching the runner.
package antic

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/1999labs/antics/internal/journal"
)

// Antic is a single piece of controlled misbehavior. Every antic must clean up
// after itself: whatever Commit breaks, Restore must put back. This is the
// contract that makes Antics safe to run on a Friday afternoon.
type Antic interface {
	// Name is the short identifier used in scenario files and the CLI.
	Name() string

	// Describe returns a one-line, human-readable summary of what this antic
	// is about to do, given its current params. Used by --dry-run and the
	// narrated output.
	Describe() string

	// Commit unleashes the mischief. It should return quickly; long-running
	// effects belong in a background goroutine that watches ctx for teardown.
	Commit(ctx context.Context) error

	// Restore undoes everything Commit did. It must be safe to call even if
	// Commit failed halfway, and safe to call more than once.
	Restore() error
}

// Journaler is implemented by antics that change state OUTSIDE this process — a
// file on disk, another process, OS-level network config — and so can't rely on
// Restore alone, because a hard kill (SIGKILL, power loss) skips Restore
// entirely. Such an antic returns the undo actions to persist in the
// crash-recovery journal, so a later run can finish the cleanup. Antics whose
// effects vanish with the process (cpuhog, memhog, fdleak) don't implement it.
//
// Recovery is consulted both before and after Commit: the actions known at
// construction time (a file path, a process pattern) are journaled before any
// mischief happens, while antics that only finalize their undo during Commit
// (the network antics, which resolve a device or binary path) are re-journaled
// once committed.
type Journaler interface {
	Recovery() []journal.Action
}

// Factory builds an antic from raw params parsed out of a scenario file.
type Factory func(params map[string]any) (Antic, error)

var registry = map[string]Factory{}

// Register makes an antic available by name. Called from each antic's init().
func Register(name string, f Factory) {
	registry[name] = f
}

// Build constructs an antic by name, or errors if no such antic exists.
func Build(name string, params map[string]any) (Antic, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown antic %q (try `antics list` to see what's available)", name)
	}
	return f(params)
}

// Available returns the names of all registered antics, sorted.
func Available() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// --- small shared helpers for pulling typed params out of the YAML soup ---

func paramDuration(params map[string]any, key string, def time.Duration) (time.Duration, error) {
	v, ok := params[key]
	if !ok {
		return def, nil
	}
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("%s must be a duration string like \"10s\"", key)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func paramString(params map[string]any, key string) (string, error) {
	v, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing required param %q", key)
	}
	// The scenario parser coerces all-digit values to ints, so a process name
	// like `match: 8080` arrives as an int. Accept that and stringify it rather
	// than rejecting a perfectly valid pattern.
	switch s := v.(type) {
	case string:
		return s, nil
	case int:
		return strconv.Itoa(s), nil
	case int64:
		return strconv.FormatInt(s, 10), nil
	}
	return "", fmt.Errorf("%s must be a string", key)
}

func paramInt(params map[string]any, key string, def int) int {
	v, ok := params[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}
