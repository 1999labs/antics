# Antics

**Misbehave, responsibly.**

Antics breaks your own stack on purpose. It kills and freezes processes, eats disk, thrashes I/O, pegs CPU, hogs memory, and exhausts file descriptors, so you find out how your system fails *before* your users do. 

It's chaos engineering for indie devs: no Kubernetes, no platform team, no budget. Just a single binary you point at your own machine on a Friday afternoon.

And it always cleans up after itself. Whatever Antics breaks, Antics puts back, even if you hit Ctrl-C halfway through.

```
  ANTICS — misbehave, responsibly.
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

Stack as many antics as you want in one file — combine `kill`, `pause`, `diskfill`, `iohog`, `cpuhog`, `memhog`, and `fdleak` (plus `latency` and `blackhole` on Linux) in any combination. They're committed top to bottom and then all run *at the same time* for the duration of the hold, so you can recreate a cascading failure (a service flapping *while* the disk fills *while* the CPU is pegged) instead of one fault at a time.

More ready-to-run scenarios live in [`examples/`](examples/):

- [`crash-loop`](examples/crash-loop.antics) — a service that keeps dying on a tight loop; does your supervisor bring it back?
- [`disk-panic`](examples/disk-panic.antics) — the classic "disk filled at 2am" failure, in isolation
- [`noisy-neighbor`](examples/noisy-neighbor.antics) — a runaway process starving everything else of CPU and memory

## The antics

Seven antics run anywhere Antics does (macOS and Linux), need no privileges, and clean up after themselves:

| antic      | what it does                                            | params                        |
|------------|---------------------------------------------------------|-------------------------------|
| `kill`     | kills processes matching a name                         | `match`, `every` (optional)   |
| `pause`    | freezes matching processes (SIGSTOP), thaws on cleanup  | `match`                       |
| `diskfill` | writes a junk file to eat disk, then deletes it         | `megabytes`, `dir` (optional) |
| `iohog`    | thrashes disk I/O bandwidth with a fixed-size file      | `megabytes`, `dir` (optional) |
| `cpuhog`   | pegs N cores with busy loops                            | `cores`                       |
| `memhog`   | allocates and holds memory                              | `megabytes`                   |
| `fdleak`   | holds N file descriptors open                           | `count`                       |

Six of them undo their effect on teardown — even on Ctrl-C. `kill` is the only exception: a killed process stays killed, so there's nothing to put back (though if it has a supervisor, that's the point — does recovery actually work?).

### Network antics (Linux only, needs root)

Two more antics break the network. They're **Linux-only** — they drive `tc` and `iptables`, which are Linux-specific — and need root:

| antic       | what it does                                      | params                 |
|-------------|---------------------------------------------------|------------------------|
| `latency`   | adds delay to outbound traffic (`tc` netem)       | `ms`, `dev` (optional) |
| `blackhole` | drops outbound packets to a host/port (`iptables`)| `host` and/or `port`   |

Because these change system-wide OS state, a hard crash could otherwise leave a rule behind — see [Cleanup](#cleanup) for how Antics guards against that.

## Cleanup

**Antics always cleans up after itself.** Six of the seven portable antics undo their effect automatically when the hold ends, and teardown runs in reverse order even if you hit Ctrl-C or an antic panics. (`kill` is the exception only because a killed process has nothing to put back.) `--dry-run` lets you see exactly what will happen before anything does.

And if Antics is *hard*-killed — `kill -9`, an OOM, a power cut — so teardown never runs? Before any antic that touches the disk, a process, or the network does its thing, Antics writes a tiny recovery journal. The next `antics run` reads it and finishes the cleanup automatically; you can also run `antics restore` yourself. So a leftover junk file, a frozen process, or a network rule gets reversed even across a crash.

Antics produce harmless misbehavior. The harmless part is not optional.

## Platform support

The seven portable antics run on **macOS and Linux** — developed on macOS, built and tested on Linux in CI, and cross-compiled to macOS (arm64 and Intel) and Linux from the same codebase, with no dependencies and no runtime. The two network antics (`latency`, `blackhole`) are **Linux-only** and need root, because they drive `tc` and `iptables`.

## License

MIT. Misbehave freely.
