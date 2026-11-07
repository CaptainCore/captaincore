package cmd

import (
	"os"
	"strings"
	"testing"
	"time"
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

func TestBreakwaterWordsScript(t *testing.T) {
	body := []byte(`{"v":1,"generated":"2026-11-07T00:00:00+00:00","p":{"0123456789ab":990},"k":["0123456789"]}`)
	script := breakwaterWordsScript(body)
	for _, want := range []string{"<<'BREAKWATERWORDSEOF'\n" + string(body) + "\nBREAKWATERWORDSEOF\n", `class_exists("Breakwater\\Words")`, `Breakwater\Words::store( stream_get_contents( STDIN ) )`} {
		if !strings.Contains(script, want) {
			t.Fatalf("script is missing %q:\n%s", want, script)
		}
	}
	if generated, clues := breakwaterWordsMeta(body); generated != "2026-11-07T00:00:00+00:00" || clues != 1 {
		t.Fatalf("meta: %q %d", generated, clues)
	}
	// BREAKWATER_WORDS_OUT=<file> writes a script carrying the model in BREAKWATER_WORDS_IN.
	if out := os.Getenv("BREAKWATER_WORDS_OUT"); out != "" {
		if real, err := os.ReadFile(os.Getenv("BREAKWATER_WORDS_IN")); err == nil {
			script = breakwaterWordsScript(real)
		}
		os.WriteFile(out, []byte(script), 0700)
	}
}

func TestBreakwaterWordsDue(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-11-07T12:00:00Z")
	cases := []struct {
		held, generated string
		want            bool
	}{
		{"", "2026-11-07T00:00:00+00:00", true},                           // none yet
		{"2026-11-06T00:00:00+00:00", "2026-11-07T00:00:00+00:00", false}, // a day old: keep
		{"2026-11-03T00:00:00+00:00", "2026-11-07T00:00:00+00:00", true},  // four days old: refresh
		{"2026-11-07T00:00:00+00:00", "2026-11-07T00:00:00+00:00", false}, // the same model
		{"garbage", "2026-11-07T00:00:00+00:00", true},                    // unreadable stamp
		{"", "", false}, // no model to send
	}
	for _, c := range cases {
		if got := breakwaterWordsDue(c.held, c.generated, now); got != c.want {
			t.Errorf("due(%q, %q) = %v, want %v", c.held, c.generated, got, c.want)
		}
	}
}
