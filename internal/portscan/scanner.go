// Package portscan enumerates listening/connected sockets and maps them to
// owning processes. Each OS gets its own backend behind the Scanner
// interface; callers never branch on runtime.GOOS.
package portscan

import "fmt"

type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

type State string

const (
	StateListen      State = "LISTEN"
	StateEstablished State = "ESTABLISHED"
	StateTimeWait    State = "TIME_WAIT"
	StateCloseWait   State = "CLOSE_WAIT"
	StateUnknown     State = "UNKNOWN"
)

// Port is one row of the local socket table: an address/port pair, its
// protocol and state, and (when the kernel still knows) the owning process.
// PID is 0 when no owning process could be resolved (e.g. a lingering
// TIME_WAIT socket after the process that held it has already exited).
type Port struct {
	Protocol    Protocol
	LocalAddr   string
	LocalPort   uint16
	RemoteAddr  string
	RemotePort  uint16
	State       State
	PID         int
	ProcessName string
}

func (p Port) String() string {
	return fmt.Sprintf("%s:%d/%s pid=%d(%s) %s", p.LocalAddr, p.LocalPort, p.Protocol, p.PID, p.ProcessName, p.State)
}

// Scanner lists the current local socket table. Implementations are
// per-OS; see linux.go and darwin.go.
type Scanner interface {
	List() ([]Port, error)
}
