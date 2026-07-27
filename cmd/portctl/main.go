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
		cmd = args[0]
		rest = args[1:]
	}

	var err error
	switch cmd {
	case "ls":
		err = runLs(rest)
	case "info":
		err = runInfo(rest)
	case "kill":
		err = runKill(rest)
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
  portctl [ls]              list listening ports (default)
  portctl info <port>       everything known about one port
  portctl kill <port>       kill whatever owns a port

Flags:
  -y, --yes                 skip confirmation on kill
  --force                   send SIGKILL instead of SIGTERM
`)
}
