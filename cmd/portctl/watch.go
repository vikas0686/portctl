package main

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vikas0686/portctl/internal/output"
	"github.com/vikas0686/portctl/internal/portscan"
)

// runWatch repaints a live `ls`-style view on an interval, so a developer
// can leave it running in a spare pane and see ports appear/disappear as
// servers start, restart, or crash — instead of re-running `ls` by hand.
func runWatch(args []string) error {
	interval := time.Second
	var filterPort uint16
	hasFilter := false

	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" || a == "--interval":
			if i+1 >= len(args) {
				return fmt.Errorf("usage: portctl watch [port] [-n seconds]")
			}
			i++
			d, err := parseIntervalSeconds(args[i])
			if err != nil {
				return err
			}
			interval = d
		case strings.HasPrefix(a, "--interval="):
			d, err := parseIntervalSeconds(strings.TrimPrefix(a, "--interval="))
			if err != nil {
				return err
			}
			interval = d
		default:
			rest = append(rest, a)
		}
	}

	if len(rest) > 0 {
		p, err := parsePort(rest[0])
		if err != nil {
			return err
		}
		filterPort = p
		hasFilter = true
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	scanner := portscan.NewScanner()
	prev := map[string]portscan.Port{}
	first := true

	tick := time.NewTicker(interval)
	defer tick.Stop()

	for {
		ports, err := scanner.List()
		if err != nil {
			return fmt.Errorf("listing ports: %w", err)
		}
		rows := filterListening(ports)
		if hasFilter {
			rows = portsOn(rows, filterPort)
		}
		rows = dedupe(rows)
		sort.Slice(rows, func(i, j int) bool { return rows[i].LocalPort < rows[j].LocalPort })

		cur := make(map[string]portscan.Port, len(rows))
		for _, p := range rows {
			cur[watchKey(p)] = p
		}

		var added, removed []portscan.Port
		if !first {
			for k, p := range cur {
				if _, ok := prev[k]; !ok {
					added = append(added, p)
				}
			}
			for k, p := range prev {
				if _, ok := cur[k]; !ok {
					removed = append(removed, p)
				}
			}
			sort.Slice(added, func(i, j int) bool { return added[i].LocalPort < added[j].LocalPort })
			sort.Slice(removed, func(i, j int) bool { return removed[i].LocalPort < removed[j].LocalPort })
		}

		renderWatch(rows, added, removed, interval, hasFilter, filterPort)

		prev = cur
		first = false

		select {
		case <-sigCh:
			fmt.Println()
			return nil
		case <-tick.C:
		}
	}
}

func parseIntervalSeconds(s string) (time.Duration, error) {
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil || secs <= 0 {
		return 0, fmt.Errorf("invalid interval %q", s)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// watchKey identifies a socket for diffing between refreshes. PID is part
// of the key so a restarted process on the same port shows as a removal
// followed by an addition, not silence.
func watchKey(p portscan.Port) string {
	return fmt.Sprintf("%s|%s|%d|%d", p.Protocol, p.LocalAddr, p.LocalPort, p.PID)
}

func renderWatch(rows, added, removed []portscan.Port, interval time.Duration, filtered bool, port uint16) {
	if output.IsTTY() {
		fmt.Print("\033[H\033[2J")
	}

	title := "portctl watch"
	if filtered {
		title = fmt.Sprintf("portctl watch %d", port)
	}
	fmt.Printf("%s — every %s — %s — ctrl-c to quit\n\n", title, interval, time.Now().Format("15:04:05"))

	if len(rows) == 0 {
		fmt.Println("nothing listening.")
	} else {
		fmt.Print(portsTable(rows).Render())
	}

	if len(added) > 0 || len(removed) > 0 {
		fmt.Println()
	}
	for _, p := range added {
		fmt.Printf("%s %s\n", output.Green("+"), watchSummary(p))
	}
	for _, p := range removed {
		fmt.Printf("%s %s\n", output.Red("-"), watchSummary(p))
	}
}

func watchSummary(p portscan.Port) string {
	proc := p.ProcessName
	if proc == "" {
		proc = "unknown"
	}
	return fmt.Sprintf("%s/%d %s (pid %d)", p.Protocol, p.LocalPort, proc, p.PID)
}
