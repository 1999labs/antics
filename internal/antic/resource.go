package antic

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// cpuhogAntic spins busy loops on N cores to simulate a noisy-neighbor process
// pegging the CPU. It self-terminates when the run's context is cancelled, so
// there is nothing persistent to Restore.
type cpuhogAntic struct {
	cores  int
	cancel context.CancelFunc
}

func init() {
	Register("cpuhog", newCPUHog)
	Register("memhog", newMemHog)
}

func newCPUHog(params map[string]any) (Antic, error) {
	cores := paramInt(params, "cores", runtime.NumCPU())
	if cores < 1 {
		cores = 1
	}
	if cores > runtime.NumCPU() {
		cores = runtime.NumCPU()
	}
	return &cpuhogAntic{cores: cores}, nil
}

func (c *cpuhogAntic) Name() string { return "cpuhog" }

func (c *cpuhogAntic) Describe() string {
	return fmt.Sprintf("peg %d CPU core(s) with busy loops", c.cores)
}

func (c *cpuhogAntic) Commit(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)
	for i := 0; i < c.cores; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// tight loop; burns a core
				}
			}
		}()
	}
	return nil
}

func (c *cpuhogAntic) Restore() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// memhogAntic allocates and touches a big slab of memory and holds it for the
// duration of the run, simulating a memory leak or runaway allocation. Touching
// every page matters: lazy allocation means untouched memory is never actually
// committed by the OS, so we write to it to make the pressure real. The slab is
// released (and GC'd) on Restore.
type memhogAntic struct {
	mb   int
	mu   sync.Mutex
	slab [][]byte
}

func newMemHog(params map[string]any) (Antic, error) {
	mb := paramInt(params, "megabytes", 256)
	if mb <= 0 {
		return nil, fmt.Errorf("memhog antic: megabytes must be positive")
	}
	return &memhogAntic{mb: mb}, nil
}

func (m *memhogAntic) Name() string { return "memhog" }

func (m *memhogAntic) Describe() string {
	return fmt.Sprintf("allocate and hold %d MB of memory", m.mb)
}

func (m *memhogAntic) Commit(ctx context.Context) error {
	// Build into a local slab and only take the lock to publish it, so a large
	// allocation never blocks a concurrent Restore. Check ctx each iteration so
	// Ctrl-C during a multi-gig allocation stops promptly instead of forcing the
	// user to wait for the whole slab.
	slab := make([][]byte, 0, m.mb)
	for i := 0; i < m.mb; i++ {
		select {
		case <-ctx.Done():
			// Interrupted: publish what we allocated so Restore frees it, then stop.
			m.mu.Lock()
			m.slab = slab
			m.mu.Unlock()
			return nil
		default:
		}
		block := make([]byte, 1024*1024)
		// Touch each page so the OS actually commits it.
		for j := 0; j < len(block); j += 4096 {
			block[j] = 1
		}
		slab = append(slab, block)
	}
	m.mu.Lock()
	m.slab = slab
	m.mu.Unlock()
	return nil
}

func (m *memhogAntic) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slab = nil
	// Hint the runtime to give memory back to the OS promptly.
	runtime.GC()
	return nil
}
