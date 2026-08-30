package main

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

// fakeProc is one entry in a synthetic process table for buildAncestryWith
// tests, so cycle/depth/disappearance scenarios can be constructed
// deterministically instead of depending on real, unpredictable OS state.
type fakeProc struct {
	name string
	ppid int
}

func fakeLookup(procs map[int]fakeProc) func(int) (process.Info, error) {
	return func(pid int) (process.Info, error) {
		p, ok := procs[pid]
		if !ok {
			return process.Info{PID: pid}, nil // unresolved, like a real best-effort Lookup miss
		}
		return process.Info{PID: pid, Name: p.name, PPID: p.ppid}, nil
	}
}

func aliveAlways(int) bool { return true }

// --- buildAncestryWith: pure walking-logic tests -------------------------

func TestBuildAncestrySimpleChain(t *testing.T) {
	procs := map[int]fakeProc{
		100: {name: "node", ppid: 50},
		50:  {name: "npm", ppid: 10},
		10:  {name: "zsh", ppid: 0},
	}
	chain := buildAncestryWith(100, "", fakeLookup(procs), aliveAlways)

	want := []ancestryNode{{PID: 100, Name: "node"}, {PID: 50, Name: "npm"}, {PID: 10, Name: "zsh"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryMultiLevel(t *testing.T) {
	procs := map[int]fakeProc{
		500: {name: "java", ppid: 400},
		400: {name: "bash", ppid: 300},
		300: {name: "sshd", ppid: 200},
		200: {name: "sshd", ppid: 1},
		1:   {name: "systemd", ppid: 0},
	}
	chain := buildAncestryWith(500, "", fakeLookup(procs), aliveAlways)

	if len(chain) != 5 {
		t.Fatalf("got %d nodes, want 5: %+v", len(chain), chain)
	}
	if chain[4].PID != 1 || chain[4].Name != "systemd" {
		t.Errorf("chain should terminate at pid 1, got %+v", chain[4])
	}
}

func TestBuildAncestryMissingParent(t *testing.T) {
	// 100's parent is 999, which we have no data for at all.
	procs := map[int]fakeProc{100: {name: "node", ppid: 999}}
	chain := buildAncestryWith(100, "", fakeLookup(procs), aliveAlways)

	want := []ancestryNode{{PID: 100, Name: "node"}, {PID: 999, Note: "unavailable"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryInaccessibleLeaf(t *testing.T) {
	// The leaf itself can't be resolved (e.g. permission denied) and no
	// fallback name was supplied — must stop cleanly, not crash or guess.
	chain := buildAncestryWith(100, "", fakeLookup(map[int]fakeProc{}), aliveAlways)
	want := []ancestryNode{{PID: 100, Note: "unavailable"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryKnownNameFallback(t *testing.T) {
	// Lookup can't resolve the leaf, but the port scan already knew its
	// name — that should be used instead of giving up immediately.
	chain := buildAncestryWith(100, "node", fakeLookup(map[int]fakeProc{}), aliveAlways)
	want := []ancestryNode{{PID: 100, Name: "node"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryProcessDisappearingMidWalk(t *testing.T) {
	procs := map[int]fakeProc{
		100: {name: "node", ppid: 50},
		50:  {name: "npm", ppid: 10},
		10:  {name: "zsh", ppid: 0},
	}
	// 50 exits between being listed as a parent and being looked up.
	alive := func(pid int) bool { return pid != 50 }

	chain := buildAncestryWith(100, "", fakeLookup(procs), alive)
	want := []ancestryNode{{PID: 100, Name: "node"}, {PID: 50, Note: "process exited"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryCycleProtection(t *testing.T) {
	// A cycle can't happen in a real process tree, but the walk must
	// defend against corrupted/pathological data without hanging.
	procs := map[int]fakeProc{
		100: {name: "a", ppid: 200},
		200: {name: "b", ppid: 100},
	}
	chain := buildAncestryWith(100, "", fakeLookup(procs), aliveAlways)

	want := []ancestryNode{{PID: 100, Name: "a"}, {PID: 200, Name: "b"}, {PID: 100, Note: "cycle detected"}}
	assertChain(t, chain, want)
}

func TestBuildAncestryMaxDepthGuard(t *testing.T) {
	procs := make(map[int]fakeProc)
	for i := 0; i < maxAncestryDepth+10; i++ {
		procs[i] = fakeProc{name: "p", ppid: i + 1}
	}
	chain := buildAncestryWith(0, "", fakeLookup(procs), aliveAlways)

	if len(chain) != maxAncestryDepth+1 {
		t.Fatalf("got %d nodes, want %d (depth guard + terminal note)", len(chain), maxAncestryDepth+1)
	}
	last := chain[len(chain)-1]
	if last.Note != "ancestry too deep, stopped" {
		t.Errorf("expected a too-deep note, got %+v", last)
	}
}

func assertChain(t *testing.T, got, want []ancestryNode) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("chain = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// --- buildAncestry: real-process integration test ------------------------

func TestBuildAncestryRealProcess(t *testing.T) {
	// A real child of the test binary gives a genuine, OS-verified chain:
	// the spawned pid, and (at minimum) this test process as its parent.
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	chain := buildAncestry(pid, "sleep")
	if len(chain) < 2 {
		t.Fatalf("expected at least [sleep, test-process], got %+v", chain)
	}
	if chain[0].PID != pid || chain[0].Note != "" {
		t.Errorf("chain[0] = %+v, want a resolved node for pid %d", chain[0], pid)
	}
	if chain[1].Name == "" && chain[1].Note == "" {
		t.Errorf("chain[1] has neither a name nor a note: %+v", chain[1])
	}
}

// --- CLI orchestration ----------------------------------------------------

func TestRunTreeSpecificPortNotFound(t *testing.T) {
	port := freeTCPPort(t)
	out, err := captureStdout(t, func() error { return runTree([]string{strconv.Itoa(int(port))}) })
	if err != nil {
		t.Fatalf("runTree: %v", err)
	}
	want := "nothing on port " + strconv.Itoa(int(port)) + ".\n"
	if out != want {
		t.Errorf("runTree(free port) = %q, want %q", out, want)
	}
}

func TestRunTreeNothingListening(t *testing.T) {
	withFakeScanner(t, nil)
	out, err := captureStdout(t, func() error { return runTree(nil) })
	if err != nil {
		t.Fatalf("runTree: %v", err)
	}
	if strings.TrimSpace(out) != "nothing listening." {
		t.Errorf("runTree() with no rows = %q, want %q", out, "nothing listening.")
	}
}

func TestRunTreeSkipsUnownedPort(t *testing.T) {
	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 5001, PID: 0, State: portscan.StateTimeWait},
	})
	out, err := captureStdout(t, func() error { return runTree(nil) })
	if err != nil {
		t.Fatalf("runTree: %v", err)
	}
	if strings.TrimSpace(out) != "nothing listening." {
		t.Errorf("runTree() with only an unowned port = %q, want %q", out, "nothing listening.")
	}
}

func TestRunTreeSpecificPortJSON(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 6001, PID: pid, ProcessName: "sleep", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runTree([]string{"6001", "--json"}) })
	if err != nil {
		t.Fatalf("runTree --json: %v", err)
	}
	var results []TreeResultJSON
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if len(results) != 1 || results[0].Port != 6001 || results[0].Proto != "tcp" {
		t.Fatalf("unexpected result: %+v", results)
	}
	if len(results[0].Ancestry) == 0 || results[0].Ancestry[0].PID != pid {
		t.Errorf("expected ancestry to start at pid %d, got %+v", pid, results[0].Ancestry)
	}
}

func TestRunTreeAllPortsGroupsByPort(t *testing.T) {
	cmdA := exec.Command("sleep", "5")
	cmdB := exec.Command("sleep", "5")
	if err := cmdA.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	t.Cleanup(func() { cmdA.Process.Kill(); cmdA.Wait() })
	if err := cmdB.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	t.Cleanup(func() { cmdB.Process.Kill(); cmdB.Wait() })

	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 7002, PID: cmdB.Process.Pid, ProcessName: "sleep", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 7001, PID: cmdA.Process.Pid, ProcessName: "sleep", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runTree(nil) })
	if err != nil {
		t.Fatalf("runTree: %v", err)
	}
	idx1 := strings.Index(out, "7001/tcp")
	idx2 := strings.Index(out, "7002/tcp")
	if idx1 == -1 || idx2 == -1 {
		t.Fatalf("expected both ports in output, got: %s", out)
	}
	if idx1 > idx2 {
		t.Errorf("expected ports sorted ascending, got 7001 at %d and 7002 at %d", idx1, idx2)
	}
	if !strings.Contains(out, "└──") {
		t.Errorf("expected tree connectors in output, got: %s", out)
	}
}
