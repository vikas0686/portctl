//go:build linux

package portscan

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxScanner reads /proc/net/{tcp,tcp6,udp,udp6} directly and cross
// references /proc/[pid]/fd to map socket inodes to owning processes. No
// shell-outs, no cgo, no external tools.
type linuxScanner struct{}

func NewScanner() Scanner {
	return linuxScanner{}
}

var tcpStates = map[string]State{
	"01": StateEstablished,
	"06": StateTimeWait,
	"08": StateCloseWait,
	"0A": StateListen,
}

func (linuxScanner) List() ([]Port, error) {
	inodeToPID, err := buildInodeIndex()
	if err != nil {
		return nil, fmt.Errorf("indexing /proc fds: %w", err)
	}

	var ports []Port
	sources := []struct {
		path     string
		proto    Protocol
		hasState bool
	}{
		{"/proc/net/tcp", TCP, true},
		{"/proc/net/tcp6", TCP, true},
		{"/proc/net/udp", UDP, false},
		{"/proc/net/udp6", UDP, false},
	}

	for _, src := range sources {
		rows, err := parseProcNet(src.path, src.proto, src.hasState)
		if err != nil {
			if os.IsNotExist(err) {
				continue // e.g. no IPv6 stack
			}
			return nil, err
		}
		for i := range rows {
			if own, ok := inodeToPID[rows[i].inode]; ok {
				rows[i].Port.PID = own.pid
				rows[i].Port.ProcessName = own.name
			}
			ports = append(ports, rows[i].Port)
		}
	}
	return ports, nil
}

type procNetRow struct {
	Port
	inode string
}

func parseProcNet(path string, proto Protocol, hasState bool) ([]procNetRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rows []procNetRow
	scanner := bufio.NewScanner(f)
	scanner.Scan() // header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		localAddr, localPort, err := parseHexAddr(fields[1])
		if err != nil {
			continue
		}
		remoteAddr, remotePort, err := parseHexAddr(fields[2])
		if err != nil {
			continue
		}

		state := StateUnknown
		if hasState {
			if s, ok := tcpStates[fields[3]]; ok {
				state = s
			}
		} else {
			state = "" // UDP has no connection state worth surfacing
		}

		rows = append(rows, procNetRow{
			Port: Port{
				Protocol:   proto,
				LocalAddr:  localAddr,
				LocalPort:  localPort,
				RemoteAddr: remoteAddr,
				RemotePort: remotePort,
				State:      state,
			},
			inode: fields[9],
		})
	}
	return rows, scanner.Err()
}

// parseHexAddr decodes the "IP:PORT" hex encoding used by /proc/net/{tcp,udp}*,
// e.g. "0100007F:0050" -> 127.0.0.1:80. Addresses are stored as 32-bit
// little-endian words (one word for IPv4, four for IPv6).
func parseHexAddr(field string) (string, uint16, error) {
	parts := strings.SplitN(field, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("malformed address %q", field)
	}
	addrHex, portHex := parts[0], parts[1]

	port, err := strconv.ParseUint(portHex, 16, 32)
	if err != nil {
		return "", 0, err
	}

	raw, err := hex.DecodeString(addrHex)
	if err != nil {
		return "", 0, err
	}

	ip := make(net.IP, len(raw))
	for word := 0; word*4 < len(raw); word++ {
		copy(ip[word*4:word*4+4], reverseBytes(raw[word*4:word*4+4]))
	}
	return ip.String(), uint16(port), nil
}

func reverseBytes(b []byte) []byte {
	out := make([]byte, len(b))
	binary.BigEndian.PutUint32(out, binary.LittleEndian.Uint32(b))
	return out
}

type owner struct {
	pid  int
	name string
}

// buildInodeIndex walks /proc/[pid]/fd, resolving symlinks of the form
// "socket:[N]" to build an inode->owning-process map. Processes we can't
// read (permissions, already exited) are silently skipped.
func buildInodeIndex() (map[string]owner, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	index := make(map[string]owner)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a pid directory
		}

		fdDir := filepath.Join("/proc", e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // permission denied or process gone
		}

		var name string
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if name == "" {
				name = processName(pid)
			}
			index[inode] = owner{pid: pid, name: name}
		}
	}
	return index, nil
}

func processName(pid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
