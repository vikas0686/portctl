package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

// isProtectedProcess reports whether name is never a legitimate kill
// target: OS/session daemons whose PID-1 parent and "unusual" state are
// their normal, permanent condition, plus container-runtime managers that
// other tooling (Docker et al.) owns the lifecycle of. The actual name
// list lives in internal/process, shared with internal/service's Source
// classification, so it exists in exactly one place.
func isProtectedProcess(name string) bool {
	return process.IsSystemProcess(name)
}

// newScanner is a var (not a direct portscan.NewScanner call) so tests can
// point runClean at a fake Scanner instead of the real system port table.
var newScanner = portscan.NewScanner

type cleanFlags struct {
	dryRun    bool
	assumeYes bool
	json      bool
}

// staleCandidate is a listening port whose owning process shows strong
// evidence of being stale/orphaned dev-server leftovers.
type staleCandidate struct {
	Port    portscan.Port
	Info    process.Info
	Reasons []string
}

func runClean(args []string) error {
	var flags cleanFlags
	for _, a := range args {
		switch a {
		case "--dry-run":
			flags.dryRun = true
		case "-y", "--yes":
			flags.assumeYes = true
		case "--json":
			flags.json = true
		}
	}

	scanner := newScanner()
	all, err := scanner.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}
	rows := dedupe(filterListening(all))

	candidates := staleCandidates(rows)

	if flags.json {
		// --json is report-only, like `why --json`: it hands back facts
		// for a script to act on, and never prompts or kills — an
		// interactive [y/N] confirmation has no sane meaning in a
		// machine-readable, presumably-non-interactive path.
		return output.PrintJSON(cleanCandidatesJSON(candidates))
	}

	if len(candidates) == 0 {
		fmt.Println("No stale development processes found.")
		return nil
	}

	printCandidates(candidates)

	if flags.dryRun {
		return nil
	}

	if !flags.assumeYes {
		fmt.Print("Kill these processes? [y/N] ")
		if !confirm() {
			fmt.Println("aborted.")
			return nil
		}
	}

	for _, c := range candidates {
		name := c.Info.Name
		if name == "" {
			name = c.Port.ProcessName
		}
		signalProcess(c.Port.PID, name, syscall.SIGTERM, "SIGTERM",
			fmt.Sprintf(" — port %d", c.Port.LocalPort))
	}
	return nil
}

// staleCandidates evaluates each distinct owning process of a listening
// port against a small set of strong staleness signals. A process is only
// flagged when at least one *strong* signal fires — a working directory or
// executable that's been deleted out from under a still-running process.
// "Reparented to init/launchd" (PPID 1) is real evidence of an orphaned
// process, but on its own it's also the ordinary, permanent state of any
// intentionally-daemonized dev server (`nohup npm start &` and friends),
// so it's only ever surfaced as a *supporting* reason alongside a strong
// one — never sufficient by itself to flag something for killing.
func staleCandidates(rows []portscan.Port) []staleCandidate {
	var out []staleCandidate
	seen := make(map[int]bool)

	for _, p := range rows {
		if p.PID == 0 || p.PID == os.Getpid() || seen[p.PID] {
			continue
		}
		seen[p.PID] = true

		if isProtectedProcess(p.ProcessName) {
			continue
		}
		// The scan and this evaluation aren't atomic; a process can exit
		// in between. Nothing to clean up if it's already gone.
		if !process.Alive(p.PID) {
			continue
		}

		info, _ := process.Lookup(p.PID)
		if isProtectedProcess(info.Name) {
			continue
		}

		reasons := evaluateStaleness(info)
		if reasons == nil {
			continue
		}

		out = append(out, staleCandidate{Port: p, Info: info, Reasons: reasons})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Port.LocalPort < out[j].Port.LocalPort })
	return out
}

// evaluateStaleness applies the strong/weak signal rule (see
// staleCandidates' doc comment) to a resolved process.Info and returns the
// reasons to report, or nil if nothing strong enough was found to flag it.
// Pure and side-effect-free so the decision rule itself can be unit tested
// without spawning real processes — including the "we couldn't determine
// anything" case (a zero-value Info, e.g. because the process vanished
// between being listed and being looked up), which must safely resolve to
// "not flagged" rather than guessing.
func evaluateStaleness(info process.Info) []string {
	var reasons []string
	strong := false

	if info.Cwd != "" {
		if _, err := os.Stat(info.Cwd); os.IsNotExist(err) {
			reasons = append(reasons, fmt.Sprintf("working directory no longer exists (%s)", info.Cwd))
			strong = true
		}
	}
	if info.ExeDeleted {
		reasons = append(reasons, "executable has been deleted")
		strong = true
	}
	if strong && info.PPID == 1 {
		reasons = append(reasons, "orphaned — reparented to init/launchd (pid 1)")
	}

	if !strong {
		return nil
	}
	return reasons
}

func printCandidates(candidates []staleCandidate) {
	fmt.Println("Potentially stale processes:")
	for _, c := range candidates {
		name := c.Info.Name
		if name == "" {
			name = c.Port.ProcessName
		}
		if name == "" {
			name = output.Dim("unknown")
		}
		cwd := c.Info.Cwd
		if cwd == "" {
			cwd = output.Dim("cwd: unknown")
		}

		fmt.Println()
		fmt.Printf("%s/%s\n", output.Green(fmt.Sprintf("%d", c.Port.LocalPort)), c.Port.Protocol)
		fmt.Printf("  %s (pid %d)\n", name, c.Port.PID)
		fmt.Printf("  %s\n", cwd)
		fmt.Printf("  reason: %s\n", strings.Join(c.Reasons, "; "))
	}
}

// CleanCandidateJSON is the --json shape for one stale-process finding.
type CleanCandidateJSON struct {
	Proto   string   `json:"proto"`
	Port    uint16   `json:"port"`
	PID     int      `json:"pid"`
	Process string   `json:"process,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
	Reasons []string `json:"reasons"`
}

func cleanCandidatesJSON(candidates []staleCandidate) []CleanCandidateJSON {
	out := make([]CleanCandidateJSON, 0, len(candidates))
	for _, c := range candidates {
		name := c.Info.Name
		if name == "" {
			name = c.Port.ProcessName
		}
		out = append(out, CleanCandidateJSON{
			Proto:   string(c.Port.Protocol),
			Port:    c.Port.LocalPort,
			PID:     c.Port.PID,
			Process: name,
			Cwd:     c.Info.Cwd,
			Reasons: c.Reasons,
		})
	}
	return out
}
