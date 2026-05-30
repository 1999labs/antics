package antic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// diskfillAntic writes a big junk file to eat disk space, simulating the
// classic "disk filled at 2am and everything fell over" failure. Unlike kill,
// this one MUST clean up — leaving gigabytes of junk on someone's machine is
// exactly the kind of harm Antics promises never to do. The file lives in a
// known temp path so Restore can always find and delete it.
type diskfillAntic struct {
	mb   int
	path string
}

func init() { Register("diskfill", newDiskfill) }

func newDiskfill(params map[string]any) (Antic, error) {
	mb := paramInt(params, "megabytes", 100)
	if mb <= 0 {
		return nil, fmt.Errorf("diskfill antic: megabytes must be positive")
	}
	dir := os.TempDir()
	if d, ok := params["dir"].(string); ok && d != "" {
		dir = d
	}
	return &diskfillAntic{
		mb:   mb,
		path: filepath.Join(dir, "antics-diskfill.junk"),
	}, nil
}

func (d *diskfillAntic) Name() string { return "diskfill" }

func (d *diskfillAntic) Describe() string {
	return fmt.Sprintf("write a %d MB junk file to %s to eat disk space", d.mb, d.path)
}

func (d *diskfillAntic) Commit(ctx context.Context) error {
	f, err := os.Create(d.path)
	if err != nil {
		return fmt.Errorf("diskfill: %w", err)
	}
	defer f.Close()

	chunk := make([]byte, 1024*1024) // 1 MB
	for i := 0; i < d.mb; i++ {
		select {
		case <-ctx.Done():
			return nil // teardown will Restore; stop early
		default:
		}
		if _, err := f.Write(chunk); err != nil {
			// Out of space mid-write is arguably success for a disk-fill antic,
			// but we surface it so the user knows what happened.
			return fmt.Errorf("diskfill (wrote partial): %w", err)
		}
	}
	return nil
}

func (d *diskfillAntic) Restore() error {
	err := os.Remove(d.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("diskfill cleanup failed, you may need to delete %s manually: %w", d.path, err)
	}
	return nil
}
