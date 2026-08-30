<div align="center">

# portctl

**A developer-first CLI for understanding and managing local network ports.**

[![CI](https://github.com/vikas0686/portctl/actions/workflows/ci.yml/badge.svg)](https://github.com/vikas0686/portctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vikas0686/portctl)](https://github.com/vikas0686/portctl/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![portctl demo](docs/demo.gif)

</div>

---
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

## Why portctl?

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

## Quick Start

### See what's listening

```sh
$ portctl ls

PROTO  PORT   PID    PROCESS   STATE
tcp    3000   82013  node      LISTEN
tcp    5432   1204   postgres  LISTEN
```

---

### Inspect a port

Get everything `portctl` knows about a specific port.

```sh
$ portctl 3000
```

```text
3000/tcp LISTEN

Owner:   node (pid 82013)
Command: node server.js --port 3000
Cwd:     ~/projects/web
```

Need more detail?

```sh
$ portctl 3000 --cpu --memory
```

```text
CPU:     0.4%
Memory:  86.2 MB
```

---

### Understand *why* a port behaves the way it does

`why` doesn't just tell you what's happening—it explains it.

```sh
$ portctl why 3000
```

```text
3000/tcp TIME_WAIT

The process that owned this connection has already exited, but the
kernel is still holding the socket in TIME_WAIT.

This commonly happens immediately after restarting a server and is
usually why you see:

  address already in use

The socket will typically be released within ~30 seconds.
```

---

### Free a port

Gracefully stop the process using a port.

```sh
$ portctl kill 3000
```

Skip confirmation:

```sh
$ portctl kill 3000 --yes
```

Force kill if needed:

```sh
$ portctl kill 3000 --force
```

---

### Watch ports live

Leave a live view running in a spare pane — refreshes on an interval and
flags what showed up or disappeared since the last refresh.

```sh
$ portctl watch
```

```text
portctl watch — every 1s — 14:32:07 — ctrl-c to quit

PROTO  PORT   PID    PROCESS   STATE
tcp    3000   82013  node      LISTEN
tcp    5432   1204   postgres  LISTEN

+ tcp/3000 node (pid 82013)
```

Narrow it to one port, and control the refresh rate:

```sh
$ portctl watch 3000 -n 2
```

---

### Clean up stale dev processes

Every long-running dev session accumulates leftovers: a server you
restarted from a directory you've since deleted, a rebuilt binary whose
old process is still bound to the port. `clean` finds processes that show
*strong* evidence of being stale — not just old — and, with confirmation,
kills them the same way `kill` does.

```sh
$ portctl clean
```

```text
Potentially stale processes:

3000/tcp
  node (pid 8123)
  ~/projects/old-app
  reason: working directory no longer exists (~/projects/old-app)

Kill these processes? [y/N]
```

See what would be cleaned without touching anything:

```sh
$ portctl clean --dry-run
```

Skip the confirmation prompt:

```sh
$ portctl clean --yes
```

`clean` only flags a process when its working directory or executable has
actually been deleted out from under it — a merely old or reparented
process is never enough on its own. It also never touches known
system/session daemons or Docker's own manager processes.

---

### Trace where a port really comes from

Sometimes the process on a port isn't the story — it's a build tool, a
process manager, or a wrapper script three layers deep, and killing it
directly just gets it respawned. `tree` walks up from the port to the
process that actually owns the session, so you know what you're really
dealing with before you kill anything.

```sh
$ portctl tree 3000
```

```text
3000/tcp
└── node (pid 8123)
    └── npm (pid 8101)
        └── zsh (pid 8012)
            └── login (pid 501)
```

Run it with no port to see every listening port's ancestry at once:

```sh
$ portctl tree
```

---

### See what you're actually running

`ls` tells you what's bound to a port; it doesn't tell you that pid 8123
is your dev server rather than some random daemon. `services` groups the
same port table by the developer-facing service it recognizes — inferred
from process name, command line, and port, not a hardcoded app database.

```sh
$ portctl services
```

```text
SERVICE     PORT  PROCESS       SOURCE
Vite        3000  node          ~/projects/shop
PostgreSQL  5432  postgres      ~/projects/shop
Redis       6379  redis-server  Docker
```

`services` names the runtime or framework it has real evidence for (Vite,
Next.js, PostgreSQL, Spring Boot, …), not the project — it doesn't try to
guess that pid 8123 is "your frontend"; the working directory does that
job instead. Something it doesn't recognize shows up honestly as `Unknown
service` rather than a wrong specific guess.

Inspect just one port:

```sh
$ portctl services 5432
```

`ls`, `tree`, and `services` answer three different questions:

```text
ls       → What ports exist?
tree     → Where did this process come from?
services → What services are running?
```

---

### Script it

`ls`, `info`, `why`, `clean`, `tree`, and `services` all take `--json` for
piping into `jq` or feeding another tool, instead of scraping the
table/prose output.

```sh
$ portctl ls --json | jq '.[] | select(.port == 3000)'
```

```json
{
  "proto": "tcp",
  "port": 3000,
  "pid": 82013,
  "process": "node",
  "state": "LISTEN"
}
```

`clean --json` is report-only: it never prompts and never kills, even
with `--yes` — pipe the candidates to your own tooling and decide from
there.

```sh
$ portctl tree 3000 --json
```

```json
[
  {
    "proto": "tcp",
    "port": 3000,
    "ancestry": [
      { "pid": 8123, "process": "node" },
      { "pid": 8101, "process": "npm" },
      { "pid": 8012, "process": "zsh" }
    ]
  }
]
```

```sh
$ portctl services --json | jq '.[] | select(.source == "DOCKER")'
```

```json
{
  "service": "Redis",
  "proto": "tcp",
  "port": 6379,
  "process": "docker-proxy",
  "source": "DOCKER",
  "confidence": 70
}
```

`confidence` (0–100) reflects how sure the match is — e.g. a database
recognized directly by its own process name scores higher than a generic
"has `python` in its command line" match. `source` is always the plain
`LOCAL`/`DOCKER`/`SYSTEM`/`UNKNOWN` value here, unlike the text table
where `LOCAL` is rendered as the working directory instead.

## Commands

| Command | What it does | Example |
|---|---|---|
| `portctl ls` | List everything listening locally. Bare `portctl` is an alias for this. | `portctl ls` |
| `portctl info <port>` | Full detail on one port: owner, command, cwd, optionally CPU/memory. | `portctl info 8080 --cpu --memory` |
| `portctl <port>` | Shorthand for `portctl info <port>` — the port is the thing you're addressing. | `portctl 8080 --cpu` |
| `portctl why <port>` | Plain-English diagnosis of a port's state — *why* it's stuck, not just what's on it. | `portctl why 8080` |
| `portctl kill <port>` | Kill whatever owns a port. Confirms by default. | `portctl kill 8080 -y` |
| `portctl watch [port]` | Live-updating `ls`, highlighting ports as they appear/disappear. | `portctl watch 3000 -n 2` |
| `portctl clean` | Find (and, with confirmation, kill) stale/orphaned dev processes occupying ports. | `portctl clean --dry-run` |
| `portctl tree [port]` | Show the process ancestry that owns a port — parent, grandparent, and up. | `portctl tree 3000` |
| `portctl services [port]` | Group ports by the developer-facing service recognized behind them. | `portctl services` |

### Flags

| Flag | Applies to | Effect |
|---|---|---|
| `--cpu` | `info` | Show CPU utilization (average since process start) |
| `--memory`, `--mem` | `info` | Show resident memory (RSS) |
| `--json` | `ls`, `info`, `why`, `clean`, `tree`, `services` | Machine-readable output instead of table/prose |
| `-y`, `--yes` | `kill`, `clean` | Skip the confirmation prompt |
| `--force` | `kill` | Send `SIGKILL` instead of `SIGTERM` |
| `-n`, `--interval <secs>` | `watch` | Refresh interval in seconds (default `1`) |
| `--dry-run` | `clean` | Report what would be cleaned; never kills anything |


## How it works

`portctl` uses the most direct source of truth available on each operating system while exposing the same CLI everywhere.

| Platform | Implementation |
|----------|----------------|
| **Linux** | Reads kernel networking information directly from `/proc`, correlating sockets with processes without invoking external commands. |
| **macOS** | Uses native system tools (`lsof`, `ps`, and `netstat`) behind a common abstraction layer. This avoids cgo today while keeping the backend replaceable with a native implementation in the future. |
| **Windows** | Planned. |

## Contributing

Early days — issues and PRs welcome, but expect the internals to move
around a lot until the core (`ls`/`info`/`kill`/`watch`) settles.

## License

[MIT](LICENSE)
