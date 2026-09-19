package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CaptainCore/captaincore/typesafe"
)

func writeTriageFixture(t *testing.T, dir, rel string, lines int) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	var b strings.Builder
	b.WriteString("<?php\n")
	for i := 2; i <= lines; i++ {
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	os.WriteFile(p, []byte(b.String()), 0o644)
	return p
}

func TestBuildTriageStateExcerptAndLocation(t *testing.T) {
	dir := t.TempDir()
	p := writeTriageFixture(t, dir, "wp-content/uploads/2026/x.php", 120)
	st := buildTriageState(Finding{File: "uploads/2026/x.php", Path: p, Line: 60, Name: "r", Severity: "high", Match: "eval("}, 5)
	if st.Source.FromLine != 55 || st.Source.ToLine != 65 || st.Source.FileLines != 121 {
		t.Errorf("window: %+v", st.Source)
	}
	if !strings.HasPrefix(st.Source.Excerpt, "55: // line 55\n") || !strings.Contains(st.Source.Excerpt, "\n60: // line 60\n") {
		t.Errorf("excerpt: %q", st.Source.Excerpt)
	}
	if !strings.Contains(st.Finding.Location, "uploads") {
		t.Errorf("location hint: %q", st.Finding.Location)
	}

	// No line (hash hit): the head of the file, clamped to the window.
	st = buildTriageState(Finding{Path: p, Line: 0}, 5)
	if st.Source.FromLine != 1 || st.Source.ToLine != 11 {
		t.Errorf("head window: %+v", st.Source)
	}

	// Near the top: from clamps to 1.
	st = buildTriageState(Finding{Path: p, Line: 2}, 5)
	if st.Source.FromLine != 1 || st.Source.ToLine != 7 {
		t.Errorf("clamped window: %+v", st.Source)
	}

	// Missing file: state still built, with a note.
	st = buildTriageState(Finding{Path: filepath.Join(dir, "nope.php"), Line: 3}, 5)
	if st.Source.Note == "" || st.Source.Excerpt != "" {
		t.Errorf("missing file: %+v", st.Source)
	}
}

func TestBuildTriageStateCapsLongLinesAndExcerpt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.php")
	long := strings.Repeat("A", 2000)
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(long + "\n")
	}
	os.WriteFile(p, []byte(b.String()), 0o644)
	st := buildTriageState(Finding{Path: p, Line: 25, Match: strings.Repeat("m", 5000)}, 25)
	if len(st.Source.Excerpt) > triageMaxExcerpt+500 || !st.Source.Truncated {
		t.Errorf("excerpt not capped: len=%d truncated=%v", len(st.Source.Excerpt), st.Source.Truncated)
	}
	if len(st.Finding.MatchedText) > triageMaxMatch+10 {
		t.Errorf("match not clipped: %d", len(st.Finding.MatchedText))
	}
}

func TestTriageFindingsRanksAndKeepsFailures(t *testing.T) {
	dir := t.TempDir()
	shell := writeTriageFixture(t, dir, "wp-content/uploads/shell.php", 10)
	legit := writeTriageFixture(t, dir, "wp-content/plugins/x/x.php", 10)
	broken := writeTriageFixture(t, dir, "wp-content/plugins/y/y.php", 10)

	var mu sync.Mutex
	var seen []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			State     triageState                  `json:"state"`
			Model     string                       `json:"model"`
			Questions map[string]typesafe.Question `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		seen = append(seen, map[string]interface{}{"file": body.State.Finding.File, "model": body.Model, "n": len(body.Questions)})
		mu.Unlock()
		switch body.State.Finding.File {
		case "uploads/shell.php":
			w.Write([]byte(`{"model":"jev-1.13.0","answers":{"true_positive":{"type":"noul","noul":0.97},"family":{"type":"choice","choice":"webshell","confidence":0.9},"action":{"type":"choice","choice":"quarantine","confidence":0.8}},"usage":{"input_tokens":400}}`))
		case "plugins/x/x.php":
			w.Write([]byte(`{"model":"jev-1.13.0","answers":{"true_positive":{"type":"noul","noul":0.08},"family":{"type":"choice","choice":"benign_other","confidence":0.7},"action":{"type":"choice","choice":"ignore","confidence":0.75}},"usage":{"input_tokens":380}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"bad state"}`))
		}
	}))
	defer srv.Close()

	client := typesafe.NewClient("k")
	client.BaseURL = srv.URL
	client.Model = "jev-1.13.0"
	findings := []Finding{
		{File: "plugins/x/x.php", Path: legit, Line: 3, Severity: "high"},
		{File: "plugins/y/y.php", Path: broken, Line: 3, Severity: "critical"},
		{File: "uploads/shell.php", Path: shell, Line: 3, Severity: "high"},
	}
	out := TriageFindings(context.Background(), client, findings, TriageOptions{Workers: 2})
	if len(out) != 3 {
		t.Fatalf("want 3, got %d", len(out))
	}
	if out[0].File != "uploads/shell.php" || out[0].Triage.TruePositive != 0.97 || out[0].Triage.Family != "webshell" || out[0].Triage.Action != "quarantine" {
		t.Errorf("first: %+v %+v", out[0].Finding, out[0].Triage)
	}
	if out[1].File != "plugins/x/x.php" || out[1].Triage.TruePositive != 0.08 {
		t.Errorf("second: %+v %+v", out[1].Finding, out[1].Triage)
	}
	if out[2].File != "plugins/y/y.php" || out[2].Triage == nil || !strings.Contains(out[2].Triage.Error, "HTTP 400") {
		t.Errorf("failed call should sort last with its error: %+v", out[2].Triage)
	}
	if out[0].Triage.Model != "jev-1.13.0" || out[0].Triage.InputTokens != 400 {
		t.Errorf("meta: %+v", out[0].Triage)
	}
	if len(seen) != 3 {
		t.Fatalf("calls: %v", seen)
	}
	for _, s := range seen {
		if s["n"] != 3 || s["model"] != "jev-1.13.0" {
			t.Errorf("request: %v", s)
		}
	}

	// JSON output carries the finding fields plus a triage object.
	raw, _ := json.Marshal(out[0])
	if !strings.Contains(string(raw), `"triage":{"true_positive":0.97`) || !strings.Contains(string(raw), `"file":"uploads/shell.php"`) {
		t.Errorf("json: %s", raw)
	}
}

func TestTriageFindingsNoClientOrFindings(t *testing.T) {
	if out := TriageFindings(context.Background(), nil, []Finding{{File: "a"}}, TriageOptions{}); len(out) != 1 || out[0].Triage != nil {
		t.Errorf("nil client: %+v", out)
	}
	if out := TriageFindings(context.Background(), typesafe.NewClient("k"), nil, TriageOptions{}); len(out) != 0 {
		t.Errorf("no findings: %+v", out)
	}
}
