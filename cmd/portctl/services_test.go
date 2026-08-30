package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/vikas0686/portctl/internal/portscan"
)

func TestRunServicesNothingListening(t *testing.T) {
	withFakeScanner(t, nil)
	out, err := captureStdout(t, func() error { return runServices(nil) })
	if err != nil {
		t.Fatalf("runServices: %v", err)
	}
	if strings.TrimSpace(out) != "nothing listening." {
		t.Errorf("runServices() with no rows = %q, want %q", out, "nothing listening.")
	}
}

func TestRunServicesSpecificPortNotFound(t *testing.T) {
	port := freeTCPPort(t)
	out, err := captureStdout(t, func() error { return runServices([]string{strconv.Itoa(int(port))}) })
	if err != nil {
		t.Fatalf("runServices: %v", err)
	}
	want := "nothing on port " + strconv.Itoa(int(port)) + ".\n"
	if out != want {
		t.Errorf("runServices(free port) = %q, want %q", out, want)
	}
}

// TestRunServicesMultipleServicesText exercises several recognized
// services at once through the real CLI path, over a fake port table so
// results don't depend on what happens to be running on the dev machine.
func TestRunServicesMultipleServicesText(t *testing.T) {
	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 5432, PID: 0, ProcessName: "postgres", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 6379, PID: 0, ProcessName: "docker-proxy", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 9999, PID: 0, ProcessName: "mystery-daemon", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runServices(nil) })
	if err != nil {
		t.Fatalf("runServices: %v", err)
	}
	for _, want := range []string{"PostgreSQL", "5432", "Redis", "6379", "Docker", "Unknown service", "9999"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRunServicesJSON(t *testing.T) {
	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 5432, PID: 0, ProcessName: "postgres", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 27017, PID: 0, ProcessName: "mongod", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runServices([]string{"--json"}) })
	if err != nil {
		t.Fatalf("runServices --json: %v", err)
	}

	var entries []ServiceEntryJSON
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Service != "PostgreSQL" || entries[0].Source != "LOCAL" || entries[0].Port != 5432 {
		t.Errorf("entries[0] = %+v, want PostgreSQL/LOCAL/5432", entries[0])
	}
	if entries[1].Service != "MongoDB" || entries[1].Source != "LOCAL" || entries[1].Port != 27017 {
		t.Errorf("entries[1] = %+v, want MongoDB/LOCAL/27017", entries[1])
	}
}

func TestRunServicesSpecificPortMatchesOnlyThatPort(t *testing.T) {
	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 5432, PID: 0, ProcessName: "postgres", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 6379, PID: 0, ProcessName: "redis-server", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runServices([]string{"6379", "--json"}) })
	if err != nil {
		t.Fatalf("runServices: %v", err)
	}
	var entries []ServiceEntryJSON
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if len(entries) != 1 || entries[0].Service != "Redis" || entries[0].Port != 6379 {
		t.Fatalf("unexpected result: %+v", entries)
	}
}

func TestRunServicesEmptyPortJSONIsEmptyArray(t *testing.T) {
	port := freeTCPPort(t)
	out, err := captureStdout(t, func() error {
		return runServices([]string{strconv.Itoa(int(port)), "--json"})
	})
	if err != nil {
		t.Fatalf("runServices: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("runServices(free port, --json) = %q, want []", out)
	}
}
