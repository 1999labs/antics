package antic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/1999labs/antics/internal/journal"
)

// blackholeAntic drops outbound packets to a destination using an iptables
// OUTPUT DROP rule, simulating a dependency going dark — the connection just
// hangs and times out. It is Linux-only (iptables is Linux-specific) and needs
// root. The rule is removed on Restore, or — after a hard kill — by the
// crash-recovery journal replaying the exact reverse command.
type blackholeAntic struct {
	host     string
	port     int
	match    []string // the rule spec shared by -A (commit) and -D (restore)
	iptables string   // resolved binary path, captured for the journal
	added    bool
	run      func(args ...string) ([]byte, error) // exec seam for tests
}

func init() { Register("blackhole", newBlackhole) }

func newBlackhole(params map[string]any) (Antic, error) {
	host := ""
	if v, ok := params["host"]; ok {
		switch s := v.(type) {
		case string:
			host = s
		case int:
			host = strconv.Itoa(s)
		}
	}
	port := paramInt(params, "port", 0)
	if host == "" && port == 0 {
		return nil, fmt.Errorf("blackhole antic: set host, port, or both (a rule with neither would drop ALL outbound traffic and lock you out)")
	}
	if port != 0 && (port < 1 || port > 65535) {
		return nil, fmt.Errorf("blackhole antic: port %d out of range (1-65535)", port)
	}

	// Build the match spec ONCE and reuse it for both -A and -D, so iptables can
	// find and delete exactly the rule we added (it matches on the spec, and a
	// byte difference would leave the DROP in place forever).
	var match []string
	if host != "" {
		match = append(match, "-d", host)
	}
	if port != 0 {
		// --dport requires -p; default to tcp (the common dependency case).
		match = append(match, "-p", "tcp", "--dport", strconv.Itoa(port))
	}
	match = append(match, "-j", "DROP")

	b := &blackholeAntic{host: host, port: port, match: match}
	b.run = func(args ...string) ([]byte, error) {
		return exec.Command(b.iptables, args...).CombinedOutput()
	}
	return b, nil
}

func (b *blackholeAntic) Name() string { return "blackhole" }

func (b *blackholeAntic) Describe() string {
	var target string
	switch {
	case b.host != "" && b.port != 0:
		target = fmt.Sprintf("%s tcp port %d", b.host, b.port)
	case b.host != "":
		target = b.host
	default:
		target = fmt.Sprintf("tcp port %d", b.port)
	}
	return fmt.Sprintf("blackhole outbound traffic to %s (drop the packets)", target)
}

func (b *blackholeAntic) Commit(ctx context.Context) error {
	// Check privileges first so the user gets a clear message instead of a
	// cryptic iptables permission error (mirrors latency).
	if os.Geteuid() != 0 {
		return fmt.Errorf("blackhole antic: needs root to run iptables (try sudo)")
	}
	if err := b.resolveIptables(); err != nil {
		return err
	}
	out, err := b.run(append([]string{"-A", "OUTPUT"}, b.match...)...)
	if err != nil {
		// Unlike Restore, a non-zero exit here is a real failure (bad args, no
		// permission) and must be surfaced.
		return fmt.Errorf("blackhole antic: iptables add failed: %s", strings.TrimSpace(string(out)))
	}
	b.added = true
	return nil
}

func (b *blackholeAntic) Restore() error {
	// Never (successfully) committed: there is no rule of ours to delete, and we
	// must not remove a coincidentally-identical rule the user already had.
	// Mirrors latency's `!l.added` guard and our own Recovery() guard.
	if !b.added {
		return nil
	}
	out, err := b.run(append([]string{"-D", "OUTPUT"}, b.match...)...)
	if err != nil {
		if isAlreadyGone(err) {
			return nil // rule already gone: idempotent, safe to call twice
		}
		return fmt.Errorf("blackhole cleanup failed, remove it manually with `iptables -D OUTPUT %s`: %s", strings.Join(b.match, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// Recovery removes the DROP rule if Antics is hard-killed before Restore runs.
// Only armed once the rule was actually added and the binary path resolved, so
// a future run replays the exact same command via the same iptables binary.
func (b *blackholeAntic) Recovery() []journal.Action {
	if !b.added || b.iptables == "" {
		return nil
	}
	return []journal.Action{journal.Exec(append([]string{b.iptables, "-D", "OUTPUT"}, b.match...)...)}
}

func (b *blackholeAntic) resolveIptables() error {
	if b.iptables != "" {
		return nil
	}
	path, err := exec.LookPath("iptables")
	if err != nil {
		return fmt.Errorf("blackhole antic: iptables not found (this antic needs Linux with iptables installed)")
	}
	b.iptables = path
	return nil
}

// isAlreadyGone reports whether an iptables -D error just means the rule was
// already absent (exit 1, "does a matching rule exist?"), which is the safe,
// idempotent case — mirroring how kill treats pkill's exit 1.
func isAlreadyGone(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}
