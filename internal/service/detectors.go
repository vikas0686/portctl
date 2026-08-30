package service

import (
	"path/filepath"
	"strings"

	"github.com/vikas0686/portctl/internal/process"
)

// processName lowercases and basenames a Signal's process name so
// detectors can match "postgres" against both "postgres" and (as darwin's
// process backend often reports) an absolute path like
// "/opt/homebrew/opt/postgresql/bin/postgres".
func processName(sig Signal) string {
	return strings.ToLower(filepath.Base(sig.ProcessName))
}

func cmdline(sig Signal) string {
	return strings.ToLower(sig.Cmdline)
}

// --- Docker ----------------------------------------------------------------

// wellKnownServicePorts maps a handful of near-universal dev-database
// default ports to a name. It exists specifically for dockerDetector: when
// a port is published by Docker, the host only ever sees Docker's own
// forwarding process (docker-proxy, or nothing at all with the kernel-NAT
// userland-proxy=false setup) — never the containerized process's real
// name — so process-name evidence is unavailable and the port itself is
// the only signal left. It is deliberately small and only used as this
// fallback, not as a general "port implies service" rule.
var wellKnownServicePorts = map[uint16]string{
	5432:  "PostgreSQL",
	6379:  "Redis",
	27017: "MongoDB",
	3306:  "MySQL",
}

// dockerDetector recognizes Docker's own runtime/manager processes (the
// list is shared with cmd/portctl/clean's kill-safety denylist — see
// internal/process.DockerManagerProcessNames) and reports Source DOCKER.
// It never shells out to the Docker CLI/API: no additional cost beyond the
// process name portctl already resolved, and no hard Docker dependency.
type dockerDetector struct{}

func (dockerDetector) Detect(sig Signal) (Detection, bool) {
	if !process.DockerManagerProcessNames[processName(sig)] {
		return Detection{}, false
	}
	if name, ok := wellKnownServicePorts[sig.Port]; ok {
		return Detection{Name: name, Source: SourceDocker, Confidence: 70}, true
	}
	// We know this port is Docker-forwarded, but not what it's forwarding
	// to — better to say so than to guess.
	return Detection{Name: "Unknown service", Source: SourceDocker, Confidence: 40}, true
}

// --- Databases ---------------------------------------------------------

// databaseProcessNames matches a locally-run (non-Docker) database daemon
// directly by its own process name — the strongest evidence available,
// since these binaries don't go by other names.
var databaseProcessNames = map[string]string{
	"postgres":     "PostgreSQL",
	"postgresql":   "PostgreSQL",
	"redis-server": "Redis",
	"mongod":       "MongoDB",
	"mysqld":       "MySQL",
}

type databaseDetector struct{}

func (databaseDetector) Detect(sig Signal) (Detection, bool) {
	if name, ok := databaseProcessNames[processName(sig)]; ok {
		return Detection{Name: name, Source: SourceLocal, Confidence: 90}, true
	}
	return Detection{}, false
}

// --- Node.js / Vite / Next.js ------------------------------------------

type nodeDetector struct{}

func (nodeDetector) Detect(sig Signal) (Detection, bool) {
	name, cmd := processName(sig), cmdline(sig)
	if !strings.Contains(name, "node") && !strings.HasPrefix(name, "npm") &&
		!strings.Contains(cmd, "node ") && !strings.Contains(cmd, "npm ") && !strings.HasPrefix(cmd, "npm") {
		return Detection{}, false
	}
	switch {
	case strings.Contains(cmd, "vite"):
		return Detection{Name: "Vite", Source: SourceLocal, Confidence: 80}, true
	case strings.Contains(cmd, "next"):
		return Detection{Name: "Next.js", Source: SourceLocal, Confidence: 80}, true
	default:
		return Detection{Name: "Node.js", Source: SourceLocal, Confidence: 50}, true
	}
}

// --- Go ------------------------------------------------------------------

type goDetector struct{}

func (goDetector) Detect(sig Signal) (Detection, bool) {
	cmd := cmdline(sig)
	// A compiled Go binary has no distinguishing name of its own (it's
	// whatever the author called it), so the only reliable signal is
	// running it via the toolchain directly — `go run`/`go build`/`go
	// test`, or the temp binary path `go run` executes from
	// (.../go-build.../exe/<name> on both Linux and macOS).
	if strings.HasPrefix(cmd, "go run") || strings.HasPrefix(cmd, "go build") ||
		strings.HasPrefix(cmd, "go test") || strings.Contains(cmd, "go-build") {
		return Detection{Name: "Go", Source: SourceLocal, Confidence: 60}, true
	}
	return Detection{}, false
}

// --- Java / Spring Boot --------------------------------------------------

type javaDetector struct{}

func (javaDetector) Detect(sig Signal) (Detection, bool) {
	name, cmd := processName(sig), cmdline(sig)
	if name != "java" && !strings.Contains(cmd, "java ") && !strings.HasPrefix(cmd, "java") {
		return Detection{}, false
	}
	if strings.Contains(cmd, "spring") {
		return Detection{Name: "Spring Boot", Source: SourceLocal, Confidence: 80}, true
	}
	return Detection{Name: "Java", Source: SourceLocal, Confidence: 50}, true
}

// --- Python ----------------------------------------------------------------

type pythonDetector struct{}

func (pythonDetector) Detect(sig Signal) (Detection, bool) {
	name, cmd := processName(sig), cmdline(sig)
	if !strings.HasPrefix(name, "python") && !strings.Contains(cmd, "python") {
		return Detection{}, false
	}
	return Detection{Name: "Python", Source: SourceLocal, Confidence: 50}, true
}

// --- System daemons --------------------------------------------------------

// systemDetector recognizes known OS/session daemons (not Docker's own
// processes — dockerDetector already claims those with a more useful
// Source) so they're labeled plainly rather than falling through to
// "Unknown service".
type systemDetector struct{}

func (systemDetector) Detect(sig Signal) (Detection, bool) {
	if !process.SystemProcessNames[processName(sig)] {
		return Detection{}, false
	}
	return Detection{Name: "System process", Source: SourceSystem, Confidence: 90}, true
}
