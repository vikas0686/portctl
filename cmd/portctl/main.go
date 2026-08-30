package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]

	// Bare `portctl` == `portctl ls`: a static, pipeable, scriptable
	// listing by default. Interactivity is opt-in later (portctl watch),
	// never the default surface.
	cmd := "ls"
	rest := args
	if len(args) > 0 && !isFlag(args[0]) {
		switch args[0] {
		case "ls", "info", "kill", "why", "watch", "clean", "tree", "services", "help":
			cmd = args[0]
			rest = args[1:]
		default:
			// The port is portctl's primary entity, so a bare number is
			// shorthand for `info`: `portctl 8080 --cpu` == `portctl info
			// 8080 --cpu`.
			if _, err := parsePort(args[0]); err == nil {
				cmd = "info"
				rest = args
			} else {
				cmd = args[0]
				rest = args[1:]
			}
		}
	}

	var err error
	switch cmd {
	case "ls":
		err = runLs(rest)
	case "info":
		err = runInfo(rest)
	case "kill":
		err = runKill(rest)
	case "why":
		err = runWhy(rest)
	case "watch":
		err = runWatch(rest)
	case "clean":
		err = runClean(rest)
	case "tree":
		err = runTree(rest)
	case "services":
		err = runServices(rest)
	case "help", "--help", "-h":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "portctl: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "portctl: %v\n", err)
		os.Exit(1)
	}
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}

func printUsage() {
	fmt.Print(`portctl — see and control what's listening on your local ports

Usage:
  portctl [ls]               list listening ports (default)
  portctl info <port>        everything known about one port
  portctl <port>             shorthand for "portctl info <port>"
  portctl kill <port>        kill whatever owns a port
  portctl why <port>         plain-English diagnosis of a port's state
  portctl watch [port]       live-updating view of listening ports
  portctl clean              find and optionally kill stale dev processes
  portctl tree [port]        show a port's owning process ancestry
  portctl services [port]    show developer-facing services, not raw processes

Flags:
  --cpu, --memory            (info) show CPU / memory utilization
  --json                     (ls, info, why, clean, tree, services) machine-readable output
  -y, --yes                  (kill, clean) skip confirmation
  --force                    (kill) send SIGKILL instead of SIGTERM
  -n, --interval <secs>      (watch) refresh interval, default 1s
  --dry-run                  (clean) report only, never kill
`)
}
