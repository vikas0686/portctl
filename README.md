# portctl

**See and control what's listening on your local ports — without memorizing `lsof` flags.**

[![CI](https://github.com/vikas0686/portctl/actions/workflows/ci.yml/badge.svg)](https://github.com/vikas0686/portctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vikas0686/portctl)](https://github.com/vikas0686/portctl/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> **Status:** early, hand-built MVP. `ls`, `info`, `kill`, and `why` work on
> macOS and Linux. Windows is not implemented yet. Everything past this
> core (project awareness, docker/k8s labels, history) is intentionally
> not built yet — see [Roadmap](#roadmap).

## The problem

Every developer has some version of this, several times a week:

```
$ npm run dev
Error: listen EADDRINUSE: address already in use :::3000
```

What follows is a small ritual: `lsof -i :3000`, squint at the columns,
copy a PID, `kill -9` it, hope it wasn't something else. And if that
doesn't fix it, there's a second, more confusing question underneath —
is something actually listening? Is this a leftover Docker container? A
zombie from a crashed process? Or is the port stuck in some kernel state
that has nothing to do with a "real" conflict at all?

The tools that are supposed to answer this were built for sysadmins
auditing a server, not developers iterating on a laptop:

| Tool | Good at | Falls short on |
|---|---|---|
| `lsof` | Exhaustive, authoritative | Cryptic flags, no color, not on Windows, slow `-i` scans on macOS |
| `ss` | Fast, modern | Linux-only, terse output, no process-friendly naming |
| `netstat` | Universally present | Deprecated on Linux, inconsistent flags per OS, no kill capability |
| `fuser` | One-shot kill-by-port | No listing, no context, no safety rails |
| `kill-port` (npm) | Zero-config | Needs a Node runtime, no real cross-platform binary, no diagnosis |

None of them are cross-platform, none of them explain *why* something is
happening, and none of them were designed as a product — they're syscalls
with a CLI face.

## The approach

`portctl` treats a **port** as the durable, addressable thing worth asking
about — not the process that happens to be bound to it right now. That
flip matters: processes are ephemeral (PIDs come and go, servers restart),
but developers think in terms of stable ports — "3000 is my frontend,"
"5432 is postgres." Every existing tool gets this backwards, showing a
process table filtered by port instead of an answer about the port itself.

In practice:

- **A verdict, not a table.** `portctl why 8080` explains *why* a port is
  behaving unexpectedly in plain English, instead of raw kernel state you
  have to interpret yourself.
- **Answers even when nothing is listening.** A `TIME_WAIT` socket left
  behind by a process that already exited is still worth explaining —
  most tools just show you nothing.
- **Cross-platform by default**, one static binary, no runtime dependency.
- **No background daemon.** Everything is point-in-time; nothing runs
  when you're not asking it to.

## Commands

| Command | What it does | Example |
|---|---|---|
| `portctl ls` | List everything listening locally. Bare `portctl` is an alias for this. | `portctl ls` |
| `portctl info <port>` | Full detail on one port: owner, command, cwd, optionally CPU/memory. | `portctl info 8080 --cpu --memory` |
| `portctl <port>` | Shorthand for `portctl info <port>` — the port is the thing you're addressing. | `portctl 8080 --cpu` |
| `portctl why <port>` | Plain-English diagnosis of a port's state — *why* it's stuck, not just what's on it. | `portctl why 8080` |
| `portctl kill <port>` | Kill whatever owns a port. Confirms by default. | `portctl kill 8080 -y` |

### Flags

| Flag | Applies to | Effect |
|---|---|---|
| `--cpu` | `info` | Show CPU utilization (average since process start) |
| `--memory`, `--mem` | `info` | Show resident memory (RSS) |
| `-y`, `--yes` | `kill` | Skip the confirmation prompt |
| `--force` | `kill` | Send `SIGKILL` instead of `SIGTERM` |

## Install

**Homebrew** (macOS/Linux):

```sh
brew install vikas0686/portctl/portctl
```

**curl** (macOS/Linux, no Homebrew required):

```sh
curl -fsSL https://raw.githubusercontent.com/vikas0686/portctl/main/install.sh | sh
```

**From source:**

```sh
git clone https://github.com/vikas0686/portctl.git
cd portctl
go build -o portctl ./cmd/portctl
```

Requires Go 1.22+. No third-party dependencies.

> Homebrew and curl installs pull prebuilt binaries from [GitHub
> Releases](https://github.com/vikas0686/portctl/releases) — these don't
> exist until the first tagged release ships.

## Usage

```sh
# list everything listening locally (bare `portctl` is an alias for this)
portctl ls

# everything portctl knows about one port
portctl info 8080
portctl 8080                 # shorthand for the above — the port is the
                              # primary thing you're addressing
portctl 8080 --cpu --memory  # add CPU utilization / RSS to the passport

# plain-English diagnosis of a port's state — not just what's on it, but
# why it's behaving the way it is (e.g. "address already in use" right
# after a restart, with nothing obviously listening)
portctl why 8080

# kill whatever owns a port (confirms by default)
portctl kill 8080
portctl kill 8080 -y        # skip the confirmation prompt
portctl kill 8080 --force   # SIGKILL instead of SIGTERM
```

```
$ portctl ls
PROTO  PORT   PID    PROCESS  STATE
tcp    3000   82013  node     LISTEN
tcp    5432   1204   postgres LISTEN

$ portctl 3000 --cpu --memory
3000/tcp LISTEN
  Owner:   node (pid 82013)
  Command: node server.js --port 3000
  Cwd:     /Users/vikas/proj/web
  CPU:     0.4% (avg since start)
  Memory:  86.2 MB

$ portctl why 3000
3000/tcp TIME_WAIT

  The process that owned this connection has already exited, but the
  kernel is still holding it in TIME_WAIT — normal TCP teardown, not a
  conflict. This is almost always why "address already in use" shows
  up right after restarting a server on the same port.
  It typically clears on its own within ~30s. To rebind
  immediately instead of waiting, have the server set SO_REUSEADDR.
```

## How it works

- **Linux** — reads `/proc/net/{tcp,tcp6,udp,udp6}` and cross-references
  `/proc/[pid]/fd` directly. No shell-outs, no cgo, stdlib only.
- **macOS** — has no `/proc`, and getting socket-to-PID mappings natively
  requires cgo bindings into `libproc`. As a pragmatic first pass, the
  macOS backend shells out to `lsof`/`ps` behind the same `Scanner`
  interface, so it's a drop-in swap for a native implementation later.
- **Windows** — not implemented yet.

`portctl why` needs a bit more than the regular scan on macOS: `lsof` only
walks *live* processes' file descriptors, so a `TIME_WAIT`/`CLOSE_WAIT`
socket left behind by a process that's already exited is invisible to it —
even though the kernel is still enforcing it (e.g. refusing a new `bind()`
on that port). `why` supplements the scan with `netstat`, which reads
kernel socket state directly. Linux doesn't need this: `/proc/net/tcp`
already reads kernel state the same way and surfaces these correctly.

## Roadmap

Rough shape of what's intentionally deferred, in no particular order:

- Native macOS backend (drop the `lsof` shim)
- Windows backend (`iphlpapi.dll` via `golang.org/x/sys/windows`)
- `portctl watch` — live view
- Project awareness (`.portctl.yml`, auto-detected from `docker-compose.yml`
  and framework config)
- Read-only docker / k8s port-forward / SSH tunnel visibility

No background daemon is planned. See the design discussion in this
project's history for the full reasoning behind these calls.

## Contributing

Early days — issues and PRs welcome, but expect the internals to move
around a lot until the core (`ls`/`info`/`kill`/`watch`) settles.

## License

[MIT](LICENSE)
