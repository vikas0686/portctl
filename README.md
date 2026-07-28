# portctl

See and control what's listening on your local ports — without memorizing
`lsof` flags.

`portctl` treats a **port** as the durable thing worth asking about, not the
process that happens to be bound to it right now. Point-in-time, no
background service, no config required to get useful output.

> **Status:** early, hand-built MVP. `ls`, `info`, `kill`, and `why` work on
> macOS and Linux. Windows is not implemented yet. Everything past this
> core (project awareness, docker/k8s labels, history) is intentionally
> not built yet — see [Roadmap](#roadmap).

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
