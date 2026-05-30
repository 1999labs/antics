// Package ui handles the narrated, slightly-mischievous output that makes
// Antics feel like a toy rather than a script. All terminal writing goes
// through here so the voice stays consistent.
package ui

import (
	"fmt"
	"os"
	"time"
)

// ANSI colors, kept minimal. Disabled automatically when output isn't a TTY
// (e.g. piped into a file or CI), so logs stay clean.
var (
	enabled = isTTY()

	dim    = wrap("\033[2m")
	red    = wrap("\033[31m")
	yellow = wrap("\033[33m")
	green  = wrap("\033[32m")
	cyan   = wrap("\033[36m")
	bold   = wrap("\033[1m")
)

func wrap(code string) func(string) string {
	return func(s string) string {
		if !enabled {
			return s
		}
		return code + s + "\033[0m"
	}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Banner prints the scenario header.
func Banner(scenario string, dryRun bool) {
	fmt.Println()
	fmt.Println(bold(cyan("  ANTICS")) + dim("  — misbehave, responsibly."))
	if dryRun {
		fmt.Println(dim("  dry run: describing mischief, committing none of it"))
	}
	fmt.Printf("  scenario: %s\n\n", bold(scenario))
}

// Plotting announces an antic that's about to run.
func Plotting(name, describe string) {
	fmt.Printf("  %s %s\n", yellow("plotting"), name)
	fmt.Printf("    %s\n", dim("→ "+describe))
}

// Committed confirms an antic was unleashed.
func Committed(name string) {
	fmt.Printf("  %s  %s\n", red("● live   "), name)
}

// Holding shows the dwell timer.
func Holding(d time.Duration) {
	fmt.Printf("\n  %s holding the chaos for %s ", yellow("⏱"), bold(d.String()))
}

// Tick prints a dot per second during the hold, for liveness.
func Tick() { fmt.Print(dim(".")) }

// Restoring announces teardown.
func Restoring() {
	fmt.Printf("\n\n  %s cleaning up after ourselves\n", green("✓"))
}

// Restored confirms one antic was undone.
func Restored(name string) {
	fmt.Printf("    %s %s\n", green("✓"), dim(name+" restored"))
}

// RestoreFailed warns that cleanup didn't fully succeed — the one thing Antics
// must never do silently.
func RestoreFailed(name string, err error) {
	fmt.Printf("    %s %s: %v\n", red("✗"), name, err)
}

// Done prints the closing line.
func Done() {
	fmt.Printf("\n  %s the coast is clear. nothing left misbehaving.\n\n", green(bold("done.")))
}

// Errorf prints an error in the house style and returns it for convenience.
func Errorf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "  %s %s\n", red("!"), fmt.Sprintf(format, a...))
}

// Info prints a plain dim line.
func Info(s string) { fmt.Printf("  %s\n", dim(s)) }
