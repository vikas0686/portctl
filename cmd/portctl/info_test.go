package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/vikas0686/portctl/internal/portscan"
)

func TestPortDetailsUnknownOwner(t *testing.T) {
	rows := []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 4000, State: portscan.StateTimeWait},
	}

	details := portDetails(rows)
	if len(details) != 1 {
		t.Fatalf("got %d details, want 1", len(details))
	}
	d := details[0]
	if d.Proto != "tcp" || d.Port != 4000 || d.State != "TIME_WAIT" {
		t.Errorf("unexpected base fields: %+v", d)
	}
	if d.PID != 0 || d.Process != "" || d.Cmdline != "" || d.Cwd != "" {
		t.Errorf("expected no owner fields for PID 0, got %+v", d)
	}
	if d.CPUPercent != nil || d.MemRSSKb != nil {
		t.Errorf("expected nil cpu/mem for unresolved owner, got %+v / %+v", d.CPUPercent, d.MemRSSKb)
	}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, unwanted := range []string{`"pid"`, `"cpu_percent"`, `"mem_rss_kb"`, `"cmdline"`, `"cwd"`} {
		if strings.Contains(string(b), unwanted) {
			t.Errorf("expected %s to be omitted, got %s", unwanted, string(b))
		}
	}
}

func TestPortDetailsResolvesOwner(t *testing.T) {
	rows := []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 4001, PID: os.Getpid(), ProcessName: "fallback", State: portscan.StateListen},
	}

	details := portDetails(rows)
	if len(details) != 1 {
		t.Fatalf("got %d details, want 1", len(details))
	}
	d := details[0]
	if d.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", d.PID, os.Getpid())
	}
	if d.CPUPercent == nil || d.MemRSSKb == nil {
		t.Errorf("expected cpu/mem to be populated for a real running process, got %+v / %+v", d.CPUPercent, d.MemRSSKb)
	}
}

func TestRunInfoJSONEmptyIsEmptyArray(t *testing.T) {
	port := freeTCPPort(t)
	out, err := captureStdout(t, func() error {
		return runInfo([]string{strconv.Itoa(int(port)), "--json"})
	})
	if err != nil {
		t.Fatalf("runInfo: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("runInfo --json on a free port = %q, want []", out)
	}
}
