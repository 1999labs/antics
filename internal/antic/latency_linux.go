package antic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/1999labs/antics/internal/journal"
)

// latencyAntic injects outbound network delay on a Linux interface using tc
// netem. It is Linux-only (tc ships in iproute2 and is Linux-specific) and
// needs root. The delay lives in the kernel until Restore — or, after a hard
// kill, the crash-recovery journal — removes the netem qdisc.
type latencyAntic struct {
	ms    int
	dev   string
	added bool
}

func init() { Register("latency", newLatency) }

func newLatency(params map[string]any) (Antic, error) {
	// A negative sentinel distinguishes "ms omitted" from an explicit "ms: 0".
	ms := paramInt(params, "ms", -1)
	if ms < 0 {
		return nil, fmt.Errorf("latency antic: ms is required (the delay in milliseconds)")
	}
	if ms == 0 {
		return nil, fmt.Errorf("latency antic: ms must be positive")
	}
	// dev is optional; auto-detected at Commit if empty. The parser coerces an
	// all-digit value to int, so accept that too (real interfaces aren't numeric,
	// but be robust).
	dev := ""
	if v, ok := params["dev"]; ok {
		switch s := v.(type) {
		case string:
			dev = s
		case int:
			dev = fmt.Sprintf("%d", s)
		}
	}
	return &latencyAntic{ms: ms, dev: dev}, nil
}

func (l *latencyAntic) Name() string { return "latency" }

func (l *latencyAntic) Describe() string {
	where := l.dev
	if where == "" {
		where = "the default interface"
	}
	return fmt.Sprintf("add %dms of delay to outbound traffic on %s", l.ms, where)
}

func (l *latencyAntic) Commit(ctx context.Context) error {
	// Check privileges first so the user gets a clear message instead of a
	// cryptic "Operation not permitted" from tc.
	if os.Geteuid() != 0 {
		return fmt.Errorf("latency antic: needs root to run tc (try sudo)")
	}
	if _, err := exec.LookPath("tc"); err != nil {
		return fmt.Errorf("latency antic: tc not found (it ships in iproute2)")
	}
	if l.dev == "" {
		dev, err := defaultDev()
		if err != nil {
			return fmt.Errorf("latency antic: %w", err)
		}
		l.dev = dev
	}
	out, err := exec.Command("tc", "qdisc", "add", "dev", l.dev, "root", "netem", "delay", fmt.Sprintf("%dms", l.ms)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("latency antic: tc add failed: %s (if netem is missing, try: modprobe sch_netem)", strings.TrimSpace(string(out)))
	}
	l.added = true // only now do we own a qdisc to tear down
	return nil
}

func (l *latencyAntic) Restore() error {
	// Safe after a partial/zero commit: if we never added a qdisc, do nothing —
	// never delete a qdisc Antics didn't create.
	if !l.added || l.dev == "" {
		return nil
	}
	out, err := exec.Command("tc", "qdisc", "del", "dev", l.dev, "root").CombinedOutput()
	if err != nil {
		if isNoQdisc(string(out)) {
			return nil // already gone (idempotent / called twice)
		}
		return fmt.Errorf("latency cleanup failed, remove it manually with `tc qdisc del dev %s root`: %s", l.dev, strings.TrimSpace(string(out)))
	}
	return nil
}

// Recovery removes the netem qdisc if Antics is hard-killed before Restore
// runs. Only armed once the qdisc was actually added (and the device resolved),
// so a future run never tries to delete config Antics didn't create.
func (l *latencyAntic) Recovery() []journal.Action {
	if !l.added || l.dev == "" {
		return nil
	}
	return []journal.Action{journal.Exec("tc", "qdisc", "del", "dev", l.dev, "root")}
}

func defaultDev() (string, error) {
	if _, err := exec.LookPath("ip"); err != nil {
		return "", fmt.Errorf("ip not found (it ships in iproute2); pass dev: <iface>")
	}
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not read default route: %s", strings.TrimSpace(string(out)))
	}
	return parseDefaultDev(string(out))
}

// parseDefaultDev pulls the interface name out of `ip route show default`
// output by scanning for the literal "dev" token (its field position varies
// across distros). The first default route wins; multi-homed hosts should pass
// dev: explicitly.
func parseDefaultDev(routeOut string) (string, error) {
	for _, line := range strings.Split(routeOut, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	}
	return "", fmt.Errorf("no default route found; pass dev: <iface>")
}

// isNoQdisc reports whether a `tc qdisc del` failure just means there was no
// qdisc to delete — the safe, already-clean case — rather than a real error.
// We key on tc's message text rather than its exit code on purpose: the
// missing-qdisc case exits 2 (not 1), so there's no clean exit code to match
// (unlike blackhole's iptables -D, which exits 1). Don't "fix" this to an
// exit-code check.
func isNoQdisc(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "no such file") ||
		strings.Contains(o, "cannot delete qdisc")
}
