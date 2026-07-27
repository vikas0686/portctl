// Package output renders portctl's results for a human terminal. Detects
// TTY vs. pipe with the stdlib only (no golang.org/x/term dependency yet)
// and falls back to plain, uncolored, alignment-only output when piped.
package output

import (
	"fmt"
	"os"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
)

// IsTTY reports whether stdout looks like an interactive terminal. Piping
// or redirecting output should always fall back to plain formatting.
func IsTTY() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorize(s, code string) string {
	if !IsTTY() {
		return s
	}
	return code + s + colorReset
}

func Green(s string) string  { return colorize(s, colorGreen) }
func Yellow(s string) string { return colorize(s, colorYellow) }
func Dim(s string) string    { return colorize(s, colorDim) }
func Red(s string) string    { return colorize(s, colorRed) }

// Table renders simple left-aligned, space-padded columns, tabwriter-style
// but dependency-free: pad by the visible (uncolored) width of each cell
// so ANSI codes don't throw off alignment.
type Table struct {
	Headers []string
	Rows    [][]string
}

func (t Table) Render() string {
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = visibleLen(h)
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if l := visibleLen(cell); l > widths[i] {
				widths[i] = l
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, cell := range cells {
			pad := widths[i] - visibleLen(cell)
			b.WriteString(cell)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", pad+2))
			}
		}
		b.WriteString("\n")
	}

	writeRow(t.Headers)
	for _, row := range t.Rows {
		writeRow(row)
	}
	return b.String()
}

// visibleLen counts runes while skipping ANSI escape sequences, so colored
// cells still line up with plain ones.
func visibleLen(s string) int {
	n := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		n++
	}
	return n
}

func Fprintln(a ...any) { fmt.Println(a...) }
