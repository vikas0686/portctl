//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// clockTicksPerSec is USER_HZ, the unit /proc/[pid]/stat's time fields are
// counted in. It's compile-time fixed at 100 on every mainstream Linux
// architecture, and not worth a cgo/sysconf round trip to confirm.
const clockTicksPerSec = 100

func lookup(pid int) (Info, error) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	info := Info{PID: pid}

	if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
		info.Name = strings.TrimSpace(string(b))
	}

	if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		args := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
		info.Cmdline = strings.Join(args, " ")
	}

	if link, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
		info.Cwd = link
	}

	info.CPUPercent = cpuPercent(dir)
	info.MemRSSKb = memRSSKb(dir)

	return info, nil
}

// cpuPercent computes average utilization since the process started:
// total CPU time consumed, divided by how long the process has existed.
func cpuPercent(dir string) float64 {
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return 0
	}
	// comm (field 2) is parenthesized and may itself contain spaces or
	// parens, so split on the *last* ')' rather than plain whitespace.
	line := string(stat)
	idx := strings.LastIndex(line, ")")
	if idx == -1 {
		return 0
	}
	fields := strings.Fields(line[idx+1:])
	// fields[0] is field 3 (state); utime=14, stime=15, starttime=22.
	if len(fields) < 20 {
		return 0
	}
	utime, err1 := strconv.ParseFloat(fields[14-3], 64)
	stime, err2 := strconv.ParseFloat(fields[15-3], 64)
	starttime, err3 := strconv.ParseFloat(fields[22-3], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}

	uptime, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	uptimeFields := strings.Fields(string(uptime))
	if len(uptimeFields) < 1 {
		return 0
	}
	systemUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0
	}

	cpuTimeSec := (utime + stime) / clockTicksPerSec
	elapsedSec := systemUptime - starttime/clockTicksPerSec
	if elapsedSec <= 0 {
		return 0
	}
	return cpuTimeSec / elapsedSec * 100
}

func memRSSKb(dir string) uint64 {
	b, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb
	}
	return 0
}
