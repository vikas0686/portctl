//go:build darwin

package portscan

import (
	"bufio"
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// darwinScanner shells out to lsof's machine-readable output (-F) to build
// the socket table. macOS has no /proc, and reading kern.ipc socket state
// directly requires cgo bindings into libproc; this is a pragmatic bootstrap
// to get something real working behind the Scanner interface, swappable for
// a native libproc implementation later without touching call sites.
type darwinScanner struct{}

func NewScanner() Scanner {
	return darwinScanner{}
}

func (darwinScanner) List() ([]Port, error) {
	cmd := exec.Command("lsof", "-nP", "-iTCP", "-iUDP", "-F", "pcnPT")
	out, err := cmd.Output()
	if err != nil {
		// lsof commonly exits non-zero when it can't read some other
		// user's process fds; whatever it did manage to print on stdout
		// is still valid, so only bail if we got nothing at all.
		if len(out) == 0 {
			return nil, err
		}
	}
	return parseLsofF(out), nil
}

func parseLsofF(out []byte) []Port {
	var ports []Port

	var pid int
	var cmdName string
	var proto Protocol
	var name string
	var state State

	flush := func() {
		if name == "" || proto == "" {
			return
		}
		if proto == UDP {
			state = "" // UDP is connectionless; TST= never applies
		}
		local, remote, ok := splitLsofName(name)
		if !ok {
			return
		}
		lhost, lport := local.host, local.port
		var rhost string
		var rport uint16
		if remote != nil {
			rhost, rport = remote.host, remote.port
		}
		ports = append(ports, Port{
			Protocol:    proto,
			LocalAddr:   lhost,
			LocalPort:   lport,
			RemoteAddr:  rhost,
			RemotePort:  rport,
			State:       state,
			PID:         pid,
			ProcessName: cmdName,
		})
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		tag, val := line[0], line[1:]
		switch tag {
		case 'p':
			flush()
			proto, name, state = "", "", StateUnknown
			pid, _ = strconv.Atoi(val)
			cmdName = ""
		case 'c':
			cmdName = val
		case 'f':
			flush()
			proto, name, state = "", "", StateUnknown
		case 'P':
			proto = Protocol(strings.ToLower(val))
		case 'n':
			name = val
		case 'T':
			if strings.HasPrefix(val, "ST=") {
				state = mapTCPState(val[3:])
			}
		}
	}
	flush()
	return ports
}

func mapTCPState(s string) State {
	switch s {
	case "LISTEN":
		return StateListen
	case "ESTABLISHED":
		return StateEstablished
	case "TIME_WAIT":
		return StateTimeWait
	case "CLOSE_WAIT":
		return StateCloseWait
	default:
		return StateUnknown
	}
}

type endpoint struct {
	host string
	port uint16
}

// splitLsofName parses lsof's NAME field, e.g. "*:4195", "127.0.0.1:3306",
// "[::1]:5432", or "127.0.0.1:54321->127.0.0.1:3000" for an established
// connection.
func splitLsofName(name string) (local endpoint, remote *endpoint, ok bool) {
	parts := strings.SplitN(name, "->", 2)
	local, ok = parseEndpoint(parts[0])
	if !ok {
		return endpoint{}, nil, false
	}
	if len(parts) == 2 {
		if r, ok := parseEndpoint(parts[1]); ok {
			remote = &r
		}
	}
	return local, remote, true
}

func parseEndpoint(s string) (endpoint, bool) {
	if strings.HasPrefix(s, "[") {
		idx := strings.Index(s, "]")
		if idx == -1 {
			return endpoint{}, false
		}
		host := s[1:idx]
		rest := strings.TrimPrefix(s[idx+1:], ":")
		port, _ := strconv.ParseUint(rest, 10, 16)
		return endpoint{host: host, port: uint16(port)}, true
	}
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return endpoint{host: s}, true
	}
	host := s[:idx]
	rest := s[idx+1:]
	if rest == "*" {
		return endpoint{host: host}, true
	}
	port, err := strconv.ParseUint(rest, 10, 16)
	if err != nil {
		return endpoint{host: host}, true
	}
	return endpoint{host: host, port: uint16(port)}, true
}
