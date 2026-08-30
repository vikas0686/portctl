package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/vikas0686/portctl/internal/portscan"
)

func TestWhyResultFree(t *testing.T) {
	r := whyResult(9999, nil)
	if !r.Free || r.Port != 9999 || len(r.Matches) != 0 {
		t.Errorf("whyResult(9999, nil) = %+v, want free result with no matches", r)
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"matches"`) {
		t.Errorf("expected matches to be omitted when empty, got %s", string(b))
	}
}

func TestWhyResultWithMatches(t *testing.T) {
	matches := []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 3000, PID: 82013, ProcessName: "node", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 3000, State: portscan.StateTimeWait, RemoteAddr: "10.0.0.5", RemotePort: 54321},
	}

	r := whyResult(3000, matches)
	if r.Free {
		t.Errorf("whyResult with matches reported Free=true")
	}
	if len(r.Matches) != 2 {
		t.Fatalf("got %d matches, want 2", len(r.Matches))
	}

	got := r.Matches[0]
	want := WhyMatch{Proto: "tcp", State: "LISTEN", PID: 82013, Process: "node"}
	if got != want {
		t.Errorf("Matches[0] = %+v, want %+v", got, want)
	}

	got = r.Matches[1]
	if got.PID != 0 || got.State != "TIME_WAIT" || got.RemoteAddr != "10.0.0.5" || got.RemotePort != 54321 {
		t.Errorf("Matches[1] = %+v, unexpected fields", got)
	}
}

func TestRunWhyJSONFreePort(t *testing.T) {
	port := freeTCPPort(t)
	out, err := captureStdout(t, func() error {
		return runWhy([]string{strconv.Itoa(int(port)), "--json"})
	})
	if err != nil {
		t.Fatalf("runWhy: %v", err)
	}

	var r WhyResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if !r.Free || r.Port != port {
		t.Errorf("runWhy --json on a free port = %+v, want free result for port %d", r, port)
	}
}
