// Package scan is CaptainCore's native malware scanner: a rule file of literal
// prefilters, RE2 patterns and hash indicators, run over WordPress file trees.
// It replaces the Wordfence CLI dependency (discontinued 2026-09-14) with an
// engine and a rule set CaptainCore owns.
package scan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule is one detection. A file matches when every prefilter literal is
// present, every Require pattern matches, and at least one Patterns entry
// matches (when Patterns is non-empty). Paths are compared as substrings of
// the slash-separated relative path.
type Rule struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Family       string   `json:"family,omitempty"` // backdoor, obfuscation, spam, ioc, hacktool, ...
	Severity     string   `json:"severity"`         // critical, high, medium, low
	Description  string   `json:"description,omitempty"`
	Prefilter    []string `json:"prefilter,omitempty"`     // literal substrings, all required
	Patterns     []string `json:"patterns,omitempty"`      // RE2, any one must match
	Require      []string `json:"require,omitempty"`       // RE2, all must match
	IncludePaths []string `json:"include_paths,omitempty"` // when set, path must contain one
	ExcludePaths []string `json:"exclude_paths,omitempty"` // path must contain none
	FileTypes    []string `json:"file_types,omitempty"`    // groups (php, js, html, svg) or ".ext"; default php
	Source       string   `json:"source,omitempty"`        // attribution for imported rules
	License      string   `json:"license,omitempty"`
}

// HashIOC flags a file by exact sha256.
type HashIOC struct {
	SHA256      string `json:"sha256"`
	Name        string `json:"name"`
	Family      string `json:"family,omitempty"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

// RuleSet is the on-disk rule file (version 2). A version 1 file, a bare JSON
// array of rules, still loads.
type RuleSet struct {
	Version     int       `json:"version"`
	Updated     string    `json:"updated,omitempty"`
	Rules       []Rule    `json:"rules"`
	Hashes      []HashIOC `json:"hashes,omitempty"`
	AllowHashes []string  `json:"allow_hashes,omitempty"` // sha256 of files never to report
}

var fileTypeGroups = map[string][]string{
	"php":  {".php", ".phtml", ".phar", ".php5", ".php7", ".php8", ".inc"},
	"js":   {".js", ".mjs"},
	"html": {".html", ".htm"},
	"svg":  {".svg"},
	"ico":  {".ico"},
	"txt":  {".txt"},
}

// Extensions expands file type groups and literal extensions to lower-case extensions.
func Extensions(types []string) []string {
	if len(types) == 0 {
		types = []string{"php"}
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range types {
		t = strings.ToLower(strings.TrimSpace(t))
		if exts, ok := fileTypeGroups[t]; ok {
			for _, e := range exts {
				if !seen[e] {
					seen[e] = true
					out = append(out, e)
				}
			}
			continue
		}
		if !strings.HasPrefix(t, ".") {
			t = "." + t
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// ParseRuleSet decodes a version 2 rule file or a version 1 bare array.
func ParseRuleSet(data []byte) (*RuleSet, error) {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var rules []Rule
		if err := json.Unmarshal(data, &rules); err != nil {
			return nil, fmt.Errorf("parse v1 rules: %w", err)
		}
		return &RuleSet{Version: 1, Rules: rules}, nil
	}
	var rs RuleSet
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	if rs.Version == 0 {
		rs.Version = 2
	}
	return &rs, nil
}

// LoadRuleSet reads one rule file.
func LoadRuleSet(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules %s: %w", path, err)
	}
	rs, err := ParseRuleSet(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rs, nil
}

// Merge appends another set's rules, hashes and allow list.
func (rs *RuleSet) Merge(other *RuleSet) {
	rs.Rules = append(rs.Rules, other.Rules...)
	rs.Hashes = append(rs.Hashes, other.Hashes...)
	rs.AllowHashes = append(rs.AllowHashes, other.AllowHashes...)
}

// DefaultRulePaths returns the shipped rule file plus any drop-in files under
// lib/malware-signatures.d/ (kept separate so third-party rule sets carry
// their own license). Missing files are simply omitted.
func DefaultRulePaths() []string {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".captaincore", "lib")
	paths := []string{filepath.Join(base, "malware-signatures.json")}
	extra, _ := filepath.Glob(filepath.Join(base, "malware-signatures.d", "*.json"))
	sort.Strings(extra)
	return append(paths, extra...)
}

// LoadDefaultRuleSet loads and merges DefaultRulePaths. The primary file must
// exist; drop-ins that fail to parse are reported as errors, not skipped.
func LoadDefaultRuleSet() (*RuleSet, error) {
	paths := DefaultRulePaths()
	rs, err := LoadRuleSet(paths[0])
	if err != nil {
		return nil, err
	}
	for _, p := range paths[1:] {
		extra, err := LoadRuleSet(p)
		if err != nil {
			return nil, err
		}
		rs.Merge(extra)
	}
	return rs, nil
}

// compiledRule is a Rule with its regexes compiled and path/extension sets prepared.
type compiledRule struct {
	Rule       Rule
	prefilter  [][]byte
	patterns   []*regexp.Regexp
	require    []*regexp.Regexp
	extensions map[string]bool
}

// Compile validates and compiles every rule. Invalid rules are returned as
// errors keyed by rule id; valid ones are still usable.
func compileRules(rules []Rule) ([]compiledRule, []error) {
	var out []compiledRule
	var errs []error
	for _, r := range rules {
		if r.ID == "" {
			errs = append(errs, fmt.Errorf("rule with empty id (%q)", r.Name))
			continue
		}
		if len(r.Patterns) == 0 && len(r.Require) == 0 {
			errs = append(errs, fmt.Errorf("rule %s: no patterns", r.ID))
			continue
		}
		c := compiledRule{Rule: r, extensions: map[string]bool{}}
		for _, p := range r.Prefilter {
			c.prefilter = append(c.prefilter, []byte(p))
		}
		bad := false
		for _, p := range r.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %s: pattern %q: %w", r.ID, p, err))
				bad = true
				break
			}
			c.patterns = append(c.patterns, re)
		}
		if bad {
			continue
		}
		for _, p := range r.Require {
			re, err := regexp.Compile(p)
			if err != nil {
				errs = append(errs, fmt.Errorf("rule %s: require %q: %w", r.ID, p, err))
				bad = true
				break
			}
			c.require = append(c.require, re)
		}
		if bad {
			continue
		}
		for _, e := range Extensions(r.FileTypes) {
			c.extensions[e] = true
		}
		out = append(out, c)
	}
	return out, errs
}

func (c *compiledRule) pathAllowed(rel string) bool {
	if len(c.Rule.IncludePaths) > 0 {
		ok := false
		for _, p := range c.Rule.IncludePaths {
			if strings.Contains(rel, p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, p := range c.Rule.ExcludePaths {
		if strings.Contains(rel, p) {
			return false
		}
	}
	return true
}
