package antic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/1999labs/antics/internal/journal"
)

const (
	iohogDefaultMB = 64
	iohogMaxMB     = 1024
)

// iohogAntic saturates disk I/O bandwidth — not space — by continuously
// rewriting and re-reading a single fixed-size file with an fsync between, the
// way a runaway log writer or a thrashing database starves everything else of
// I/O. It reuses ONE bounded file (so it never fills the disk — that's
// diskfill's job) and deletes it on Restore.
type iohogAntic struct {
	path      string
	sizeBytes int64
	cancel    context.CancelFunc
	done      chan struct{} // closed when the I/O goroutine has stopped and closed its file
}

func init() { Register("iohog", newIohog) }

func newIohog(params map[string]any) (Antic, error) {
	dir := os.TempDir()
	if d, ok := params["dir"].(string); ok && d != "" {
		dir = d
	}
	mb := paramInt(params, "megabytes", iohogDefaultMB)
	if mb <= 0 {
		mb = iohogDefaultMB
	}
	if mb > iohogMaxMB {
		mb = iohogMaxMB
	}
	return &iohogAntic{
		path:      filepath.Join(dir, "antics-iohog.tmp"),
		sizeBytes: int64(mb) * 1024 * 1024,
	}, nil
}

func (i *iohogAntic) Name() string { return "iohog" }

func (i *iohogAntic) Describe() string {
	return fmt.Sprintf("thrash disk I/O with a %d MB scratch file at %s (bandwidth, not space)", i.sizeBytes/(1024*1024), i.path)
}

func (i *iohogAntic) Commit(ctx context.Context) error {
	f, err := os.Create(i.path)
	if err != nil {
		return fmt.Errorf("iohog: %w", err)
	}
	ctx, i.cancel = context.WithCancel(ctx)
	i.done = make(chan struct{})
	go func() {
		// Order matters: f.Close() runs first (registered last), then close(done)
		// signals Restore that the handle is released and it's safe to os.Remove.
		defer close(i.done)
		defer f.Close()
		buf := make([]byte, 1024*1024) // 1 MB I/O unit
		for {
			if ctx.Err() != nil {
				return
			}
			// Rewrite the whole file from the top, reusing the same file so it
			// never grows beyond sizeBytes.
			if _, err := f.Seek(0, 0); err != nil {
				return
			}
			for off := int64(0); off < i.sizeBytes; off += int64(len(buf)) {
				if ctx.Err() != nil {
					return
				}
				if _, err := f.Write(buf); err != nil {
					return // out of space or closed: stop rather than spin hot
				}
			}
			// fsync is what actually generates I/O pressure; a bare Write often
			// just lands in the page cache.
			if err := f.Sync(); err != nil {
				return
			}
			// Read it back to generate read I/O too.
			if _, err := f.Seek(0, 0); err != nil {
				return
			}
			for off := int64(0); off < i.sizeBytes; off += int64(len(buf)) {
				if ctx.Err() != nil {
					return
				}
				if _, err := f.Read(buf); err != nil {
					break
				}
			}
		}
	}()
	return nil
}

func (i *iohogAntic) Restore() error {
	if i.cancel != nil {
		i.cancel()
	}
	// Wait for the I/O goroutine to actually stop and close its file handle
	// before removing the file, so Restore returning truly means the mess is
	// gone (and os.Remove can't race an open handle). Bounded: the goroutine
	// checks ctx at each I/O step. The channel is closed, so repeat calls return
	// immediately, keeping Restore idempotent.
	if i.done != nil {
		<-i.done
	}
	err := os.Remove(i.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("iohog cleanup failed, you may need to delete %s manually: %w", i.path, err)
	}
	return nil
}

// Recovery lets the crash-recovery journal delete the scratch file if Antics is
// hard-killed before Restore runs. The path is known at construction time.
func (i *iohogAntic) Recovery() []journal.Action {
	return []journal.Action{journal.Delete(i.path)}
}
