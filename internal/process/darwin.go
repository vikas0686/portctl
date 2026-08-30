//go:build darwin

package process

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// lookup shells out to ps/lsof, same bootstrap trade-off as the darwin
// port scanner: no /proc, no cgo yet, real answers today via the tools
// already on every Mac.
func lookup(pid int) (Info, error) {
	info := Info{PID: pid}

	if out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output(); err == nil {
		info.Name = strings.TrimSpace(string(out))
	}

	if out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-ww", "-o", "args=").Output(); err == nil {
		info.Cmdline = strings.TrimSpace(string(out))
	}

	if cwd, ok := lsofCwd(pid); ok {
		info.Cwd = cwd
	}

	if out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=").Output(); err == nil {
		if p, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			info.PPID = p
		}
	}

	// ExeDeleted is intentionally left false (unknown) on macOS: there's
	// no /proc/[pid]/exe equivalent, and lsof's "txt" fd type covers every
	// text segment a process has mapped — the executable *and* every
	// shared library/framework it dynamically links — with no reliable
	// way to tell which "txt" line is the actual binary. Guessing wrong
	// would misclassify a process with an ordinarily-unloaded library as
	// having a deleted executable, which is worse than not checking at
	// all for a signal that feeds a kill decision. See internal/process
	// package docs on Info.ExeDeleted.

	// %cpu and rss are both plain numbers with no internal spaces, safe to
	// whitespace-split — unlike comm/args above, which can contain spaces
	// ("Google Chrome Helper") and so are fetched in their own calls.
	if out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=,rss=").Output(); err == nil {
		fields := strings.Fields(string(out))
		if len(fields) == 2 {
			if cpu, err := strconv.ParseFloat(fields[0], 64); err == nil {
				info.CPUPercent = cpu
			}
			if rss, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				info.MemRSSKb = rss
			}
		}
	}

	return info, nil
}

// lsofCwd asks lsof for a single process's current working directory entry
// (FD "cwd") via the same -F machine-readable format used by the scanner.
func lsofCwd(pid int) (string, bool) {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil || len(out) == 0 {
		return "", false
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "n") {
			return line[1:], true
		}
	}
	return "", false
}
