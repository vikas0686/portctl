package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vikas0686/portctl/internal/portscan"
)

func TestPortEntries(t *testing.T) {
	rows := []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 3000, PID: 82013, ProcessName: "node", State: portscan.StateListen},
		{Protocol: portscan.UDP, LocalPort: 5353, PID: 0, ProcessName: "", State: ""},
	}

	entries := portEntries(rows)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	got := entries[0]
	want := PortEntry{Proto: "tcp", Port: 3000, PID: 82013, Process: "node", State: "LISTEN"}
	if got != want {
		t.Errorf("entries[0] = %+v, want %+v", got, want)
	}

	b, err := json.Marshal(entries[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, unwanted := range []string{`"pid"`, `"process"`, `"state"`} {
		if strings.Contains(s, unwanted) {
			t.Errorf("zero-value field %s should be omitted, got %s", unwanted, s)
		}
	}
	if !strings.Contains(s, `"proto":"udp"`) || !strings.Contains(s, `"port":5353`) {
		t.Errorf("expected proto/port to be present, got %s", s)
	}
}

func TestPortEntriesEmptyIsEmptyArray(t *testing.T) {
	b, err := json.Marshal(portEntries(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != "[]" {
		t.Errorf("portEntries(nil) marshaled to %s, want []", string(b))
	}
}

func TestRunLsJSONProducesValidArray(t *testing.T) {
	out, err := captureStdout(t, func() error { return runLs([]string{"--json"}) })
	if err != nil {
		t.Fatalf("runLs: %v", err)
	}

	var entries []PortEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	for _, e := range entries {
		if e.Proto != "tcp" && e.Proto != "udp" {
			t.Errorf("unexpected proto %q in %+v", e.Proto, e)
		}
	}
}
