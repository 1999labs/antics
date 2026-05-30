package antic

import (
	"strings"
	"testing"
)

func TestLatencyRejectsMissingMs(t *testing.T) {
	if _, err := newLatency(map[string]any{}); err == nil {
		t.Error("expected error when ms is omitted")
	}
}

func TestLatencyRejectsNonPositiveMs(t *testing.T) {
	if _, err := newLatency(map[string]any{"ms": 0}); err == nil {
		t.Error("expected error for ms: 0")
	}
	if _, err := newLatency(map[string]any{"ms": -5}); err == nil {
		t.Error("expected error for negative ms")
	}
}

func TestLatencyBuildsWithMsAndDev(t *testing.T) {
	a, err := newLatency(map[string]any{"ms": 100, "dev": "eth0"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "latency" {
		t.Errorf("name = %q, want latency", a.Name())
	}
	d := a.Describe()
	for _, want := range []string{"100ms", "eth0", "outbound"} {
		if !strings.Contains(d, want) {
			t.Errorf("Describe()=%q, missing %q", d, want)
		}
	}
}

func TestLatencyDevOptional(t *testing.T) {
	a, err := newLatency(map[string]any{"ms": 50})
	if err != nil {
		t.Fatal(err)
	}
	l := a.(*latencyAntic)
	if l.dev != "" || l.ms != 50 {
		t.Errorf("dev=%q ms=%d, want dev empty (auto-detect) and ms 50", l.dev, l.ms)
	}
}

// dev: 0 arrives from the parser as an int; latency must stringify it.
func TestLatencyNumericDevCoerced(t *testing.T) {
	a, err := newLatency(map[string]any{"ms": 10, "dev": 0})
	if err != nil {
		t.Fatal(err)
	}
	if l := a.(*latencyAntic); l.dev != "0" {
		t.Errorf("dev = %q, want \"0\"", l.dev)
	}
}

func TestLatencyRestoreSafeBeforeCommit(t *testing.T) {
	a, err := newLatency(map[string]any{"ms": 100, "dev": "eth0"})
	if err != nil {
		t.Fatal(err)
	}
	// Never committed (added==false): Restore must be a no-op, never shelling
	// out to delete a qdisc we didn't create.
	if err := a.Restore(); err != nil {
		t.Errorf("Restore before Commit: %v", err)
	}
}

func TestLatencyRecoveryEmptyUntilAdded(t *testing.T) {
	a, _ := newLatency(map[string]any{"ms": 100, "dev": "eth0"})
	l := a.(*latencyAntic)
	if acts := l.Recovery(); acts != nil {
		t.Errorf("Recovery before add = %v, want nil", acts)
	}
	l.added = true
	acts := l.Recovery()
	want := []string{"tc", "qdisc", "del", "dev", "eth0", "root"}
	if len(acts) != 1 || !equalStrings(acts[0].Argv, want) {
		t.Errorf("Recovery after add = %v, want exec %v", acts, want)
	}
}

func TestParseDefaultDev(t *testing.T) {
	cases := map[string]string{
		"default via 192.168.1.1 dev en0 proto dhcp metric 100":        "en0",
		"default dev wg0 scope link":                                   "wg0",
		"default via 10.0.0.1 dev eth0\ndefault via 10.0.1.1 dev eth1": "eth0", // first wins
	}
	for in, want := range cases {
		got, err := parseDefaultDev(in)
		if err != nil || got != want {
			t.Errorf("parseDefaultDev(%q) = %q,%v want %q", in, got, err, want)
		}
	}
	if _, err := parseDefaultDev(""); err == nil {
		t.Error("expected error for empty route output")
	}
}

func TestIsNoQdisc(t *testing.T) {
	yes := []string{
		"RTNETLINK answers: No such file or directory",
		"Error: Cannot delete qdisc with handle of zero.",
	}
	for _, s := range yes {
		if !isNoQdisc(s) {
			t.Errorf("isNoQdisc(%q) = false, want true", s)
		}
	}
	if isNoQdisc("RTNETLINK answers: Operation not permitted") {
		t.Error("a permission error must not be classified as 'no qdisc'")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
