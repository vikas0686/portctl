package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

func runKill(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: portctl kill <port> [-y] [--force]")
	}
	port, err := parsePort(args[0])
	if err != nil {
		return err
	}

	assumeYes, force := false, false
	for _, a := range args[1:] {
		switch a {
		case "-y", "--yes":
			assumeYes = true
		case "--force":
			force = true
		}
	}

	scanner := portscan.NewScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	matches := dedupe(portsOn(all, port))
	pids := distinctOwningPIDs(matches)
	if len(pids) == 0 {
		fmt.Printf("nothing on port %d — nothing to kill.\n", port)
		return nil
	}

	sig := syscall.SIGTERM
	sigName := "SIGTERM"
	if force {
		sig = syscall.SIGKILL
		sigName = "SIGKILL"
	}

	for _, pid := range pids {
		name := processNameFor(matches, pid)

		if !assumeYes {
			fmt.Printf("kill %s (pid %d), listening on :%d? [y/N] ", name, pid, port)
			if !confirm() {
				fmt.Println("skipped.")
				continue
			}
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("%s could not find process %d: %v\n", output.Red("✗"), pid, err)
			continue
		}
		if err := proc.Signal(sig); err != nil {
			fmt.Printf("%s failed to signal %s (pid %d): %v\n", output.Red("✗"), name, pid, err)
			continue
		}
		fmt.Printf("%s sent %s to %s (pid %d) — port %d\n", output.Green("✓"), sigName, name, pid, port)
	}
	return nil
}

func distinctOwningPIDs(ports []portscan.Port) []int {
	seen := make(map[int]bool)
	var out []int
	for _, p := range ports {
		if p.PID == 0 || seen[p.PID] {
			continue
		}
		seen[p.PID] = true
		out = append(out, p.PID)
	}
	return out
}

func processNameFor(ports []portscan.Port, pid int) string {
	for _, p := range ports {
		if p.PID == pid && p.ProcessName != "" {
			return p.ProcessName
		}
	}
	if info, err := process.Lookup(pid); err == nil && info.Name != "" {
		return info.Name
	}
	return "unknown"
}

func confirm() bool {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
