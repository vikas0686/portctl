package main

import (
	"fmt"
	"strconv"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

type infoFlags struct {
	cpu    bool
	memory bool
	json   bool
}

func runInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: portctl info <port> [--cpu] [--memory] [--json]")
	}
	port, err := parsePort(args[0])
	if err != nil {
		return err
	}

	var flags infoFlags
	for _, a := range args[1:] {
		switch a {
		case "--cpu":
			flags.cpu = true
		case "--memory", "--mem":
			flags.memory = true
		case "--json":
			flags.json = true
		}
	}

	scanner := portscan.NewScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	matches := dedupe(portsOn(all, port))

	if flags.json {
		return output.PrintJSON(portDetails(matches))
	}

	if len(matches) == 0 {
		fmt.Printf("nothing on port %d.\n", port)
		return nil
	}

	for i, p := range matches {
		if i > 0 {
			fmt.Println()
		}
		printPassport(p, flags)
	}
	return nil
}

// PortDetail is the --json shape for `info`: everything printPassport
// would otherwise render as prose.
type PortDetail struct {
	Proto      string   `json:"proto"`
	Port       uint16   `json:"port"`
	State      string   `json:"state,omitempty"`
	PID        int      `json:"pid,omitempty"`
	Process    string   `json:"process,omitempty"`
	Cmdline    string   `json:"cmdline,omitempty"`
	Cwd        string   `json:"cwd,omitempty"`
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	MemRSSKb   *uint64  `json:"mem_rss_kb,omitempty"`
	RemoteAddr string   `json:"remote_addr,omitempty"`
	RemotePort uint16   `json:"remote_port,omitempty"`
}

func portDetails(matches []portscan.Port) []PortDetail {
	out := make([]PortDetail, 0, len(matches))
	for _, p := range matches {
		d := PortDetail{
			Proto:      string(p.Protocol),
			Port:       p.LocalPort,
			State:      string(p.State),
			PID:        p.PID,
			Process:    p.ProcessName,
			RemoteAddr: p.RemoteAddr,
			RemotePort: p.RemotePort,
		}
		if p.PID != 0 {
			if info, err := process.Lookup(p.PID); err == nil {
				if info.Name != "" {
					d.Process = info.Name
				}
				d.Cmdline = info.Cmdline
				d.Cwd = info.Cwd
				d.CPUPercent = &info.CPUPercent
				d.MemRSSKb = &info.MemRSSKb
			}
		}
		out = append(out, d)
	}
	return out
}

func portsOn(all []portscan.Port, port uint16) []portscan.Port {
	var out []portscan.Port
	for _, p := range all {
		if p.LocalPort == port {
			out = append(out, p)
		}
	}
	return out
}

func printPassport(p portscan.Port, flags infoFlags) {
	state := string(p.State)
	if state == "" {
		state = "-"
	}
	fmt.Printf("%s/%s %s\n", output.Green(strconv.Itoa(int(p.LocalPort))), p.Protocol, output.Dim(state))

	if p.PID == 0 {
		fmt.Printf("  Owner:   %s\n", output.Dim("unknown (not observed)"))
		return
	}

	info, err := process.Lookup(p.PID)
	name := p.ProcessName
	if err == nil && info.Name != "" {
		name = info.Name
	}
	fmt.Printf("  Owner:   %s (pid %d)\n", name, p.PID)
	if err == nil {
		if info.Cmdline != "" {
			fmt.Printf("  Command: %s\n", info.Cmdline)
		}
		if info.Cwd != "" {
			fmt.Printf("  Cwd:     %s\n", info.Cwd)
		}
		if flags.cpu {
			fmt.Printf("  CPU:     %.1f%% %s\n", info.CPUPercent, output.Dim("(avg since start)"))
		}
		if flags.memory {
			fmt.Printf("  Memory:  %s\n", formatKb(info.MemRSSKb))
		}
	}
	if p.RemoteAddr != "" {
		fmt.Printf("  Remote:  %s:%d\n", p.RemoteAddr, p.RemotePort)
	}
}

func formatKb(kb uint64) string {
	if kb == 0 {
		return output.Dim("unknown")
	}
	if kb < 1024 {
		return fmt.Sprintf("%d KB", kb)
	}
	mb := float64(kb) / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	return fmt.Sprintf("%.2f GB", mb/1024)
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return uint16(n), nil
}
