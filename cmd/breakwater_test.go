package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestBreakwaterScript(t *testing.T) {
	body := []byte(`{"generated":"2026-10-19T00:00:00+00:00","email":{"0123456789abcdef":4}}`)
	script := breakwaterScript(body)
	for _, want := range []string{"<<'BREAKWATERLISTEOF'\n" + string(body) + "\nBREAKWATERLISTEOF\n", `class_exists("Breakwater\\Reputation")`, `Breakwater\Reputation::store(`} {
		if !strings.Contains(script, want) {
			t.Fatalf("script is missing %q:\n%s", want, script)
		}
	}
	// BREAKWATER_SCRIPT_OUT=<file> writes a script to run by hand on a test site, carrying the
	// list in BREAKWATER_SCRIPT_IN when given (a Manager export saved to a file).
	if out := os.Getenv("BREAKWATER_SCRIPT_OUT"); out != "" {
		if in := os.Getenv("BREAKWATER_SCRIPT_IN"); in != "" {
			if real, err := os.ReadFile(in); err == nil {
				script = breakwaterScript(real)
			}
		}
		os.WriteFile(out, []byte(script), 0700)
	}
}

func TestBreakwaterGenerated(t *testing.T) {
	if got := breakwaterGenerated([]byte(`{"generated":"x","ip":{}}`)); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := breakwaterGenerated([]byte(`not json`)); got != "" {
		t.Fatalf("got %q", got)
	}
}
