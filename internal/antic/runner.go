package antic

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/noahmclaughlin/antics/internal/ui"
)

// Run executes a scenario: commit every antic, hold the chaos for `hold`, then
// restore everything — guaranteed. Cleanup runs even if the user hits Ctrl-C
// or an antic panics mid-run. This guarantee is the whole reason Antics is safe
// to point at your own machine.
//
// If dryRun is true, nothing is committed; we just narrate what would happen.
func Run(sc *Scenario, hold time.Duration, dryRun bool) error {
	ui.Banner(sc.Name, dryRun)

	if dryRun {
		for _, a := range sc.Antics {
			ui.Plotting(a.Name(), a.Describe())
		}
		ui.Info("(dry run — no mischief committed)")
		ui.Done()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// toRestore tracks every antic we BEGIN committing — registered before its
	// Commit runs — so teardown restores it even if that Commit fails halfway.
	// Restore is contractually safe to call after a partial or zero commit, so
	// this is what keeps the "always cleans up" promise in the failure case.
	var toRestore []Antic

	// teardown is deferred immediately so it runs no matter how we exit:
	// normal finish, Ctrl-C, or panic. Restoring in reverse order undoes the
	// most recent mischief first.
	defer func() {
		ui.Restoring()
		for i := len(toRestore) - 1; i >= 0; i-- {
			a := toRestore[i]
			if err := a.Restore(); err != nil {
				ui.RestoreFailed(a.Name(), err)
			} else {
				ui.Restored(a.Name())
			}
		}
		ui.Done()
	}()

	// Catch Ctrl-C so the deferred teardown still runs instead of the process
	// being killed outright.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	interrupted := make(chan struct{})
	go func() {
		select {
		case <-sig:
			ui.Info("caught interrupt — wrapping up early")
			// Stop catching signals so a second Ctrl-C hard-kills the process
			// if teardown ever hangs. The user always keeps an escape hatch.
			signal.Stop(sig)
			cancel()
			close(interrupted)
		case <-ctx.Done():
		}
	}()

	// Commit each antic in order. Each is registered for teardown BEFORE its
	// Commit runs (see toRestore above), so a Commit that fails partway —
	// a diskfill that genuinely fills the disk and then errors — still gets
	// Restored instead of leaking its mess.
	for _, a := range sc.Antics {
		// Stop unleashing new antics once we've been interrupted mid-batch.
		if ctx.Err() != nil {
			break
		}
		ui.Plotting(a.Name(), a.Describe())
		toRestore = append(toRestore, a)
		if err := a.Commit(ctx); err != nil {
			ui.Errorf("%s failed to commit: %v", a.Name(), err)
			return err
		}
		ui.Committed(a.Name())
	}

	// If an interrupt arrived during commit, skip the hold and go straight to
	// the deferred teardown.
	if ctx.Err() != nil {
		return nil
	}

	// Hold the chaos, ticking once a second, until the timer or an interrupt.
	ui.Holding(hold)
	deadline := time.After(hold)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			return nil
		case <-interrupted:
			return nil
		case <-tick.C:
			ui.Tick()
		}
	}
}
