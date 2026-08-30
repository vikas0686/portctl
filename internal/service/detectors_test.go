package service

import "testing"

func TestIdentifyRecognizers(t *testing.T) {
	cases := []struct {
		name       string
		sig        Signal
		wantName   string
		wantSource Source
	}{
		{
			name:       "node",
			sig:        Signal{ProcessName: "node", Cmdline: "node server.js --port 3000"},
			wantName:   "Node.js",
			wantSource: SourceLocal,
		},
		{
			name:       "npm process name only",
			sig:        Signal{ProcessName: "npm", Cmdline: "npm run dev"},
			wantName:   "Node.js",
			wantSource: SourceLocal,
		},
		{
			name:       "vite via npm run dev",
			sig:        Signal{ProcessName: "node", Cmdline: "npm exec vite --port 5173"},
			wantName:   "Vite",
			wantSource: SourceLocal,
		},
		{
			name:       "next.js dev server",
			sig:        Signal{ProcessName: "node", Cmdline: "next dev -p 3000"},
			wantName:   "Next.js",
			wantSource: SourceLocal,
		},
		{
			name:       "python generic",
			sig:        Signal{ProcessName: "python3", Cmdline: "python3 -m http.server 8000"},
			wantName:   "Python",
			wantSource: SourceLocal,
		},
		{
			name:       "python from full path",
			sig:        Signal{ProcessName: "/opt/homebrew/bin/python3.11", Cmdline: ""},
			wantName:   "Python",
			wantSource: SourceLocal,
		},
		{
			name:       "go run",
			sig:        Signal{ProcessName: "exe", Cmdline: "go run main.go"},
			wantName:   "Go",
			wantSource: SourceLocal,
		},
		{
			name:       "go run temp binary path",
			sig:        Signal{ProcessName: "b001", Cmdline: "/tmp/go-build12345/b001/exe/main serve"},
			wantName:   "Go",
			wantSource: SourceLocal,
		},
		{
			name:       "plain compiled go binary is not recognized",
			sig:        Signal{ProcessName: "myapp", Cmdline: "/usr/local/bin/myapp --port 9000"},
			wantName:   "Unknown service",
			wantSource: SourceUnknown,
		},
		{
			name:       "java generic",
			sig:        Signal{ProcessName: "java", Cmdline: "java -jar app.jar"},
			wantName:   "Java",
			wantSource: SourceLocal,
		},
		{
			name:       "spring boot",
			sig:        Signal{ProcessName: "java", Cmdline: "java -jar spring-boot-app-1.0.jar"},
			wantName:   "Spring Boot",
			wantSource: SourceLocal,
		},
		{
			name:       "postgresql",
			sig:        Signal{ProcessName: "postgres", Port: 5432},
			wantName:   "PostgreSQL",
			wantSource: SourceLocal,
		},
		{
			name:       "postgresql from full path",
			sig:        Signal{ProcessName: "/opt/homebrew/opt/postgresql/bin/postgres", Port: 5432},
			wantName:   "PostgreSQL",
			wantSource: SourceLocal,
		},
		{
			name:       "redis",
			sig:        Signal{ProcessName: "redis-server", Port: 6379},
			wantName:   "Redis",
			wantSource: SourceLocal,
		},
		{
			name:       "mongodb",
			sig:        Signal{ProcessName: "mongod", Port: 27017},
			wantName:   "MongoDB",
			wantSource: SourceLocal,
		},
		{
			name:       "unrecognized process",
			sig:        Signal{ProcessName: "some-random-daemon", Cmdline: "some-random-daemon --flag"},
			wantName:   "Unknown service",
			wantSource: SourceUnknown,
		},
		{
			name:       "system process",
			sig:        Signal{ProcessName: "launchd"},
			wantName:   "System process",
			wantSource: SourceSystem,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.sig)
			if got.Name != c.wantName || got.Source != c.wantSource {
				t.Errorf("Identify(%+v) = {%s %s}, want {%s %s}", c.sig, got.Name, got.Source, c.wantName, c.wantSource)
			}
		})
	}
}

func TestIdentifyDockerSource(t *testing.T) {
	cases := []struct {
		name       string
		sig        Signal
		wantName   string
		wantSource Source
	}{
		{
			name:       "docker-proxy on the postgres default port",
			sig:        Signal{ProcessName: "docker-proxy", Port: 5432},
			wantName:   "PostgreSQL",
			wantSource: SourceDocker,
		},
		{
			name:       "docker-proxy on redis's default port",
			sig:        Signal{ProcessName: "docker-proxy", Port: 6379},
			wantName:   "Redis",
			wantSource: SourceDocker,
		},
		{
			name:       "com.docker.backend on an unrecognized port",
			sig:        Signal{ProcessName: "com.docker.backend", Port: 41000},
			wantName:   "Unknown service",
			wantSource: SourceDocker,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.sig)
			if got.Name != c.wantName || got.Source != c.wantSource {
				t.Errorf("Identify(%+v) = {%s %s}, want {%s %s}", c.sig, got.Name, got.Source, c.wantName, c.wantSource)
			}
		})
	}
}

// TestIdentifyAmbiguousProcessPrefersStrongerEvidence covers a Signal two
// Detectors legitimately match: the process is literally named "postgres"
// (databaseDetector, direct name match, confidence 90) but its cmdline
// also happens to mention "python" (pythonDetector, weak substring match,
// confidence 50) — e.g. a Python wrapper script that execs into postgres.
// The name-based match must win.
func TestIdentifyAmbiguousProcessPrefersStrongerEvidence(t *testing.T) {
	sig := Signal{ProcessName: "postgres", Cmdline: "python postgres_supervisor.py", Port: 5432}
	got := Identify(sig)
	if got.Name != "PostgreSQL" || got.Source != SourceLocal {
		t.Errorf("Identify(ambiguous) = {%s %s}, want {PostgreSQL LOCAL}", got.Name, got.Source)
	}
}

// TestIdentifyWithEmptySignal confirms an empty/unresolved Signal (e.g.
// process.Lookup came back with nothing) never crashes and never guesses.
func TestIdentifyWithEmptySignal(t *testing.T) {
	got := Identify(Signal{})
	if got.Name != "Unknown service" || got.Source != SourceUnknown || got.Confidence != 0 {
		t.Errorf("Identify(empty) = %+v, want the zero-confidence unknown fallback", got)
	}
}

// TestIdentifyWithRespectsExplicitDetectorList confirms IdentifyWith runs
// only the detectors it's given, for tests that want to isolate one
// Detector from the full registry.
func TestIdentifyWithRespectsExplicitDetectorList(t *testing.T) {
	sig := Signal{ProcessName: "postgres", Port: 5432}
	got := IdentifyWith(sig, []Detector{nodeDetector{}})
	if got.Name != "Unknown service" {
		t.Errorf("IdentifyWith(postgres signal, [nodeDetector]) = %+v, want the unknown fallback", got)
	}
}
