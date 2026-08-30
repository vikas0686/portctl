package output

import (
	"encoding/json"
	"os"
)

// PrintJSON marshals v as indented JSON to stdout. Used by --json on ls,
// info, and why so portctl's output can be piped into jq or consumed by
// other tools instead of scraped from the table/prose renderers.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
