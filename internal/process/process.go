// Package process resolves a PID into human-meaningful detail: the command
// that's running, where it was launched from, and its working directory.
// Per-OS backends live in linux.go / darwin.go.
package process

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

// Info is what we can find out about a running process.
type Info struct {
	PID     int
	PPID    int    // parent PID; 0 when it couldn't be determined
	Name    string // short command name, e.g. "node"
	Cmdline string // full invocation, e.g. "node server.js --port 3000"
	Cwd     string // working directory, e.g. "/Users/vikas/proj/web"

	// CPUPercent is average CPU utilization since the process started
	// (total CPU time / wall-clock elapsed time), the same convention
	// classic `ps -o %cpu` uses — not an instantaneous, current-second
	// reading, which would require a second sample a moment apart.
	CPUPercent float64
	MemRSSKb   uint64 // resident set size, in KB

	// ExeDeleted reports whether the on-disk file backing the process's
	// running executable is gone — a still-running binary whose file was
	// deleted or replaced out from under it. Left false when this can't
	// be determined reliably (see darwin.go); callers should treat false
	// as "unknown", not "confirmed present".
	ExeDeleted bool
}

// Lookup resolves detail for a PID. Any field it can't determine is left
// empty rather than guessed — callers render empty fields as "unknown".
func Lookup(pid int) (Info, error) {
	return lookup(pid)
}

// Alive reports whether pid currently refers to a running process. It uses
// the standard Unix idiom of signaling 0: os.FindProcess always succeeds on
// Unix (there's no lookup step), and Signal(0) probes for existence without
// actually delivering anything. Signaling a process owned by another user
// (root-owned daemons, most commonly) fails with EPERM even though it very
// much exists — only ESRCH means the PID doesn't refer to a running
// process, so that's the only error treated as "not alive"; anything else
// (including EPERM) means it exists but we can't touch it.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return !errors.Is(err, syscall.ESRCH)
}

// SystemProcessNames are OS/session daemons — never a legitimate target
// for portctl to kill, and not what "clean" or "services" mean by a
// developer-facing service even though they're plainly always running.
// Shared by cmd/portctl's clean (as a kill-safety denylist) and
// internal/service (to classify a port's Source as SYSTEM).
var SystemProcessNames = map[string]bool{
	"launchd": true, "systemd": true, "init": true,
	"kernel_task": true, "kthreadd": true,
	"windowserver": true, "loginwindow": true, "finder": true,
	"sshd": true, "syslogd": true, "rsyslogd": true,
	"cron": true, "crond": true, "dbus-daemon": true,
	"networkmanager": true, "systemd-resolved": true, "systemd-journald": true,
	"coreaudiod": true, "bluetoothd": true, "logd": true, "notifyd": true,
	"cfprefsd": true, "powerd": true, "diskarbitrationd": true,
	"mds": true, "mds_stores": true, "mdworker": true, "distnoted": true,
}

// DockerManagerProcessNames are Docker's own runtime/manager processes —
// distinct from a containerized workload, which portctl never observes
// directly (see internal/service's Docker detector for how a container's
// service is inferred despite that). Also never a legitimate kill target.
var DockerManagerProcessNames = map[string]bool{
	"com.docker.backend": true, "com.docker.hyperkit": true, "com.docker.vmnetd": true,
	"dockerd": true, "containerd": true, "containerd-shim": true, "docker-proxy": true,
}

// IsSystemProcess reports whether name is a known OS/session daemon or a
// Docker runtime-manager process — the union clean.go uses as a kill-safety
// denylist. Callers that need to tell the two apart (internal/service, to
// pick SYSTEM vs. DOCKER as a Source) should consult the two maps directly.
func IsSystemProcess(name string) bool {
	n := strings.ToLower(name)
	return SystemProcessNames[n] || DockerManagerProcessNames[n]
}
