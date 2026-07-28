//go:build darwin

package portscan

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CheckOrphaned looks for kernel-level socket state on port that the
// regular Scanner can't see. The darwin Scanner is lsof-backed, and lsof
// only walks *live* processes' file descriptor tables — a TIME_WAIT or
// CLOSE_WAIT socket left behind after its owning process has already
// exited isn't attached to any process anymore, so lsof reports nothing
// for it even though the kernel is still enforcing it (e.g. refusing a
// new bind on the same port). netstat reads kernel socket state directly,
// independent of any process, and still sees it.
func CheckOrphaned(port uint16) ([]Port, error) {
	out, err := exec.Command("netstat", "-an", "-p", "tcp").Output()
	if err != nil {
		return nil, err
	}

	var ports []Port
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 || !strings.HasPrefix(fields[0], "tcp") {
			continue
		}
		local, ok := parseNetstatAddr(fields[3])
		if !ok || local.port != port {
			continue
		}
		remote, _ := parseNetstatAddr(fields[4])

		ports = append(ports, Port{
			Protocol:   TCP,
			LocalAddr:  local.host,
			LocalPort:  local.port,
			RemoteAddr: remote.host,
			RemotePort: remote.port,
			State:      mapTCPState(fields[5]),
			// PID intentionally left 0: that's exactly the case this
			// exists for. Whatever process owned this connection may no
			// longer exist.
		})
	}
	return ports, scanner.Err()
}

// TimeWaitDuration estimates how long a TIME_WAIT socket takes to clear on
// this machine. Unlike Linux (a fixed 60s kernel constant), macOS's
// TIME_WAIT length is 2x its MSL sysctl, which is tunable — so it's
// computed from the actual live value instead of assumed.
func TimeWaitDuration() string {
	out, err := exec.Command("sysctl", "-n", "net.inet.tcp.msl").Output()
	if err != nil {
		return "30–60 seconds"
	}
	mslMs, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || mslMs <= 0 {
		return "30–60 seconds"
	}
	return fmt.Sprintf("~%ds", (mslMs*2)/1000)
}

// parseNetstatAddr parses macOS netstat's "host.port" address format,
// e.g. "127.0.0.1.8080", "*.19322", or "fe80::1.62431". The port is
// always after the last '.', since neither IPv4 nor netstat's IPv6
// rendering otherwise use '.'.
func parseNetstatAddr(s string) (endpoint, bool) {
	idx := strings.LastIndex(s, ".")
	if idx == -1 {
		return endpoint{}, false
	}
	host := s[:idx]
	port, err := strconv.ParseUint(s[idx+1:], 10, 16)
	if err != nil {
		return endpoint{host: host}, true // e.g. "*.*", no numeric port
	}
	return endpoint{host: host, port: uint16(port)}, true
}
