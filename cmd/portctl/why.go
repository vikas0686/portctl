package main

import (
	"fmt"
	"strconv"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
)

func runWhy(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: portctl why <port>")
	}
	port, err := parsePort(args[0])
	if err != nil {
		return err
	}
	var asJSON bool
	for _, a := range args[1:] {
		if a == "--json" {
			asJSON = true
		}
	}

	scanner := portscan.NewScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}
	matches := portsOn(all, port)

	// Best-effort: some backends can see kernel socket state the regular
	// scan can't (an orphaned TIME_WAIT/CLOSE_WAIT left behind by a
	// process that already exited). Ignore errors — if this isn't
	// available, why still works off what the regular scan found.
	if orphaned, oerr := portscan.CheckOrphaned(port); oerr == nil {
		matches = mergeOrphaned(matches, orphaned)
	}

	if asJSON {
		return output.PrintJSON(whyResult(port, matches))
	}

	if len(matches) == 0 {
		explainFree(port)
		return nil
	}

	groups := groupByState(matches)
	order := []portscan.State{
		portscan.StateListen,
		portscan.StateEstablished,
		portscan.StateTimeWait,
		portscan.StateCloseWait,
		"", // UDP: no connection state
		portscan.StateUnknown,
	}
	printed := make(map[portscan.State]bool)
	for _, st := range order {
		if g, ok := groups[st]; ok {
			explainGroup(port, st, g)
			printed[st] = true
		}
	}
	for st, g := range groups {
		if !printed[st] {
			explainGroup(port, st, g)
		}
	}
	return nil
}

// WhyResult is the --json shape for `why`: the same facts the prose
// explanation is built from, without the prose.
type WhyResult struct {
	Port    uint16     `json:"port"`
	Free    bool       `json:"free"`
	Matches []WhyMatch `json:"matches,omitempty"`
}

type WhyMatch struct {
	Proto      string `json:"proto"`
	State      string `json:"state,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	RemotePort uint16 `json:"remote_port,omitempty"`
}

func whyResult(port uint16, matches []portscan.Port) WhyResult {
	r := WhyResult{Port: port, Free: len(matches) == 0}
	for _, p := range matches {
		r.Matches = append(r.Matches, WhyMatch{
			Proto:      string(p.Protocol),
			State:      string(p.State),
			PID:        p.PID,
			Process:    p.ProcessName,
			RemoteAddr: p.RemoteAddr,
			RemotePort: p.RemotePort,
		})
	}
	return r
}

func mergeOrphaned(primary, orphaned []portscan.Port) []portscan.Port {
	key := func(p portscan.Port) string {
		return fmt.Sprintf("%s|%s|%d|%s|%d|%s", p.Protocol, p.LocalAddr, p.LocalPort, p.RemoteAddr, p.RemotePort, p.State)
	}
	seen := make(map[string]bool)
	for _, p := range primary {
		seen[key(p)] = true
	}
	out := append([]portscan.Port{}, primary...)
	for _, p := range orphaned {
		if k := key(p); !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

func groupByState(ports []portscan.Port) map[portscan.State][]portscan.Port {
	groups := make(map[portscan.State][]portscan.Port)
	for _, p := range ports {
		groups[p.State] = append(groups[p.State], p)
	}
	return groups
}

func explainFree(port uint16) {
	fmt.Printf("port %d is free — nothing is listening, and no lingering socket state was found.\n", port)
	if port < 1024 {
		fmt.Println(output.Dim("note: ports below 1024 need elevated privileges (root, or CAP_NET_BIND_SERVICE on Linux) to bind."))
	}
}

func explainGroup(port uint16, state portscan.State, group []portscan.Port) {
	label := string(state)
	if label == "" {
		label = "bound"
	}
	header := fmt.Sprintf("%s/%s %s", output.Green(strconv.Itoa(int(port))), group[0].Protocol, output.Dim(label))
	if len(group) > 1 {
		header += output.Dim(fmt.Sprintf(" (%d)", len(group)))
	}
	fmt.Println(header)
	fmt.Println()

	owners := describeOwners(group)

	switch state {
	case portscan.StateListen:
		if owners == "" {
			fmt.Println("  Something is listening here, but its owning process couldn't be determined.")
		} else {
			fmt.Printf("  %s is listening here. This looks normal.\n", owners)
		}

	case portscan.StateEstablished:
		if owners != "" {
			fmt.Printf("  %s has %d active connection(s):\n", owners, len(group))
		} else {
			fmt.Printf("  %d active connection(s), owning process unknown:\n", len(group))
		}
		printRemotes(group)

	case portscan.StateTimeWait:
		if owners == "" {
			fmt.Println("  The process that owned this connection has already exited, but the")
			fmt.Println("  kernel is still holding it in TIME_WAIT — normal TCP teardown, not a")
			fmt.Println("  conflict. This is almost always why \"address already in use\" shows")
			fmt.Println("  up right after restarting a server on the same port.")
		} else {
			fmt.Printf("  %s still has this connection finishing TCP teardown (TIME_WAIT).\n", owners)
		}
		fmt.Printf("  It typically clears on its own within %s. To rebind\n", portscan.TimeWaitDuration())
		fmt.Println("  immediately instead of waiting, have the server set SO_REUSEADDR.")

	case portscan.StateCloseWait:
		if owners == "" {
			fmt.Println("  The remote side closed this connection, and its owning process is")
			fmt.Println("  now gone too, but the kernel hasn't fully released it yet.")
		} else {
			fmt.Printf("  The remote side closed this connection, but %s hasn't closed its\n", owners)
			fmt.Println("  end yet. If this keeps happening, it usually points to a socket or")
			fmt.Println("  connection leak in that process.")
		}

	case "": // UDP has no connection state
		if owners == "" {
			fmt.Println("  Something is bound here, but its owning process couldn't be determined.")
		} else {
			fmt.Printf("  %s is receiving datagrams here. UDP has no connection state, so\n", owners)
			fmt.Println("  this is as much detail as the kernel tracks.")
		}

	default:
		if owners == "" {
			fmt.Println("  A socket is present here, but its state and owner couldn't be fully determined.")
		} else {
			fmt.Printf("  %s owns a socket here in state %s.\n", owners, state)
		}
	}
	fmt.Println()
}

func describeOwners(group []portscan.Port) string {
	seen := make(map[int]bool)
	var parts []string
	for _, p := range group {
		if p.PID == 0 || seen[p.PID] {
			continue
		}
		seen[p.PID] = true
		parts = append(parts, fmt.Sprintf("%s (pid %d)", processNameFor(group, p.PID), p.PID))
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}

func printRemotes(group []portscan.Port) {
	const maxShown = 5
	shown := 0
	for _, p := range group {
		if p.RemoteAddr == "" {
			continue
		}
		if shown >= maxShown {
			fmt.Printf("    ... and %d more\n", len(group)-shown)
			return
		}
		fmt.Printf("    %s:%d\n", p.RemoteAddr, p.RemotePort)
		shown++
	}
}
