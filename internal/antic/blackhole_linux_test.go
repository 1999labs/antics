package antic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestBlackholeRequiresHostOrPort(t *testing.T) {
	if _, err := newBlackhole(map[string]any{}); err == nil {
		t.Error("expected error when neither host nor port is set")
	}
}

func TestBlackholeHostOnlyRule(t *testing.T) {
	a, err := newBlackhole(map[string]any{"host": "1.2.3.4"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-d", "1.2.3.4", "-j", "DROP"}
	if got := a.(*blackholeAntic).match; !equalStrings(got, want) {
		t.Errorf("match = %v, want %v", got, want)
	}
}

func TestBlackholePortOnlyRule(t *testing.T) {
	a, err := newBlackhole(map[string]any{"port": 5432})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "tcp", "--dport", "5432", "-j", "DROP"}
	if got := a.(*blackholeAntic).match; !equalStrings(got, want) {
		t.Errorf("match = %v, want %v", got, want)
	}
}

func TestBlackholeHostAndPortRule(t *testing.T) {
	a, err := newBlackhole(map[string]any{"host": "1.2.3.4", "port": 5432})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-d", "1.2.3.4", "-p", "tcp", "--dport", "5432", "-j", "DROP"}
	if got := a.(*blackholeAntic).match; !equalStrings(got, want) {
		t.Errorf("match = %v, want %v", got, want)
	}
}

func TestBlackholePortRangeRejected(t *testing.T) {
	if _, err := newBlackhole(map[string]any{"port": 0, "host": ""}); err == nil {
		t.Error("expected error: neither host nor port")
	}
	if _, err := newBlackhole(map[string]any{"port": 70000}); err == nil {
		t.Error("expected error for out-of-range port")
	}
}

func TestBlackholeRecoveryMatchesRestore(t *testing.T) {
	a, _ := newBlackhole(map[string]any{"host": "1.2.3.4", "port": 5432})
	b := a.(*blackholeAntic)
	if acts := b.Recovery(); acts != nil {
		t.Errorf("Recovery before commit = %v, want nil", acts)
	}
	b.iptables = "/sbin/iptables"
	b.added = true
	want := append([]string{"/sbin/iptables", "-D", "OUTPUT"}, b.match...)
	acts := b.Recovery()
	if len(acts) != 1 || !equalStrings(acts[0].Argv, want) {
		t.Errorf("Recovery = %v, want exec %v", acts, want)
	}
}

// Restore must treat iptables -D exit 1 (rule already gone) as success, and be
// safe to call repeatedly.
func TestBlackholeRestoreIdempotentWhenRuleGone(t *testing.T) {
	a, _ := newBlackhole(map[string]any{"host": "1.2.3.4"})
	b := a.(*blackholeAntic)
	b.iptables = "iptables" // skip LookPath
	b.added = true          // pretend Commit succeeded so Restore exercises -D
	b.run = exitRun(1)
	if err := b.Restore(); err != nil {
		t.Errorf("Restore with rule gone (exit 1): %v", err)
	}
	if err := b.Restore(); err != nil {
		t.Errorf("second Restore: %v", err)
	}
}

func TestBlackholeRestoreSurfacesRealErrors(t *testing.T) {
	a, _ := newBlackhole(map[string]any{"host": "1.2.3.4"})
	b := a.(*blackholeAntic)
	b.iptables = "iptables"
	b.added = true
	b.run = exitRun(3) // e.g. permission denied
	if err := b.Restore(); err == nil {
		t.Error("expected Restore to surface a non-exit-1 error")
	}
}

// Commit refuses to touch iptables without root, with a clear pre-flight error
// (mirrors latency). Skipped if the suite happens to run as root.
func TestBlackholeCommitRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; can't exercise the non-root pre-flight")
	}
	a, _ := newBlackhole(map[string]any{"host": "1.2.3.4"})
	if err := a.Commit(context.Background()); err == nil {
		t.Error("expected Commit to require root")
	}
}

// Restore after a zero/partial commit (added==false) must be a no-op that never
// shells out — so it can never delete a rule Antics didn't create.
func TestBlackholeRestoreNoopWhenNotCommitted(t *testing.T) {
	a, _ := newBlackhole(map[string]any{"host": "1.2.3.4"})
	b := a.(*blackholeAntic)
	called := false
	b.run = func(args ...string) ([]byte, error) { called = true; return nil, nil }
	if err := b.Restore(); err != nil {
		t.Errorf("Restore before commit: %v", err)
	}
	if called {
		t.Error("Restore shelled out despite nothing being committed")
	}
}

// exitRun returns a fake exec that runs `sh -c 'exit <code>'`, producing a real
// *exec.ExitError with the given code so the exit-code classification is tested
// against genuine errors rather than hand-built ones.
func exitRun(code int) func(args ...string) ([]byte, error) {
	return func(args ...string) ([]byte, error) {
		return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).CombinedOutput()
	}
}
