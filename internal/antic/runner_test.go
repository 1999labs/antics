package antic

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeAntic records the order of Commit/Restore calls so tests can assert the
// runner honors its cleanup contract without touching the real machine.
type fakeAntic struct {
	name      string
	commitErr error
	events    *[]string
}

func (f *fakeAntic) Name() string     { return f.name }
func (f *fakeAntic) Describe() string { return "fake " + f.name }
func (f *fakeAntic) Commit(ctx context.Context) error {
	*f.events = append(*f.events, "commit:"+f.name)
	return f.commitErr
}
func (f *fakeAntic) Restore() error {
	*f.events = append(*f.events, "restore:"+f.name)
	return nil
}

func TestRunRestoresInReverseOrder(t *testing.T) {
	var events []string
	sc := &Scenario{Name: "t", Antics: []Antic{
		&fakeAntic{name: "a", events: &events},
		&fakeAntic{name: "b", events: &events},
		&fakeAntic{name: "c", events: &events},
	}}
	if err := Run(sc, time.Millisecond, false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertEvents(t, events, []string{
		"commit:a", "commit:b", "commit:c",
		"restore:c", "restore:b", "restore:a",
	})
}

// The core guarantee: an antic whose Commit fails partway is STILL restored, so
// it can never leak its mess — e.g. a diskfill that genuinely fills the disk and
// then errors. This is the bug the audit found; this test locks the fix in.
func TestRunRestoresAnticThatFailedToCommit(t *testing.T) {
	var events []string
	boom := errors.New("disk full")
	sc := &Scenario{Name: "t", Antics: []Antic{
		&fakeAntic{name: "a", events: &events},
		&fakeAntic{name: "b", commitErr: boom, events: &events},
		&fakeAntic{name: "c", events: &events}, // must never commit
	}}
	err := Run(sc, time.Millisecond, false)
	if !errors.Is(err, boom) {
		t.Fatalf("Run err = %v, want %v", err, boom)
	}
	assertEvents(t, events, []string{
		"commit:a", "commit:b", // c never runs
		"restore:b", "restore:a", // b is restored despite failing to commit
	})
}

func TestDryRunCommitsNothing(t *testing.T) {
	var events []string
	sc := &Scenario{Name: "t", Antics: []Antic{
		&fakeAntic{name: "a", events: &events},
	}}
	if err := Run(sc, time.Millisecond, true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("dry run touched things: %v", events)
	}
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}
