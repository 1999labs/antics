package antic

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// maxFDLeak bounds how many descriptors fdleak will hold. The cap leaves
// headroom so the leak can never starve Antics' OWN descriptors — the crash
// journal, stdout/stderr — which it needs in order to narrate and clean up.
// That is why fdleak takes a bounded count rather than "open until failure".
const maxFDLeak = 4096

// fdleakAntic exhausts file descriptors by opening and holding a bounded number
// of them, simulating the classic "too many open files" failure. The handles
// are released on Restore, and the kernel reclaims them automatically when the
// process dies — so there is nothing to journal for crash recovery.
type fdleakAntic struct {
	count int
	mu    sync.Mutex
	held  []*os.File
}

func init() { Register("fdleak", newFdleak) }

func newFdleak(params map[string]any) (Antic, error) {
	count := paramInt(params, "count", 256)
	if count <= 0 {
		return nil, fmt.Errorf("fdleak antic: count must be positive")
	}
	if count > maxFDLeak {
		count = maxFDLeak
	}
	return &fdleakAntic{count: count}, nil
}

func (f *fdleakAntic) Name() string { return "fdleak" }

func (f *fdleakAntic) Describe() string {
	return fmt.Sprintf("hold %d open file descriptors hostage", f.count)
}

func (f *fdleakAntic) Commit(ctx context.Context) error {
	held := make([]*os.File, 0, f.count)
	for i := 0; i < f.count; i++ {
		select {
		case <-ctx.Done():
			f.publish(held)
			return nil
		default:
		}
		h, err := os.Open(os.DevNull)
		if err != nil {
			// We hit the real OS limit before our target. A partial leak is
			// still a valid leak — keep what we have rather than failing.
			break
		}
		held = append(held, h)
	}
	f.publish(held)
	return nil
}

// publish stores the open handles on the struct. Keeping them referenced (not
// just their int fds) prevents the GC's *os.File finalizer from closing them
// and silently un-leaking mid-run.
func (f *fdleakAntic) publish(held []*os.File) {
	f.mu.Lock()
	f.held = held
	f.mu.Unlock()
}

func (f *fdleakAntic) Restore() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.held {
		h.Close()
	}
	// Nil the slice so a second Restore (the idempotency contract) doesn't
	// double-close an already-closed handle.
	f.held = nil
	return nil
}
