package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vikas0686/portctl/internal/portscan"
	"github.com/vikas0686/portctl/internal/process"
)

// --- pure decision-rule tests (evaluateStaleness) -------------------------

func TestEvaluateStalenessNoSignals(t *testing.T) {
	info := process.Info{PID: 123, Name: "node", Cwd: os.TempDir()}
	if reasons := evaluateStaleness(info); reasons != nil {
		t.Errorf("expected no reasons for an intact process, got %v", reasons)
	}
}

func TestEvaluateStalenessVanishedMidLookup(t *testing.T) {
	// A zero-value Info is exactly what a best-effort Lookup returns when
	// the process disappeared between being listed and being inspected
	// (every OS backend leaves fields empty rather than erroring). That
	// must resolve to "not flagged", never a crash or a guess.
	if reasons := evaluateStaleness(process.Info{}); reasons != nil {
		t.Errorf("expected no reasons for an unresolved process, got %v", reasons)
	}
}

func TestEvaluateStalenessDeletedCwd(t *testing.T) {
	info := process.Info{PID: 123, Name: "node", Cwd: "/definitely/does/not/exist/xyz"}
	reasons := evaluateStaleness(info)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "working directory no longer exists") {
		t.Errorf("evaluateStaleness(deleted cwd) = %v, want one deleted-cwd reason", reasons)
	}
}

func TestEvaluateStalenessDeletedExe(t *testing.T) {
	info := process.Info{PID: 123, Name: "node", ExeDeleted: true}
	reasons := evaluateStaleness(info)
	if len(reasons) != 1 || !strings.Contains(reasons[0], "executable has been deleted") {
		t.Errorf("evaluateStaleness(deleted exe) = %v, want one deleted-exe reason", reasons)
	}
}

func TestEvaluateStalenessReparentedAloneIsNotEnough(t *testing.T) {
	// PPID 1 is the ordinary, permanent state of any intentionally
	// daemonized process (nohup, double-fork). On its own it must never
	// be sufficient to flag something for killing.
	info := process.Info{PID: 123, Name: "node", Cwd: os.TempDir(), PPID: 1}
	if reasons := evaluateStaleness(info); reasons != nil {
		t.Errorf("expected reparented-alone to be insufficient, got %v", reasons)
	}
}

func TestEvaluateStalenessReparentedWithStrongSignal(t *testing.T) {
	info := process.Info{PID: 123, Name: "node", Cwd: "/definitely/does/not/exist/xyz", PPID: 1}
	reasons := evaluateStaleness(info)
	if len(reasons) != 2 {
		t.Fatalf("evaluateStaleness(deleted cwd + ppid 1) = %v, want 2 reasons", reasons)
	}
	if !strings.Contains(reasons[1], "reparented to init") {
		t.Errorf("expected reparented reason to accompany the strong one, got %v", reasons)
	}
}

// --- isProtectedProcess -----------------------------------------------

func TestIsProtectedProcess(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"launchd", true},
		{"LAUNCHD", true},
		{"systemd", true},
		{"dockerd", true},
		{"com.docker.backend", true},
		{"node", false},
		{"python3", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isProtectedProcess(c.name); got != c.want {
			t.Errorf("isProtectedProcess(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// --- staleCandidates: real-process integration tests --------------------

// spawnSleeper starts a real, briefly-alive background process in dir and
// returns its PID plus a cleanup func. Using an actual OS process (rather
// than faking process.Info) exercises the real per-OS process.Lookup path.
func spawnSleeper(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return pid
}

func TestStaleCandidatesIntactProcessNotFlagged(t *testing.T) {
	dir := t.TempDir()
	pid := spawnSleeper(t, dir)

	rows := []portscan.Port{{Protocol: portscan.TCP, LocalPort: 4001, PID: pid, ProcessName: "sleep", State: portscan.StateListen}}
	got := staleCandidates(rows)
	if len(got) != 0 {
		t.Errorf("expected an intact process not to be flagged, got %+v", got)
	}
}

func TestStaleCandidatesDeletedWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	pid := spawnSleeper(t, dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("removing dir out from under the process: %v", err)
	}

	rows := []portscan.Port{{Protocol: portscan.TCP, LocalPort: 4002, PID: pid, ProcessName: "sleep", State: portscan.StateListen}}
	got := staleCandidates(rows)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
	}
	if got[0].Port.LocalPort != 4002 {
		t.Errorf("candidate port = %d, want 4002", got[0].Port.LocalPort)
	}
	found := false
	for _, r := range got[0].Reasons {
		if strings.Contains(r, "working directory no longer exists") {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %v, want a deleted-working-directory reason", got[0].Reasons)
	}
}

func TestStaleCandidatesProtectedNameNeverFlagged(t *testing.T) {
	dir := t.TempDir()
	pid := spawnSleeper(t, dir)
	os.RemoveAll(dir) // strong signal present...

	// ...but the port-table's process name says this is a protected
	// system process, which must veto the finding regardless of signals.
	rows := []portscan.Port{{Protocol: portscan.TCP, LocalPort: 4003, PID: pid, ProcessName: "launchd", State: portscan.StateListen}}
	got := staleCandidates(rows)
	if len(got) != 0 {
		t.Errorf("expected a protected process name to never be flagged, got %+v", got)
	}
}

func TestStaleCandidatesSelfNeverFlagged(t *testing.T) {
	rows := []portscan.Port{{Protocol: portscan.TCP, LocalPort: 4004, PID: os.Getpid(), ProcessName: "portctl", State: portscan.StateListen}}
	got := staleCandidates(rows)
	if len(got) != 0 {
		t.Errorf("expected portctl's own PID to never be flagged, got %+v", got)
	}
}

func TestStaleCandidatesVanishedProcessSkippedGracefully(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("running short-lived process: %v", err)
	}
	pid := cmd.Process.Pid // reaped and exited by the time Run() returns

	rows := []portscan.Port{{Protocol: portscan.TCP, LocalPort: 4005, PID: pid, ProcessName: "true", State: portscan.StateListen}}
	got := staleCandidates(rows) // must not panic
	if len(got) != 0 {
		t.Errorf("expected a vanished process to be skipped, got %+v", got)
	}
}

func TestStaleCandidatesSortedByPortAndDeduped(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	pidA := spawnSleeper(t, dirA)
	pidB := spawnSleeper(t, dirB)
	os.RemoveAll(dirA)
	os.RemoveAll(dirB)

	rows := []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 9002, PID: pidB, ProcessName: "sleep", State: portscan.StateListen},
		{Protocol: portscan.TCP, LocalPort: 9001, PID: pidA, ProcessName: "sleep", State: portscan.StateListen},
		// A duplicate row for the same PID (e.g. multiple fds on the same
		// socket) must not produce a duplicate finding.
		{Protocol: portscan.TCP, LocalPort: 9001, PID: pidA, ProcessName: "sleep", State: portscan.StateListen},
	}
	got := staleCandidates(rows)
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2 (deduped): %+v", len(got), got)
	}
	if got[0].Port.LocalPort != 9001 || got[1].Port.LocalPort != 9002 {
		t.Errorf("candidates not sorted by port: %d, %d", got[0].Port.LocalPort, got[1].Port.LocalPort)
	}
}

// fakeScanner implements portscan.Scanner over a fixed row set, so CLI
// orchestration tests can seed a deterministic "stale process" without
// depending on the real machine's live port table.
type fakeScanner struct{ rows []portscan.Port }

func (f fakeScanner) List() ([]portscan.Port, error) { return f.rows, nil }

func withFakeScanner(t *testing.T, rows []portscan.Port) {
	t.Helper()
	orig := newScanner
	newScanner = func() portscan.Scanner { return fakeScanner{rows: rows} }
	t.Cleanup(func() { newScanner = orig })
}

// --- CLI orchestration: dry-run / confirm / --yes / --json / empty -------

func TestRunCleanEmptyResult(t *testing.T) {
	// A machine with no stale candidates is the common case in CI.
	out, err := captureStdout(t, func() error { return runClean(nil) })
	if err != nil {
		t.Fatalf("runClean: %v", err)
	}
	if !strings.Contains(out, "No stale development processes found.") {
		// Not necessarily an error on a dirty dev machine, but on a clean
		// CI runner this is the expected, and the important, path.
		t.Logf("runClean output (informational): %s", out)
	}
}

func TestRunCleanJSONNeverPrompts(t *testing.T) {
	// --json must be report-only: it must return promptly with valid JSON
	// and never block on stdin for a confirmation.
	out, err := captureStdout(t, func() error { return runClean([]string{"--json"}) })
	if err != nil {
		t.Fatalf("runClean --json: %v", err)
	}
	var candidates []CleanCandidateJSON
	if err := json.Unmarshal([]byte(out), &candidates); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
}

func TestRunCleanDryRunNeverKills(t *testing.T) {
	dir := t.TempDir()
	pid := spawnSleeper(t, dir)
	os.RemoveAll(dir)

	// runClean scans the live system, so seed a real stale process and
	// confirm it's reported without needing confirmation, and left alive.
	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 4006, PID: pid, ProcessName: "sleep", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runClean([]string{"--dry-run"}) })
	if err != nil {
		t.Fatalf("runClean --dry-run: %v", err)
	}
	if !strings.Contains(out, "4006/tcp") {
		t.Fatalf("expected the stale candidate to be reported, got: %s", out)
	}
	if !process.Alive(pid) {
		t.Errorf("--dry-run must never kill anything, but pid %d is gone", pid)
	}
}

func TestRunCleanDeclineDoesNotKill(t *testing.T) {
	dir := t.TempDir()
	pid := spawnSleeper(t, dir)
	os.RemoveAll(dir)

	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 4007, PID: pid, ProcessName: "sleep", State: portscan.StateListen},
	})

	restoreStdin := stubStdin(t, "n\n")
	defer restoreStdin()

	out, err := captureStdout(t, func() error { return runClean(nil) })
	if err != nil {
		t.Fatalf("runClean: %v", err)
	}
	if !strings.Contains(out, "aborted.") {
		t.Errorf("expected an aborted message on decline, got: %s", out)
	}
	if !process.Alive(pid) {
		t.Errorf("declining the prompt must not kill anything, but pid %d is gone", pid)
	}
}

func TestRunCleanYesKillsWithoutPrompting(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("sleep", "30")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	pid := cmd.Process.Pid
	os.RemoveAll(dir)

	withFakeScanner(t, []portscan.Port{
		{Protocol: portscan.TCP, LocalPort: 4008, PID: pid, ProcessName: "sleep", State: portscan.StateListen},
	})

	out, err := captureStdout(t, func() error { return runClean([]string{"--yes"}) })
	if err != nil {
		t.Fatalf("runClean --yes: %v", err)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("--yes must skip the confirmation prompt, got: %s", out)
	}

	// Wait() (rather than process.Alive, which reports a reaped-pending
	// zombie as "alive") confirms the process actually terminated.
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case werr := <-waitErr:
		if werr == nil {
			t.Errorf("expected the process to have exited via signal, got a clean exit")
		}
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		<-waitErr
		t.Errorf("--yes must kill flagged candidates, but pid %d did not exit in time", pid)
	}
}
