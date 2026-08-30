package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestPrintJSON(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	in := sample{Name: "port", N: 3000}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	err = PrintJSON(in)
	w.Close()
	os.Stdout = orig
	if err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}

	var out sample
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, buf.String())
	}
	if out != in {
		t.Errorf("round-tripped %+v, want %+v", out, in)
	}

	if !bytes.Contains(buf.Bytes(), []byte("  \"name\"")) {
		t.Errorf("expected 2-space indentation, got %s", buf.String())
	}
}

func TestPrintJSONEmptySlice(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	err = PrintJSON([]int{})
	w.Close()
	os.Stdout = orig
	if err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	if got := buf.String(); got != "[]\n" {
		t.Errorf("PrintJSON([]int{}) wrote %q, want \"[]\\n\"", got)
	}
}
