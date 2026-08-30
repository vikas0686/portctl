// Package process resolves a PID into human-meaningful detail: the command
// that's running, where it was launched from, and its working directory.
// Per-OS backends live in linux.go / darwin.go.
package process

import (
	"errors"
	"os"
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
