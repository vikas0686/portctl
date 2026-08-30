package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it, so run* functions (which print directly) can
// be tested without a mockable writer.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fnErr := fn()

	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return buf.String(), fnErr
}

// stubStdin replaces os.Stdin with a pipe pre-loaded with input, for
// exercising confirm()-style prompts without touching the real terminal.
// Call the returned func to restore the original os.Stdin.
func stubStdin(t *testing.T, input string) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("writing stub stdin: %v", err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	return func() { os.Stdin = orig }
}

// freeTCPPort finds a port that's very likely unused by binding to :0 and
// releasing it immediately.
func freeTCPPort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}
