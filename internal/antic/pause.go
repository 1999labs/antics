//go:build unix

package antic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/1999labs/antics/internal/journal"
)

// pauseAntic freezes processes matching a pattern with SIGSTOP and thaws them
// with SIGCONT on Restore — like kill, but reversible. A killed process
// restarts and recovers; a *frozen* one holds its connections open and hangs
// everything waiting on it, which is often the nastier, more revealing failure.
// It's Unix-only (it relies on pkill / SIGSTOP).
type pauseAntic struct {
	pattern string
}

func init() { Register("pause", newPause) }

func newPause(params map[string]any) (Antic, error) {
	pattern, err := paramString(params, "match")
	if err != nil {
		return nil, fmt.Errorf("pause antic: %w", err)
	}
	if pattern == "" {
		return nil, fmt.Errorf("pause antic: match cannot be empty (it would freeze every process)")
	}
	// Like kill, refuse a pattern that matches antics' own command line: a pause
	// that froze antics itself could never thaw it, and the deferred teardown
	// (the thing that resumes everything) would never run.
	if strings.Contains(strings.Join(os.Args, " "), pattern) {
		return nil, fmt.Errorf("pause antic: match %q matches antics' own command line; refusing so we don't freeze ourselves mid-run", pattern)
	}
	return &pauseAntic{pattern: pattern}, nil
}

func (p *pauseAntic) Name() string { return "pause" }

func (p *pauseAntic) Describe() string {
	return fmt.Sprintf("freeze processes matching %q (SIGSTOP), then thaw them on cleanup", p.pattern)
}

func (p *pauseAntic) Commit(ctx context.Context) error {
	return pkillSignal("-STOP", p.pattern)
}

func (p *pauseAntic) Restore() error {
	return pkillSignal("-CONT", p.pattern)
}

// Recovery thaws any still-frozen processes if Antics is hard-killed before
// Restore runs — otherwise they would stay suspended forever. The undo is known
// at construction time (just the pattern), so it's journaled before Commit.
func (p *pauseAntic) Recovery() []journal.Action {
	return []journal.Action{journal.Exec("pkill", "-CONT", "-f", p.pattern)}
}

// pkillSignal sends a signal (e.g. "-STOP", "-CONT") to every process whose
// full command line matches pattern. pkill exits 1 when nothing matched, which
// is not an error for us — resuming or signalling "nothing" is a no-op.
func pkillSignal(signal, pattern string) error {
	out, err := exec.Command("pkill", signal, "-f", pattern).CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("pkill %s: %s", signal, strings.TrimSpace(string(out)))
	}
	return nil
}
