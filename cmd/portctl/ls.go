package main

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
)

func runLs(args []string) error {
	var asJSON bool
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}

	scanner := portscan.NewScanner()
	ports, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	rows := filterListening(ports)
	sort.Slice(rows, func(i, j int) bool { return rows[i].LocalPort < rows[j].LocalPort })
	rows = dedupe(rows)

	if asJSON {
		return output.PrintJSON(portEntries(rows))
	}

	if len(rows) == 0 {
		fmt.Println("nothing listening.")
		return nil
	}

	fmt.Print(portsTable(rows).Render())
	return nil
}

// PortEntry is the --json shape for a listening/bound port, shared by `ls`
// and the base fields of `info`.
type PortEntry struct {
	Proto   string `json:"proto"`
	Port    uint16 `json:"port"`
	PID     int    `json:"pid,omitempty"`
	Process string `json:"process,omitempty"`
	State   string `json:"state,omitempty"`
}

func portEntries(rows []portscan.Port) []PortEntry {
	entries := make([]PortEntry, 0, len(rows))
	for _, p := range rows {
		entries = append(entries, PortEntry{
			Proto:   string(p.Protocol),
			Port:    p.LocalPort,
			PID:     p.PID,
			Process: p.ProcessName,
			State:   string(p.State),
		})
	}
	return entries
}

// portsTable renders the standard PROTO/PORT/PID/PROCESS/STATE table shared
// by `ls` and `watch`.
func portsTable(rows []portscan.Port) output.Table {
	t := output.Table{Headers: []string{"PROTO", "PORT", "PID", "PROCESS", "STATE"}}
	for _, p := range rows {
		pid, proc := "-", output.Dim("unknown")
		if p.PID != 0 {
			pid = strconv.Itoa(p.PID)
			proc = p.ProcessName
			if proc == "" {
				proc = output.Dim("unknown")
			}
		}
		state := string(p.State)
		if p.State == portscan.StateListen {
			state = output.Green(state)
		} else if state == "" {
			state = output.Dim("-")
		}
		t.Rows = append(t.Rows, []string{
			string(p.Protocol),
			strconv.Itoa(int(p.LocalPort)),
			pid,
			proc,
			state,
		})
	}
	return t
}

// filterListening keeps TCP sockets actually in LISTEN state, and UDP
// sockets (which have no connection state, but binding one *is* listening).
func filterListening(ports []portscan.Port) []portscan.Port {
	var out []portscan.Port
	for _, p := range ports {
		if p.LocalPort == 0 {
			continue // unbound wildcard socket, not actually using a port
		}
		if p.Protocol == portscan.UDP || p.State == portscan.StateListen {
			out = append(out, p)
		}
	}
	return out
}

// dedupe collapses duplicate rows that arise from a process holding more
// than one file descriptor on the same socket.
func dedupe(ports []portscan.Port) []portscan.Port {
	seen := make(map[string]bool)
	var out []portscan.Port
	for _, p := range ports {
		key := fmt.Sprintf("%s|%s|%d|%d", p.Protocol, p.LocalAddr, p.LocalPort, p.PID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}
