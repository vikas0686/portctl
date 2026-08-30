package main

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

// maxAncestryDepth caps how far buildAncestry walks up the parent-PID
// chain — a defensive bound alongside cycle detection, in case of
// pathological or corrupted process state.
const maxAncestryDepth = 32

// ancestryNode is one process in a port's ownership chain. Name is set on
// a normally-resolved process; Note is set instead when the walk had to
// stop for a reason worth telling the user about (rather than silently
// truncating or guessing).
type ancestryNode struct {
	PID  int
	Name string
	Note string // "process exited" | "cycle detected" | "unavailable" | "ancestry too deep, stopped"
}

func runTree(args []string) error {
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
				return output.PrintJSON([]TreeResultJSON{})
			}
			fmt.Printf("nothing on port %d.\n", port)
			return nil
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LocalPort < rows[j].LocalPort })

	var results []treeResult
	for _, p := range rows {
		if p.PID == 0 {
			continue // an orphaned socket has no owning process to trace
		}
		results = append(results, treeResult{Port: p, Ancestry: buildAncestry(p.PID, p.ProcessName)})
	}

	if asJSON {
		return output.PrintJSON(treeResultsJSON(results))
	}

	if len(results) == 0 {
		fmt.Println("nothing listening.")
		return nil
	}

	for i, r := range results {
		if i > 0 {
			fmt.Println()
		}
		printAncestryTree(r)
	}
	return nil
}

type treeResult struct {
	Port     portscan.Port
	Ancestry []ancestryNode
}

// buildAncestry walks the parent-PID chain for pid, using the real
// process package — see buildAncestryWith for the (independently
// testable) walking logic itself. knownName seeds the leaf's displayed
// name from the port scan, which already resolved it once, in case a
// fresh Lookup a moment later can't (e.g. the process is exiting).
func buildAncestry(pid int, knownName string) []ancestryNode {
	return buildAncestryWith(pid, knownName, process.Lookup, process.Alive)
}

// buildAncestryWith is the actual walking algorithm, parameterized over
// how to resolve and probe a PID so the decision logic (cycle detection,
// depth guard, disappearing processes, unresolvable parents) can be unit
// tested without spawning real OS processes for every scenario.
func buildAncestryWith(pid int, knownName string, lookup func(int) (process.Info, error), alive func(int) bool) []ancestryNode {
	var chain []ancestryNode
	visited := make(map[int]bool)

	cur := pid
	fallback := knownName
	for depth := 0; depth < maxAncestryDepth; depth++ {
		if visited[cur] {
			chain = append(chain, ancestryNode{PID: cur, Note: "cycle detected"})
			return chain
		}
		visited[cur] = true

		// The port scan and this walk aren't atomic, and neither is
		// walking from one ancestor to the next — a process can exit at
		// any step. Alive is checked before trusting whatever Lookup
		// returns for it.
		if !alive(cur) {
			chain = append(chain, ancestryNode{PID: cur, Note: "process exited"})
			return chain
		}

		info, _ := lookup(cur)
		name := info.Name
		if name == "" {
			name = fallback
		}
		if name == "" {
			// Lookup came back empty: permission denied, or a transient
			// gap we can't otherwise explain. Either way, don't guess —
			// say so and stop, since we also have no trustworthy PPID to
			// continue from.
			chain = append(chain, ancestryNode{PID: cur, Note: "unavailable"})
			return chain
		}
		chain = append(chain, ancestryNode{PID: cur, Name: name})

		if info.PPID == 0 {
			return chain // reached the top of what we can determine
		}
		cur = info.PPID
		fallback = ""
	}

	chain = append(chain, ancestryNode{PID: cur, Note: "ancestry too deep, stopped"})
	return chain
}

func printAncestryTree(r treeResult) {
	fmt.Printf("%s/%s\n", output.Green(strconv.Itoa(int(r.Port.LocalPort))), r.Port.Protocol)
	indent := ""
	for _, node := range r.Ancestry {
		label := node.Name
		if node.Note != "" {
			label = output.Dim(node.Note)
		}
		fmt.Printf("%s└── %s (pid %d)\n", indent, label, node.PID)
		indent += "    "
	}
}

// TreeNodeJSON is the --json shape for one process in an ancestry chain.
type TreeNodeJSON struct {
	PID     int    `json:"pid"`
	Process string `json:"process,omitempty"`
	Note    string `json:"note,omitempty"`
}

// TreeResultJSON is the --json shape for one port's ancestry. `tree` always
// returns an array, even for `tree <port>` — matching `info --json`'s
// convention, since a port can have more than one owning PID.
type TreeResultJSON struct {
	Proto    string         `json:"proto"`
	Port     uint16         `json:"port"`
	Ancestry []TreeNodeJSON `json:"ancestry"`
}

func treeResultsJSON(results []treeResult) []TreeResultJSON {
	out := make([]TreeResultJSON, 0, len(results))
	for _, r := range results {
		nodes := make([]TreeNodeJSON, 0, len(r.Ancestry))
		for _, n := range r.Ancestry {
			nodes = append(nodes, TreeNodeJSON{PID: n.PID, Process: n.Name, Note: n.Note})
		}
		out = append(out, TreeResultJSON{
			Proto:    string(r.Port.Protocol),
			Port:     r.Port.LocalPort,
			Ancestry: nodes,
		})
	}
	return out
}
