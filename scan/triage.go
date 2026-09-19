package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/CaptainCore/captaincore/typesafe"
)

// Triage is Jev's judgment of one finding (see typesafe package). It never
// replaces the rule match; it ranks findings so the ones worth a human's time
// come first and the known false-positive shapes sink.
type Triage struct {
	// TruePositive is the probability the flagged code is real malware rather
	// than a false positive of the rule.
	TruePositive float64 `json:"true_positive"`
	// Family is the malware family the code looks like, or a benign_* bucket.
	Family           string  `json:"family"`
	FamilyConfidence float64 `json:"family_confidence"`
	// Action is the recommended next step for hosting operations.
	Action           string  `json:"action"`
	ActionConfidence float64 `json:"action_confidence"`
	// Model is the versioned model that answered.
	Model string `json:"model"`
	// InputTokens is what the call cost; output tokens are free.
	InputTokens int `json:"input_tokens"`
	// Error is set when the call failed; the other fields are then zero.
	Error string `json:"error,omitempty"`
}

// TriagedFinding is a Finding with its Triage attached.
type TriagedFinding struct {
	Finding
	Triage *Triage `json:"triage,omitempty"`
}

// TriageFamilies are the options offered for the family question, in the order
// they are shown to the model.
var TriageFamilies = map[string]string{
	"webshell":           "Interactive shell or file manager: runs commands, edits or uploads files on request",
	"backdoor_auth":      "Creates or logs in an administrator, sets auth cookies, or bypasses login with a secret",
	"loader":             "Decodes, fetches or includes a hidden payload and executes it (eval, include, create_function)",
	"spam_seo":           "Injects hidden links, cloaked pages or search-engine only content",
	"redirect_skimmer":   "Injects JavaScript or redirects visitors, steals form or card data",
	"dropper":            "Writes new PHP files, unpacks archives, or reinstalls itself",
	"benign_security":    "A legitimate security or maintenance tool that contains detection strings or lock files",
	"benign_obfuscation": "Legitimate minified, packed, licensed or encoded code with an ordinary purpose",
	"benign_other":       "Ordinary application code that merely matches the pattern",
}

// TriageActions are the options offered for the action question.
var TriageActions = map[string]string{
	"ignore":     "False positive; dismiss",
	"watch":      "Probably fine; log it and re-check on the next scan",
	"flag":       "Unclear; a person should read the file",
	"quarantine": "Malicious; disable or remove the file or plugin",
	"page":       "Active compromise with admin or shell access; wake someone up",
}

// TriageOptions tunes TriageFindings.
type TriageOptions struct {
	Workers int    // concurrent API calls (default 8)
	Model   string // model name or alias (default: client's)
	// ContextLines is how many source lines to send around the match (default 25).
	ContextLines int
}

const (
	triageMaxExcerpt = 8000
	triageMaxMatch   = 600
)

// triageQuestions is the fixed question set. Question ids are not seen by the
// model, so every instruction carries its full meaning.
func triageQuestions() typesafe.Questions {
	return typesafe.Questions{
		"true_positive": typesafe.Noul(
			"A malware scanner flagged the file described in `finding` because its contents matched the rule named there. "+
				"Judging only from `source.excerpt`, `finding.matched_text` and the file location, is this real malicious code "+
				"rather than a false positive of the rule?",
			"Real malware: a web shell, an admin or login backdoor, code that decodes or fetches and then executes a hidden payload, "+
				"hidden spam links or cloaking, injected redirects or skimmers, or a dropper that writes PHP files.",
			"A false positive: ordinary plugin, theme or library code that happens to match; legitimate minified, packed, "+
				"licensed or encoded code; a security tool's own detection strings; a test fixture; or too little evidence to say.",
		),
		"family": typesafe.Choice(
			"Which description best fits what the code in `source.excerpt` does? Choose a benign option when the code has an ordinary purpose.",
			TriageFamilies,
		),
		"action": typesafe.Choice(
			"What should a hosting operator do next about this finding, based on `source.excerpt` and the rule severity in `finding.severity`?",
			TriageActions,
		),
	}
}

// triageState is the JSON sent as the state for one finding.
type triageState struct {
	Finding triageFinding `json:"finding"`
	Source  triageSource  `json:"source"`
}

type triageFinding struct {
	File        string `json:"file"`
	Location    string `json:"location"`
	Rule        string `json:"rule"`
	Family      string `json:"rule_family,omitempty"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
	MatchedText string `json:"matched_text,omitempty"`
	Layer       string `json:"decoded_layer,omitempty"`
}

type triageSource struct {
	Excerpt   string `json:"excerpt"`
	FromLine  int    `json:"from_line"`
	ToLine    int    `json:"to_line"`
	FileLines int    `json:"file_lines"`
	FileBytes int64  `json:"file_bytes"`
	Truncated bool   `json:"truncated,omitempty"`
	Note      string `json:"note,omitempty"`
}

// locationHint names where in a WordPress tree the file sits, which is
// evidence on its own (PHP under uploads is never legitimate).
func locationHint(path string) string {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "/uploads/"):
		return "inside wp-content/uploads (PHP here is never legitimate)"
	case strings.Contains(p, "/mu-plugins/"):
		return "a must-use plugin (loads on every request, cannot be deactivated from the admin)"
	case strings.Contains(p, "/plugins/"):
		return "inside a plugin directory"
	case strings.Contains(p, "/themes/"):
		return "inside a theme directory"
	case strings.Contains(p, "/wp-admin/") || strings.Contains(p, "/wp-includes/"):
		return "inside WordPress core (core files are never modified legitimately)"
	case strings.Contains(p, "/cache/"):
		return "inside a cache directory"
	}
	base := filepath.Base(p)
	if strings.HasPrefix(base, ".") {
		return "a hidden dot-file"
	}
	return "in the site root or an unusual location"
}

// buildTriageState reads the source around the match and assembles the state.
func buildTriageState(f Finding, contextLines int) triageState {
	st := triageState{
		Finding: triageFinding{
			File:        f.File,
			Location:    locationHint(f.Path),
			Rule:        f.Name,
			Family:      f.Family,
			Severity:    f.Severity,
			Description: f.Description,
			MatchedText: clip(f.Match, triageMaxMatch),
			Layer:       f.Layer,
		},
	}
	if f.Layer != "" {
		st.Finding.Layer = f.Layer + " (the rule matched inside a decoded payload; the excerpt shows the encoded wrapper)"
	}
	path := f.Path
	if path == "" {
		path = f.File
	}
	data, err := os.ReadFile(path)
	if err != nil {
		st.Source.Note = "source not readable: " + err.Error()
		return st
	}
	st.Source.FileBytes = int64(len(data))
	if !utf8.Valid(data) {
		data = []byte(strings.ToValidUTF8(string(data), "�"))
	}
	lines := strings.Split(string(data), "\n")
	st.Source.FileLines = len(lines)
	if contextLines <= 0 {
		contextLines = 25
	}
	from, to := 1, len(lines)
	if f.Line > 0 {
		from = f.Line - contextLines
		if from < 1 {
			from = 1
		}
		to = f.Line + contextLines
		if to > len(lines) {
			to = len(lines)
		}
	} else if to > 2*contextLines+1 {
		// Hash hits and decoded-layer matches have no line: show the head.
		to = 2*contextLines + 1
	}
	var b strings.Builder
	for i := from; i <= to; i++ {
		line := lines[i-1]
		if len(line) > 400 {
			line = line[:400] + " …"
		}
		fmt.Fprintf(&b, "%d: %s\n", i, line)
		if b.Len() > triageMaxExcerpt {
			st.Source.Truncated = true
			to = i
			break
		}
	}
	st.Source.Excerpt = b.String()
	st.Source.FromLine, st.Source.ToLine = from, to
	return st
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + " …"
}

// TriageFindings asks Jev about every finding concurrently and returns them
// sorted by true-positive probability, highest first. A failed call leaves
// Triage.Error set and sorts last, so one API hiccup never hides a finding.
func TriageFindings(ctx context.Context, client *typesafe.Client, findings []Finding, opts TriageOptions) []TriagedFinding {
	out := make([]TriagedFinding, len(findings))
	for i, f := range findings {
		out[i] = TriagedFinding{Finding: f}
	}
	if len(findings) == 0 || client == nil {
		return out
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = 8
	}
	if workers > len(findings) {
		workers = len(findings)
	}
	model := opts.Model
	if model == "" {
		model = client.Model
	}
	questions := triageQuestions()
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i].Triage = triageOne(ctx, client, model, questions, findings[i], opts.ContextLines)
			}
		}()
	}
	for i := range findings {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	sort.SliceStable(out, func(a, b int) bool {
		ta, tb := out[a].Triage, out[b].Triage
		if (ta == nil || ta.Error != "") != (tb == nil || tb.Error != "") {
			return tb == nil || tb.Error != ""
		}
		if ta == nil || tb == nil {
			return false
		}
		if ta.TruePositive != tb.TruePositive {
			return ta.TruePositive > tb.TruePositive
		}
		return SeverityRank(out[a].Severity) > SeverityRank(out[b].Severity)
	})
	return out
}

func triageOne(ctx context.Context, client *typesafe.Client, model string, questions typesafe.Questions, f Finding, contextLines int) *Triage {
	state := buildTriageState(f, contextLines)
	resp, err := client.SystemOneWithModel(ctx, model, state, questions)
	if err != nil {
		return &Triage{Error: err.Error()}
	}
	t := &Triage{Model: resp.Model, InputTokens: resp.Usage.InputTokens}
	if a, ok := resp.Answers["true_positive"]; ok {
		t.TruePositive = a.Yes()
	}
	if a, ok := resp.Answers["family"]; ok {
		t.Family, t.FamilyConfidence = a.Choice, a.Conf()
	}
	if a, ok := resp.Answers["action"]; ok {
		t.Action, t.ActionConfidence = a.Choice, a.Conf()
	}
	return t
}
