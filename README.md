# Antics

**Misbehave, responsibly.**

Antics breaks your own stack on purpose. It kills processes, eats disk, pegs CPU, and hogs memory, so you find out how your system fails *before* your users do. 

It's chaos engineering for indie devs: no Kubernetes, no platform team, no budget. Just a single binary you point at your own machine on a Friday afternoon.

And it always cleans up after itself. Whatever Antics breaks, Antics puts back, even if you hit Ctrl-C halfway through.

```
  ANTICS  — misbehave, responsibly.
  scenario: api-meltdown

  plotting kill
    → hunt down processes matching "my-api" and kill them every 5s
  ● live     kill
  plotting diskfill
    → write a 500 MB junk file to /tmp/antics-diskfill.junk to eat disk space
  ● live     diskfill

  ⏱ holding the chaos for 30s ..............................

  ✓ cleaning up after ourselves
    ✓ diskfill restored
    ✓ kill restored

  done. the coast is clear. nothing left misbehaving.
```

## Why

Most teams learn how their system fails by watching it fail in front of real users at 2am. The database pool exhausts. A slow dependency cascades into a full outage because nothing had a timeout. The disk fills. 

These failures are all *knowable* — but chaos engineering tools (Gremlin, Chaos Mesh, Litmus) assume you're a big company with a cluster and a platform team.

Antics is the opposite. It's local-first, single-binary, and approachable enough that one developer can try it in five minutes. "Run some antics against staging" is an invitation. "Conduct a chaos engineering experiment" is a project nobody starts.

## Install

Download the binary for your platform from [Releases](https://github.com/1999labs/antics/releases), or build from source:

```sh
git clone https://github.com/1999labs/antics
cd antics
go build -o antics ./cmd/
```

No dependencies. No runtime. One file.

## Quickstart

```sh
# see what antics are available
antics list

# write a starter scenario you can edit
antics init

# see what it would do — commits nothing
antics run starter.antics --dry-run

# actually do it (and watch it clean up after)
antics run starter.antics --hold 30s
```

## Scenarios

A scenario is a batch of antics in a tiny config file. This is [`examples/api-meltdown.antics`](examples/api-meltdown.antics) — the scenario whose run is shown at the top of this README:

```ini
name: api-meltdown

# kill your service every 5 seconds — does it come back?
[kill]
match: my-api
every: 5s

# eat half a gig of disk while it flaps
[diskfill]
megabytes: 500
```

The `name:` field is what the banner echoes back as `scenario: api-meltdown` when you run it:

```sh
antics run examples/api-meltdown.antics
```

Antics commits each antic, holds the chaos for `--hold`, then restores everything in reverse order.

Stack as many antics as you want in one file — `kill`, `diskfill`, `cpuhog`, and `memhog` in any combination. They're committed top to bottom and then all run *at the same time* for the duration of the hold, so you can recreate a cascading failure (a service flapping *while* the disk fills *while* the CPU is pegged) instead of one fault at a time.

More ready-to-run scenarios live in [`examples/`](examples/):

- [`crash-loop`](examples/crash-loop.antics) — a service that keeps dying on a tight loop; does your supervisor bring it back?
- [`disk-panic`](examples/disk-panic.antics) — the classic "disk filled at 2am" failure, in isolation
- [`noisy-neighbor`](examples/noisy-neighbor.antics) — a runaway process starving everything else of CPU and memory

## The antics

| antic      | what it does                                  | params                          |
|------------|-----------------------------------------------|---------------------------------|
| `kill`     | kills processes matching a name               | `match`, `every` (optional)     |
| `diskfill` | writes a junk file to eat disk, then deletes it | `megabytes`, `dir` (optional) |
| `cpuhog`   | pegs N cores with busy loops                  | `cores`                         |
| `memhog`   | allocates and holds memory                    | `megabytes`                     |

More antics — network latency, packet blackholing — are coming per-platform. They're OS-specific (macOS, Linux, and Windows each break differently), so they land one platform at a time rather than half-working everywhere.

## Cleanup

**Antics always cleans up after itself.** Every antic that breaks something knows how to put it back, teardown runs even on Ctrl-C or a crash, and `--dry-run` lets you see exactly what will happen before anything does. This is harmless collective misbehavior. The harmless part is not optional.

## Platform support

Built and tested on macOS. Linux and Windows binaries are cross-compiled from the same codebase; the four antics above are portable across all three. The network antics (latency, blackhole) will be added per-platform.

## License

MIT. Misbehave freely.
