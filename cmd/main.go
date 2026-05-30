// Command antics is a local-first chaos CLI: a sandbox for misbehaving
// responsibly. It breaks your own stack on purpose — and always cleans up after
// itself — so you find out how your system fails before your users do.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/1999labs/antics/internal/antic"
	"github.com/1999labs/antics/internal/journal"
	"github.com/1999labs/antics/internal/ui"
)

const usage = `antics — misbehave, responsibly.

usage:
  antics run <scenario-file> [--hold 30s] [--dry-run]
  antics list
  antics init [path]
  antics restore

commands:
  run      unleash a scenario of antics, hold the chaos, then clean up
  list     show every antic available to misbehave with
  init     write a starter scenario file you can edit
  restore  clean up anything a previously-crashed run left behind

run flags:
  --hold     how long to hold the chaos before restoring (default 30s)
  --dry-run  describe the mischief without committing any of it

antics always cleans up after itself. nothing is left misbehaving.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "list":
		cmdList()
	case "init":
		cmdInit(os.Args[2:])
	case "restore":
		cmdRestore()
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		ui.Errorf("unknown command %q", os.Args[1])
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	hold := fs.Duration("hold", 30*time.Second, "how long to hold the chaos")
	dry := fs.Bool("dry-run", false, "describe the mischief without committing it")

	// Go's flag package stops at the first positional arg, so `run file --dry-run`
	// would silently ignore the flag. Reorder so all flags come first, letting
	// users put the scenario file and flags in any order.
	flags, positionals := splitArgs(args)
	_ = fs.Parse(append(flags, positionals...))

	if fs.NArg() < 1 {
		ui.Errorf("run needs a scenario file: antics run my-scenario.antics")
		os.Exit(1)
	}

	// Before unleashing new mischief, finish cleaning up after any prior run
	// that was hard-killed before its teardown ran. Skipped on --dry-run, which
	// must change nothing.
	if !*dry {
		recoverLeftovers()
	}

	sc, err := antic.LoadScenario(fs.Arg(0))
	if err != nil {
		ui.Errorf("%v", err)
		os.Exit(1)
	}

	if err := antic.Run(sc, *hold, *dry); err != nil {
		os.Exit(1)
	}
}

// cmdRestore cleans up anything a previously-crashed run left behind, for users
// who want to tidy up without starting a fresh scenario.
func cmdRestore() {
	notes, err := journal.Recover()
	if err != nil {
		ui.Errorf("%v", err)
		os.Exit(1)
	}
	if len(notes) == 0 {
		ui.Info("nothing to recover — no leftover mischief from a crashed run")
		return
	}
	for _, n := range notes {
		ui.Info(n)
	}
}

// recoverLeftovers replays any crash-recovery journal from a hard-killed prior
// run, narrating what it cleaned up. Errors are reported but never block a run.
func recoverLeftovers() {
	notes, err := journal.Recover()
	if err != nil {
		ui.Errorf("crash-recovery check failed: %v", err)
		return
	}
	for _, n := range notes {
		ui.Info(n)
	}
}

// splitArgs separates --flags (and their values) from positional arguments so
// they can be reordered before flag.Parse, which otherwise stops at the first
// positional. --dry-run is boolean; --hold takes a value.
func splitArgs(args []string) (flags, positionals []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--hold" || a == "-hold":
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case len(a) > 0 && a[0] == '-':
			// boolean or --flag=value form
			flags = append(flags, a)
		default:
			positionals = append(positionals, a)
		}
	}
	return flags, positionals
}

func cmdList() {
	fmt.Print("\n  antics available to misbehave with:\n\n")
	for _, name := range antic.Available() {
		fmt.Printf("    %s\n", name)
	}
	fmt.Println("\n  put them in a scenario file (see `antics init`) and run with `antics run`.")
	fmt.Println()
}

func cmdInit(args []string) {
	path := "starter.antics"
	if len(args) > 0 {
		path = args[0]
	}
	if _, err := os.Stat(path); err == nil {
		ui.Errorf("%s already exists — refusing to overwrite it", path)
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(starter), 0644); err != nil {
		ui.Errorf("couldn't write %s: %v", path, err)
		os.Exit(1)
	}
	ui.Info(fmt.Sprintf("wrote %s — edit it, then run: antics run %s --dry-run", path, path))
}

const starter = `# a starter batch of antics. edit freely, then:
#   antics run starter.antics --dry-run   (see what it would do)
#   antics run starter.antics             (actually do it)
name: starter-mischief

# eat some disk space, then give it back
[diskfill]
megabytes: 200

# peg one CPU core
[cpuhog]
cores: 1

# hold ~256 MB of memory
[memhog]
megabytes: 256

# kill anything matching this name (uncomment and set a real match first!)
# [kill]
# match: my-service
# every: 5s
`
