package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
	"github.com/vikas0686/portctl/internal/service"
)

func runServices(args []string) error {
	var asJSON bool
	var portArg string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
			continue
		}
		if portArg == "" {
			portArg = a
		}
	}

	scanner := newScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}
	rows := dedupe(filterListening(all))

	if portArg != "" {
		port, err := parsePort(portArg)
		if err != nil {
			return err
		}
		rows = portsOn(rows, port)
		if len(rows) == 0 {
			if asJSON {
				return output.PrintJSON([]ServiceEntryJSON{})
			}
			fmt.Printf("nothing on port %d.\n", port)
			return nil
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LocalPort < rows[j].LocalPort })

	entries := make([]serviceEntry, 0, len(rows))
	for _, p := range rows {
		entries = append(entries, identifyService(p))
	}

	if asJSON {
		return output.PrintJSON(serviceEntriesJSON(entries))
	}

	if len(entries) == 0 {
		fmt.Println("nothing listening.")
		return nil
	}

	printServices(entries)
	return nil
}

// serviceEntry pairs one listening port with what service.Identify made of
// it, plus the display-friendly bits (resolved process name, cwd) that
// come along for the ride from the same process.Lookup call.
type serviceEntry struct {
	Port      portscan.Port
	Process   string
	Cwd       string
	Detection service.Detection
}

// identifyService assembles a service.Signal from data ls/info/tree
// already know how to collect — one Lookup per distinct PID, nothing
// service-specific spawned — and hands it to service.Identify.
func identifyService(p portscan.Port) serviceEntry {
	sig := service.Signal{Proto: string(p.Protocol), Port: p.LocalPort, PID: p.PID, ProcessName: p.ProcessName}
	entry := serviceEntry{Port: p, Process: p.ProcessName}

	if p.PID != 0 {
		if info, err := process.Lookup(p.PID); err == nil {
			if info.Name != "" {
				sig.ProcessName = info.Name
				entry.Process = info.Name
			}
			sig.Cmdline = info.Cmdline
			entry.Cwd = info.Cwd
		}
	}

	entry.Process = filepath.Base(entry.Process)
	entry.Detection = service.Identify(sig)
	return entry
}

func printServices(entries []serviceEntry) {
	t := output.Table{Headers: []string{"SERVICE", "PORT", "PROCESS", "SOURCE"}}
	for _, e := range entries {
		proc := e.Process
		if proc == "" {
			proc = output.Dim("unknown")
		}
		t.Rows = append(t.Rows, []string{
			e.Detection.Name,
			strconv.Itoa(int(e.Port.LocalPort)),
			proc,
			sourceDisplay(e.Detection.Source, e.Cwd),
		})
	}
	fmt.Print(t.Render())
}

// sourceDisplay renders the SOURCE column: a local service's working
// directory is more useful at a glance than the literal word "LOCAL", so
// it's shown when known; every other source (or a local service with no
// resolvable cwd) falls back to a plain label.
func sourceDisplay(src service.Source, cwd string) string {
	if src == service.SourceLocal && cwd != "" {
		return shortenHome(cwd)
	}
	switch src {
	case service.SourceDocker:
		return "Docker"
	case service.SourceSystem:
		return "System"
	case service.SourceLocal:
		return "Local"
	default:
		return output.Dim("Unknown")
	}
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

// ServiceEntryJSON is the --json shape for one detected service. Unlike
// the text table, source and cwd are kept as separate fields rather than
// folded together — a stable structure for scripts shouldn't require the
// consumer to know the text renderer's display convention.
type ServiceEntryJSON struct {
	Service    string `json:"service"`
	Proto      string `json:"proto"`
	Port       uint16 `json:"port"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
	Source     string `json:"source"`
	Cwd        string `json:"cwd,omitempty"`
	Confidence int    `json:"confidence"`
}

func serviceEntriesJSON(entries []serviceEntry) []ServiceEntryJSON {
	out := make([]ServiceEntryJSON, 0, len(entries))
	for _, e := range entries {
		out = append(out, ServiceEntryJSON{
			Service:    e.Detection.Name,
			Proto:      string(e.Port.Protocol),
			Port:       e.Port.LocalPort,
			PID:        e.Port.PID,
			Process:    e.Process,
			Source:     string(e.Detection.Source),
			Cwd:        e.Cwd,
			Confidence: e.Detection.Confidence,
		})
	}
	return out
}
