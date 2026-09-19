package scan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// phpExtensions are the file types the decoding layer applies to.
var phpExtensions = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range Extensions([]string{"php"}) {
		m[e] = true
	}
	return m
}()

// DefaultMaxBytes is how much of a file is read and matched. Larger files are
// matched on their first DefaultMaxBytes only.
const DefaultMaxBytes = 8 * 1024 * 1024

// MatchContext is how many bytes of matched text a finding keeps.
const MatchContext = 200

// Finding is one rule or hash hit on one file.
type Finding struct {
	File        string `json:"file"`           // relative to the scan root when one is known
	Path        string `json:"path"`           // absolute path
	Line        int    `json:"line,omitempty"` // 1-based line of the match (0 for hash hits)
	RuleID      string `json:"rule_id"`
	Name        string `json:"name"`
	Family      string `json:"family,omitempty"`
	Severity    string `json:"severity"`
	Description string `json:"description,omitempty"`
	Match       string `json:"match,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Source      string `json:"source,omitempty"`
	Layer       string `json:"layer,omitempty"` // set when the match was made on a decoded payload, e.g. "base64+gzinflate"
}

// LegacyFinding is the shape the Wordfence CSV path produced and the Manager's
// malware-alert ingest still consumes. Keep it stable.
type LegacyFinding struct {
	Filename             string `json:"filename"`
	SignatureID          string `json:"signature_id"`
	SignatureName        string `json:"signature_name"`
	SignatureDescription string `json:"signature_description"`
	MatchedText          string `json:"matched_text"`
}

// Legacy converts a Finding to the malware-alert payload shape.
func (f Finding) Legacy() LegacyFinding {
	desc := f.Description
	if f.Layer != "" {
		desc += " (matched inside a " + f.Layer + " encoded payload)"
	}
	return LegacyFinding{
		Filename:             f.File,
		SignatureID:          f.RuleID,
		SignatureName:        f.Name,
		SignatureDescription: desc,
		MatchedText:          f.Match,
	}
}

// Options tunes a Scanner.
type Options struct {
	Workers  int   // goroutines for file scanning; default runtime.NumCPU()
	MaxBytes int64 // bytes read per file; default DefaultMaxBytes
	// KnownGood, when set, is asked with a file's sha256 before matching; a
	// true answer skips the file (core checksums, wp.org release hashes).
	KnownGood func(sha256 string) bool
	// ExcludePaths are substrings of the relative path that are never scanned.
	// Always includes "/.git/".
	ExcludePaths []string
	// NoDecode disables the decoding layer (base64, deflate, rot13, escapes).
	NoDecode bool
}

// Scanner holds compiled rules. Safe for concurrent use.
type Scanner struct {
	rules      []compiledRule
	hashes     map[string]HashIOC
	allow      map[string]bool
	extensions map[string]bool // union of every rule's extensions
	opts       Options
	Errors     []error // rules that failed to compile
}

// New compiles a rule set. Rules that fail to compile are dropped and listed
// in Scanner.Errors; the scanner is still usable.
func New(rs *RuleSet, opts Options) *Scanner {
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	opts.ExcludePaths = append(opts.ExcludePaths, "/.git/")
	s := &Scanner{
		hashes:     map[string]HashIOC{},
		allow:      map[string]bool{},
		extensions: map[string]bool{},
		opts:       opts,
	}
	s.rules, s.Errors = compileRules(rs.Rules)
	for _, r := range s.rules {
		for e := range r.extensions {
			s.extensions[e] = true
		}
	}
	for _, h := range rs.Hashes {
		s.hashes[strings.ToLower(h.SHA256)] = h
	}
	for _, h := range rs.AllowHashes {
		s.allow[strings.ToLower(h)] = true
	}
	return s
}

// RuleCount reports how many rules compiled.
func (s *Scanner) RuleCount() int { return len(s.rules) }

// HashCount reports how many hash indicators are loaded.
func (s *Scanner) HashCount() int { return len(s.hashes) }

// Scannable reports whether a path's extension is covered by any rule. Hash
// indicators apply to every extension, so a scan still hashes such files.
func (s *Scanner) Scannable(path string) bool {
	return s.extensions[ExtOf(path)]
}

func (s *Scanner) excluded(rel string) bool {
	probe := "/" + rel
	for _, p := range s.opts.ExcludePaths {
		if strings.Contains(probe, p) {
			return true
		}
	}
	return false
}

// ScanFile matches one file. rel is the slash-separated path used for rule
// include/exclude checks and the Finding.File field; pass the basename when
// there is no root.
func (s *Scanner) ScanFile(path, rel string) ([]Finding, error) {
	rel = filepath.ToSlash(rel)
	if s.excluded(rel) {
		return nil, nil
	}
	// The logical name decides the file type, so a corpus sample stored by
	// hash can be scanned as "x.php" by passing that as rel.
	ext := ExtOf(rel)
	wantRules := s.extensions[ext]
	wantHash := len(s.hashes) > 0 || len(s.allow) > 0 || s.opts.KnownGood != nil
	if !wantRules && !wantHash {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.opts.MaxBytes))
	if err != nil {
		return nil, err
	}
	// Anything past MaxBytes is matched in overlapping windows below, so a
	// payload appended to a large file is still seen.
	var tail []byte
	if int64(len(data)) == s.opts.MaxBytes {
		tail, _ = io.ReadAll(io.LimitReader(f, 64*s.opts.MaxBytes))
	}

	var sum string
	hashOf := func() string {
		if sum == "" {
			h := sha256.New()
			h.Write(data)
			h.Write(tail)
			io.Copy(h, f) // beyond the windowed region, for an exact hash
			sum = hex.EncodeToString(h.Sum(nil))
		}
		return sum
	}

	if wantHash {
		h := hashOf()
		if s.allow[h] || (s.opts.KnownGood != nil && s.opts.KnownGood(h)) {
			return nil, nil
		}
		if ioc, ok := s.hashes[h]; ok {
			return []Finding{{
				File: rel, Path: path, RuleID: "hash:" + h[:12], Name: ioc.Name, Family: ioc.Family,
				Severity: ioc.Severity, Description: ioc.Description, SHA256: h, Source: ioc.Source,
			}}, nil
		}
	}
	if !wantRules {
		return nil, nil
	}

	findings := s.matchRules(data, ext, rel, path, hashOf, "")
	if len(tail) > 0 {
		hit := map[string]bool{}
		for _, f := range findings {
			hit[f.RuleID] = true
		}
		window := int(s.opts.MaxBytes)
		overlap := min(64*1024, window/4)
		// Each window starts overlap bytes before the previous one ended so
		// a match straddling a boundary is not missed.
		whole := append(data[len(data)-min(len(data), overlap):], tail...)
		for start := 0; start < len(whole); start += window - overlap {
			end := min(start+window, len(whole))
			for _, f := range s.matchRules(whole[start:end], ext, rel, path, hashOf, "") {
				if !hit[f.RuleID] {
					hit[f.RuleID] = true
					f.Line = 0 // line numbers past the first window are not tracked
					findings = append(findings, f)
				}
			}
			if end == len(whole) {
				break
			}
		}
	}
	// Only PHP is decoded: minified JavaScript carries long base64 data URIs
	// and decoder-like names, and pays for the search without ever hiding PHP.
	if !s.opts.NoDecode && phpExtensions[ext] {
		hit := map[string]bool{}
		for _, f := range findings {
			hit[f.RuleID] = true
		}
		for _, layer := range decodeLayers(data) {
			for _, f := range s.matchRules(layer.Data, ext, rel, path, hashOf, layer.Layer) {
				if hit[f.RuleID] {
					continue // the same rule already fired on the raw file
				}
				hit[f.RuleID] = true
				findings = append(findings, f)
			}
		}
	}
	return findings, nil
}

// matchRules runs every applicable rule over data. layer is "" for the raw
// file or the decoding chain that produced data.
func (s *Scanner) matchRules(data []byte, ext, rel, path string, hashOf func() string, layer string) []Finding {
	var findings []Finding
	for i := range s.rules {
		r := &s.rules[i]
		if !r.extensions[ext] || !r.pathAllowed(rel) {
			continue
		}
		if len(r.magic) > 0 && layer != "" {
			continue // magic-byte rules describe the file on disk, not a payload
		}
		if loc, ok := r.match(data); ok {
			f := Finding{
				File: rel, Path: path, RuleID: r.Rule.ID, Name: r.Rule.Name, Family: r.Rule.Family,
				Severity: r.Rule.Severity, Description: r.Rule.Description, Match: excerpt(data, loc),
				SHA256: hashOf(), Source: r.Rule.Source, Layer: layer,
			}
			if layer == "" {
				f.Line = lineOf(data, loc[0])
			}
			findings = append(findings, f)
		}
	}
	return findings
}

// match returns the location of the pattern that satisfied the rule.
func (r *compiledRule) match(data []byte) ([]int, bool) {
	if len(r.magic) > 0 {
		ok := false
		for _, m := range r.magic {
			if bytes.HasPrefix(data, m) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, false
		}
	}
	for _, lit := range r.prefilter {
		if !bytes.Contains(data, lit) {
			return nil, false
		}
	}
	var loc []int
	for _, re := range r.require {
		l := re.FindIndex(data)
		if l == nil {
			return nil, false
		}
		if loc == nil {
			loc = l
		}
	}
	if len(r.patterns) == 0 {
		return loc, true
	}
	need := r.Rule.MinMatches
	if need < 1 {
		need = 1
	}
	matched := 0
	var first []int
	for i, re := range r.patterns {
		var l []int
		if lp := r.literals[i]; lp != nil {
			l = lp.find(data)
		} else {
			l = re.FindIndex(data)
		}
		if l != nil {
			matched++
			if first == nil {
				first = l
			}
			if matched >= need {
				return first, true
			}
		}
		// Not enough patterns left to reach need: stop early.
		if matched+(len(r.patterns)-i-1) < need {
			return nil, false
		}
	}
	return nil, false
}

func lineOf(data []byte, off int) int {
	if off > len(data) {
		off = len(data)
	}
	return bytes.Count(data[:off], []byte{'\n'}) + 1
}

func excerpt(data []byte, loc []int) string {
	start, end := loc[0], loc[1]
	if end-start > MatchContext {
		end = start + MatchContext
	}
	// Extend a short match to the rest of its line for readability.
	if end-start < 40 {
		if nl := bytes.IndexByte(data[end:], '\n'); nl >= 0 && nl < MatchContext {
			end += nl
		} else if nl < 0 && len(data)-end < MatchContext {
			end = len(data)
		}
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(data[start:end]), "?"))
}

// Result is the outcome of ScanPaths or ScanDir.
type Result struct {
	Findings []Finding
	Scanned  int      // files opened and matched
	Skipped  int      // files not covered by any rule or excluded
	Errors   []string // unreadable files
}

// ScanPaths scans explicit files. root, when non-empty, makes Finding.File
// relative to it; otherwise File is the path as given.
func (s *Scanner) ScanPaths(paths []string, root string) Result {
	type job struct{ path, rel string }
	jobs := make(chan job)
	var mu sync.Mutex
	res := Result{}
	var wg sync.WaitGroup
	for i := 0; i < s.opts.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				f, err := s.ScanFile(j.path, j.rel)
				mu.Lock()
				if err != nil {
					res.Errors = append(res.Errors, j.path+": "+err.Error())
				} else {
					res.Scanned++
					res.Findings = append(res.Findings, f...)
				}
				mu.Unlock()
			}
		}()
	}
	for _, p := range paths {
		rel := p
		if root != "" {
			if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
				rel = r
			}
		}
		if s.excluded(filepath.ToSlash(rel)) || (!s.Scannable(p) && len(s.hashes) == 0) {
			mu.Lock()
			res.Skipped++
			mu.Unlock()
			continue
		}
		jobs <- job{p, rel}
	}
	close(jobs)
	wg.Wait()
	sortFindings(res.Findings)
	return res
}

// ScanDir walks a tree and scans every covered file.
func (s *Scanner) ScanDir(root string) Result {
	var paths []string
	res := Result{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if !errors.Is(err, fs.ErrPermission) {
				res.Errors = append(res.Errors, path+": "+err.Error())
			}
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	r := s.ScanPaths(paths, root)
	r.Errors = append(res.Errors, r.Errors...)
	return r
}

func sortFindings(f []Finding) {
	sort.Slice(f, func(i, j int) bool {
		if f[i].File != f[j].File {
			return f[i].File < f[j].File
		}
		if f[i].Line != f[j].Line {
			return f[i].Line < f[j].Line
		}
		return f[i].RuleID < f[j].RuleID
	})
}

// SeverityRank orders severities for filtering and display.
func SeverityRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}
