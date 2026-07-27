package main

import (
	"fmt"
	"strconv"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

func runInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: portctl info <port>")
	}
	port, err := parsePort(args[0])
	if err != nil {
		return err
	}

	scanner := portscan.NewScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	matches := dedupe(portsOn(all, port))
	if len(matches) == 0 {
		fmt.Printf("nothing on port %d.\n", port)
		return nil
	}

	for i, p := range matches {
		if i > 0 {
			fmt.Println()
		}
		printPassport(p)
	}
	return nil
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

func printPassport(p portscan.Port) {
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
	}
	if p.RemoteAddr != "" {
		fmt.Printf("  Remote:  %s:%d\n", p.RemoteAddr, p.RemotePort)
	}
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return uint16(n), nil
}
