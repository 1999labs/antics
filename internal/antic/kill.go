package antic

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// killAntic finds processes whose command line matches a pattern and kills
// them, optionally on a repeating loop for the duration of the run (simulating
// a service that keeps crashing). There is nothing to Restore — a killed
// process stays killed — but if your service has a supervisor (systemd, pm2,
// docker restart policy), this antic tests whether that recovery actually works.
type killAntic struct {
	pattern string
	every   time.Duration // if >0, re-kill on this interval
	cancel  context.CancelFunc
}

func init() { Register("kill", newKill) }

func newKill(params map[string]any) (Antic, error) {
	pattern, err := paramString(params, "match")
	if err != nil {
		return nil, fmt.Errorf("kill antic: %w", err)
	}
	if pattern == "" {
		return nil, fmt.Errorf("kill antic: match cannot be empty (it would match every process)")
	}
	// pkill -f matches against the full command line, which includes antics'
	// own invocation (e.g. the scenario path). Refuse a pattern that would
	// match us, so the kill antic can never take down the very process meant
	// to clean everything up afterward.
	if strings.Contains(strings.Join(os.Args, " "), pattern) {
		return nil, fmt.Errorf("kill antic: match %q matches antics' own command line; refusing so we don't kill ourselves mid-run", pattern)
	}
	every, err := paramDuration(params, "every", 0)
	if err != nil {
		return nil, fmt.Errorf("kill antic: %w", err)
	}
	return &killAntic{pattern: pattern, every: every}, nil
}

func (k *killAntic) Name() string { return "kill" }

func (k *killAntic) Describe() string {
	if k.every > 0 {
		return fmt.Sprintf("hunt down processes matching %q and kill them every %s", k.pattern, k.every)
	}
	return fmt.Sprintf("hunt down processes matching %q and kill them once", k.pattern)
}

func (k *killAntic) Commit(ctx context.Context) error {
	if err := k.killOnce(); err != nil {
		return err
	}
	if k.every > 0 {
		ctx, k.cancel = context.WithCancel(ctx)
		go func() {
			t := time.NewTicker(k.every)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					_ = k.killOnce()
				}
			}
		}()
	}
	return nil
}

func (k *killAntic) Restore() error {
	if k.cancel != nil {
		k.cancel()
	}
	return nil
}

func (k *killAntic) killOnce() error {
	// pkill exists on macOS and Linux; on Windows we fall back to taskkill.
	if runtime.GOOS == "windows" {
		// /F force, /IM image name, /FI filter on window/command is limited,
		// so we match on image name. Best-effort on Windows for v1.
		out, err := exec.Command("taskkill", "/F", "/IM", k.pattern).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "not found") {
			return fmt.Errorf("taskkill: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	// -f matches against the full command line, which is what people expect.
	out, err := exec.Command("pkill", "-f", k.pattern).CombinedOutput()
	if err != nil {
		// pkill exits 1 when nothing matched — that's not an error for us.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("pkill: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
