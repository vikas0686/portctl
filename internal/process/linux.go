//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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

	return info, nil
}
