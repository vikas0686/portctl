//go:build linux

package portscan

// CheckOrphaned is a no-op on Linux: the native /proc/net/tcp backend
// already surfaces TIME_WAIT/CLOSE_WAIT sockets with no owning process
// (PID left at 0) as part of the regular scan, since it reads kernel
// socket state directly rather than walking process file descriptors.
func CheckOrphaned(port uint16) ([]Port, error) {
	return nil, nil
}

// TimeWaitDuration: Linux hardcodes TIME_WAIT at 60 seconds
// (TCP_TIMEWAIT_LEN in the kernel source), not sysctl-tunable.
func TimeWaitDuration() string {
	return "~60s"
}
