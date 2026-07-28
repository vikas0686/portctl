// Package process resolves a PID into human-meaningful detail: the command
// that's running, where it was launched from, and its working directory.
// Per-OS backends live in linux.go / darwin.go.
package process

// Info is what we can find out about a running process.
type Info struct {
	PID     int
	Name    string // short command name, e.g. "node"
	Cmdline string // full invocation, e.g. "node server.js --port 3000"
	Cwd     string // working directory, e.g. "/Users/vikas/proj/web"

	// CPUPercent is average CPU utilization since the process started
	// (total CPU time / wall-clock elapsed time), the same convention
	// classic `ps -o %cpu` uses — not an instantaneous, current-second
	// reading, which would require a second sample a moment apart.
	CPUPercent float64
	MemRSSKb   uint64 // resident set size, in KB
}

// Lookup resolves detail for a PID. Any field it can't determine is left
// empty rather than guessed — callers render empty fields as "unknown".
func Lookup(pid int) (Info, error) {
	return lookup(pid)
}
