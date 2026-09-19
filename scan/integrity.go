package scan

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The integrity layer compares a site's plugins and themes against the files
// wordpress.org shipped for that exact version. Two things come out of it:
//
//   - a known-good set: any file whose sha256 matches a release file skips
//     signature matching entirely, so rules can be aggressive inside vendor
//     code without false positives;
//   - integrity findings: a PHP file inside a wp.org component that is not in
//     the release ("unknown"), or whose contents differ from it ("modified").
//     An injected line in a legitimate plugin file is caught this way whether
//     or not any signature knows the payload.
//
// Manifests come from downloads.wordpress.org: the plugin-checksums JSON for
// plugins and the release zip (hashed on download) for themes. Both are
// immutable per version and cached on disk forever; a version that is not on
// wordpress.org (a commercial plugin sharing a slug shape, or a release too old
// for checksums) is remembered as missing for MissingTTL so it is not asked
// for every night.

// Manifest is the file list of one wordpress.org release: relative path inside
// the component directory to the lowercase hex sha256 values wordpress.org
// accepts for it. A file usually has one; a few (readme.txt most often) have
// two when the zip and the tagged source disagree.
type Manifest struct {
	Type    string              `json:"type"` // plugin or theme
	Slug    string              `json:"slug"`
	Version string              `json:"version"`
	Files   map[string][]string `json:"files"`
}

// Accepts reports whether h is one of the release hashes for rel.
func (m *Manifest) Accepts(rel, h string) bool {
	for _, w := range m.Files[rel] {
		if w == h {
			return true
		}
	}
	return false
}

// Component is a plugin or theme directory found in a tree.
type Component struct {
	Type    string // plugin or theme
	Slug    string // directory name
	Version string // from the plugin header or style.css
	Dir     string // relative to the tree root, forward slashes
}

// ManifestStore fetches and caches release manifests.
type ManifestStore struct {
	Dir        string        // cache directory
	Client     *http.Client  // nil uses a 30 s client
	BaseURL    string        // "" means https://downloads.wordpress.org
	MaxFetches int           // network fetches allowed per store lifetime; 0 = 300
	MissingTTL time.Duration // how long a 404 is remembered; 0 = 30 days
	// Offline disables network fetches; only cached manifests are used.
	Offline bool

	mu      sync.Mutex
	fetches int
	mem     map[string]*Manifest // key type/slug/version, nil = known missing
}

// NewManifestStore returns a store caching under dir.
func NewManifestStore(dir string) *ManifestStore {
	return &ManifestStore{Dir: dir, mem: map[string]*Manifest{}}
}

// ErrNotOnWordPressOrg reports that wordpress.org has no such release.
var ErrNotOnWordPressOrg = errors.New("not a wordpress.org release")

// ErrFetchBudget reports that the store's network budget is spent.
var ErrFetchBudget = errors.New("manifest fetch budget exhausted")

func (m *ManifestStore) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (m *ManifestStore) baseURL() string {
	if m.BaseURL != "" {
		return strings.TrimRight(m.BaseURL, "/")
	}
	return "https://downloads.wordpress.org"
}

func (m *ManifestStore) missingTTL() time.Duration {
	if m.MissingTTL > 0 {
		return m.MissingTTL
	}
	return 30 * 24 * time.Hour
}

func (m *ManifestStore) maxFetches() int {
	if m.MaxFetches > 0 {
		return m.MaxFetches
	}
	return 300
}

var safeSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var safeVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Get returns the manifest for a component, from memory, disk or the network,
// in that order. ErrNotOnWordPressOrg is returned (and cached) when
// wordpress.org has no such release.
func (m *ManifestStore) Get(typ, slug, version string) (*Manifest, error) {
	slug = strings.ToLower(slug)
	if !safeSlug.MatchString(slug) || !safeVersion.MatchString(version) {
		return nil, ErrNotOnWordPressOrg
	}
	if typ != "plugin" && typ != "theme" {
		return nil, fmt.Errorf("unknown component type %q", typ)
	}
	key := typ + "/" + slug + "/" + version
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mem == nil {
		m.mem = map[string]*Manifest{}
	}
	if man, ok := m.mem[key]; ok {
		if man == nil {
			return nil, ErrNotOnWordPressOrg
		}
		return man, nil
	}
	if man, ok := m.readCache(typ, slug, version); ok {
		m.mem[key] = man
		if man == nil {
			return nil, ErrNotOnWordPressOrg
		}
		return man, nil
	}
	if m.Offline {
		return nil, ErrNotOnWordPressOrg
	}
	if m.fetches >= m.maxFetches() {
		return nil, ErrFetchBudget
	}
	m.fetches++
	var man *Manifest
	var err error
	if typ == "plugin" {
		man, err = m.fetchPlugin(slug, version)
	} else {
		man, err = m.fetchTheme(slug, version)
	}
	if errors.Is(err, ErrNotOnWordPressOrg) {
		m.mem[key] = nil
		m.writeMissing(typ, slug, version)
		return nil, err
	}
	if err != nil {
		return nil, err // transient: not cached, asked again next time
	}
	m.mem[key] = man
	m.writeCache(man)
	return man, nil
}

// Fetches reports how many network requests the store has made.
func (m *ManifestStore) Fetches() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetches
}

func (m *ManifestStore) cachePath(typ, slug, version string) string {
	return filepath.Join(m.Dir, typ+"s", slug, version+".json")
}

// readCache returns (manifest, true) for a cached release, (nil, true) for a
// release recorded as missing within MissingTTL, and (nil, false) otherwise.
func (m *ManifestStore) readCache(typ, slug, version string) (*Manifest, bool) {
	if m.Dir == "" {
		return nil, false
	}
	p := m.cachePath(typ, slug, version)
	if b, err := os.ReadFile(p); err == nil {
		var man Manifest
		if json.Unmarshal(b, &man) == nil && len(man.Files) > 0 {
			man.Type, man.Slug, man.Version = typ, slug, version
			return &man, true
		}
	}
	if b, err := os.ReadFile(p + ".missing"); err == nil {
		if ts, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			if time.Since(time.Unix(ts, 0)) < m.missingTTL() {
				return nil, true
			}
		}
	}
	return nil, false
}

func (m *ManifestStore) writeCache(man *Manifest) {
	if m.Dir == "" {
		return
	}
	p := m.cachePath(man.Type, man.Slug, man.Version)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(map[string]any{"files": man.Files})
	if err != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, p)
	}
	os.Remove(p + ".missing")
}

func (m *ManifestStore) writeMissing(typ, slug, version string) {
	if m.Dir == "" {
		return
	}
	p := m.cachePath(typ, slug, version) + ".missing"
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	os.WriteFile(p, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
}

func (m *ManifestStore) get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CaptainCore integrity check (+https://captaincore.io)")
	resp, err := m.client().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, ErrNotOnWordPressOrg
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

// fetchPlugin reads downloads.wordpress.org/plugin-checksums/<slug>/<version>.json.
func (m *ManifestStore) fetchPlugin(slug, version string) (*Manifest, error) {
	resp, err := m.get(fmt.Sprintf("%s/plugin-checksums/%s/%s.json", m.baseURL(), slug, version))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var doc struct {
		Files map[string]struct {
			SHA256 json.RawMessage `json:"sha256"` // a string, or an array of strings
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("plugin-checksums %s %s: %w", slug, version, err)
	}
	if len(doc.Files) == 0 {
		return nil, ErrNotOnWordPressOrg
	}
	man := &Manifest{Type: "plugin", Slug: slug, Version: version, Files: make(map[string][]string, len(doc.Files))}
	for rel, f := range doc.Files {
		var one string
		var many []string
		if json.Unmarshal(f.SHA256, &one) == nil && one != "" {
			many = []string{one}
		} else if json.Unmarshal(f.SHA256, &many) != nil {
			continue
		}
		var hs []string
		for _, h := range many {
			if len(h) == 64 {
				hs = append(hs, strings.ToLower(h))
			}
		}
		if len(hs) > 0 {
			man.Files[path.Clean(rel)] = hs
		}
	}
	return man, nil
}

// fetchTheme downloads downloads.wordpress.org/theme/<slug>.<version>.zip and
// hashes its entries.
func (m *ManifestStore) fetchTheme(slug, version string) (*Manifest, error) {
	resp, err := m.get(fmt.Sprintf("%s/theme/%s.%s.zip", m.baseURL(), slug, version))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("theme zip %s %s: %w", slug, version, err)
	}
	man := &Manifest{Type: "theme", Slug: slug, Version: version, Files: map[string][]string{}}
	for _, e := range zr.File {
		if e.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(e.Name)
		// The archive holds a single top-level directory named after the slug.
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		} else {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			continue
		}
		h := sha256.New()
		io.Copy(h, rc)
		rc.Close()
		man.Files[name] = []string{hex.EncodeToString(h.Sum(nil))}
	}
	if len(man.Files) == 0 {
		return nil, ErrNotOnWordPressOrg
	}
	return man, nil
}

var headerVersion = regexp.MustCompile(`(?mi)^[ \t/*#@]*Version:[ \t]*([^\s*]+)`)

// headerField reads a "Key: value" line from a file header.
func headerField(b []byte, key string) string {
	head := b[:min(len(b), 8192)]
	i := bytes.Index(head, []byte(key))
	if i < 0 {
		return ""
	}
	rest := head[i+len(key):]
	if nl := bytes.IndexAny(rest, "\r\n"); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(string(rest)), "*/"))
}

// pluginVersion finds the top-level file carrying a Plugin Name: header and
// returns its Version:.
func pluginVersion(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || ExtOf(e.Name()) != ".php" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		head := b[:min(len(b), 8192)]
		if !bytes.Contains(head, []byte("Plugin Name:")) {
			continue
		}
		if m := headerVersion.FindSubmatch(head); m != nil {
			return string(m[1])
		}
		return ""
	}
	return ""
}

// themeVersion returns the Version: from style.css.
func themeVersion(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "style.css"))
	if err != nil {
		return ""
	}
	head := b[:min(len(b), 8192)]
	if !bytes.Contains(head, []byte("Theme Name:")) {
		return ""
	}
	if m := headerVersion.FindSubmatch(head); m != nil {
		return string(m[1])
	}
	return ""
}

// FindComponents lists the plugin and theme directories under root that carry
// a version header. root is a content directory (plugins/, themes/ inside it).
func FindComponents(root string) []Component {
	var out []Component
	for _, kind := range []struct{ dir, typ string }{{"plugins", "plugin"}, {"themes", "theme"}} {
		entries, err := os.ReadDir(filepath.Join(root, kind.dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, kind.dir, e.Name())
			var v string
			if kind.typ == "plugin" {
				v = pluginVersion(dir)
			} else {
				v = themeVersion(dir)
			}
			if v == "" {
				continue
			}
			out = append(out, Component{Type: kind.typ, Slug: e.Name(), Version: v, Dir: kind.dir + "/" + e.Name()})
		}
	}
	return out
}

// ComponentOf maps a relative path to the plugins/<slug> or themes/<slug>
// directory that contains it, or "" when it is outside both.
func ComponentOf(rel string) (typ, dir string) {
	rel = filepath.ToSlash(rel)
	for _, kind := range []struct{ prefix, typ string }{{"plugins/", "plugin"}, {"themes/", "theme"}} {
		i := strings.Index(rel, kind.prefix)
		if i < 0 || (i > 0 && rel[i-1] != '/') {
			continue
		}
		rest := rel[i+len(kind.prefix):]
		j := strings.Index(rest, "/")
		if j <= 0 {
			continue
		}
		return kind.typ, rel[:i+len(kind.prefix)+j]
	}
	return "", ""
}

// IntegrityResult is the outcome of checking a tree against release manifests.
type IntegrityResult struct {
	Findings []Finding
	// KnownGood holds the sha256 of every release file of every covered
	// component, for Options.KnownGood.
	KnownGood map[string]bool
	// Components seen, and how many had a wordpress.org manifest.
	Components, Covered int
	// Components whose files differ so much from the release that they are a
	// different build (a commercial plugin under a wordpress.org slug, a fork):
	// reported once at low severity, not file by file.
	Mismatched int
	// Components whose unknown files are too many to be an injection (a
	// runtime bundle or template cache), reported once at medium.
	Generated int
	Modified  int
	Unknown   int
	Errors    []string
}

// mismatchFraction: above this share of release files modified, the directory
// holds a different build, not an injection.
const mismatchFraction = 0.2

// mismatchMinFiles: below this many modified files a component is still
// judged file by file whatever the fraction.
const mismatchMinFiles = 10

// integritySeverity says which unknown or modified files are worth a finding
// and at what severity. A PHP file that the release never shipped is high: on
// clean sites it does not happen outside runtime-generated caches, which the
// cluster rule below folds away. A PHP file that differs from the release is
// only medium on its own: hosts patch abandoned plugins for PHP 8, developers
// leave a debug line, vendors add a header. Escalate raises it to high when a
// signature also fires on the same file.
func integritySeverity(rel string) (unknown, modified string) {
	switch ExtOf(rel) {
	case ".php", ".phtml", ".php5", ".php7", ".inc", ".phar":
		return "high", "medium"
	case ".htaccess":
		return "high", "medium"
	case ".js", ".html", ".htm", ".svg":
		return "low", "low"
	}
	return "", ""
}

// clusterNote folds a component's findings of one kind into a single note
// when there are too many to be an injection: a plugin that unpacks a bundle
// or writes a template cache into its own directory (unknown files), or a
// commercial build under a wordpress.org slug (modified files).
func clusterNote(c Component, dir string, man *Manifest, kind string, n int) Finding {
	if kind == "integrity-modified-file" {
		return Finding{
			File: c.Dir, Path: dir, RuleID: "integrity-different-build",
			Name:   fmt.Sprintf("%s %s %s is not the wordpress.org build", c.Type, c.Slug, c.Version),
			Family: "integrity", Severity: "low", Source: "wordpress.org",
			Description: "Most files differ from the wordpress.org release of the same version: a commercial or forked build under a wordpress.org slug, or a nulled copy.",
			Match:       strconv.Itoa(n) + " of " + strconv.Itoa(len(man.Files)) + " release files differ",
		}
	}
	return Finding{
		File: c.Dir, Path: dir, RuleID: "integrity-generated-files",
		Name:   fmt.Sprintf("%s %s %s carries %d PHP files the release does not", c.Type, c.Slug, c.Version, n),
		Family: "integrity", Severity: "medium", Source: "wordpress.org",
		Description: "Many PHP files inside the directory are not in the wordpress.org release: a bundle the plugin downloads at runtime or a template cache it writes into itself. Each file was still scanned for signatures.",
		Match:       strconv.Itoa(n) + " files not among the " + strconv.Itoa(len(man.Files)) + " of " + c.Slug + " " + c.Version,
	}
}

// vendorExtraDirs are directories a vendor adds to its own build of a
// wordpress.org plugin without publishing them to wordpress.org: WP Engine's
// Delicious Brains plugins carry an ext/ updater that pulls releases from
// wpengine.com since late 2024. Files under them are not unknown files.
var vendorExtraDirs = []string{
	"better-search-replace/ext/",
	"wp-migrate-db/ext/",
	"advanced-custom-fields/ext/",
	"wp-offload-media-lite/ext/",
}

func vendorExtra(componentDir, rel string) bool {
	full := componentDir + "/" + rel
	for _, d := range vendorExtraDirs {
		if strings.Contains(full, d) {
			return true
		}
	}
	return false
}

// clusterAbsolute: this many files of one kind in one component is a cluster
// whatever the release size. wp-phpmyadmin-extension ships thousands of files
// and still writes 58 more into its own template cache.
const clusterAbsolute = 25

// isCluster: at least mismatchMinFiles and at least mismatchFraction of the
// release's file count, or clusterAbsolute outright.
func isCluster(n, releaseFiles int) bool {
	return n >= clusterAbsolute || (n >= mismatchMinFiles && float64(n) >= mismatchFraction*float64(releaseFiles))
}

// CheckTree hashes every file of every component under root and compares it
// with the wordpress.org release. Components without a manifest contribute
// nothing. root is the content directory.
func (m *ManifestStore) CheckTree(root string, comps []Component) IntegrityResult {
	res := IntegrityResult{KnownGood: map[string]bool{}, Components: len(comps)}
	for _, c := range comps {
		man, err := m.Get(c.Type, c.Slug, c.Version)
		if err != nil {
			if !errors.Is(err, ErrNotOnWordPressOrg) {
				res.Errors = append(res.Errors, c.Dir+"@"+c.Version+": "+err.Error())
			}
			continue
		}
		res.Covered++
		for _, hs := range man.Files {
			for _, h := range hs {
				res.KnownGood[h] = true
			}
		}
		var found []Finding
		modified := 0
		dir := filepath.Join(root, filepath.FromSlash(c.Dir))
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				if d != nil && d.IsDir() && d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			rel = filepath.ToSlash(rel)
			want, inRelease := man.Files[rel]
			unknownSev, modifiedSev := integritySeverity(rel)
			if !inRelease {
				if unknownSev == "" || vendorExtra(c.Dir, rel) {
					return nil
				}
				h, err := fileSHA256(p)
				if err != nil {
					return nil
				}
				if res.KnownGood[h] {
					return nil // a release file under another name (a copy), not new code
				}
				found = append(found, Finding{
					File: c.Dir + "/" + rel, Path: p, RuleID: "integrity-unknown-file",
					Name:   fmt.Sprintf("File not in the %s release of %s %s", c.Type, c.Slug, c.Version),
					Family: "integrity", Severity: unknownSev, SHA256: h, Source: "wordpress.org",
					Description: "The file does not exist in the wordpress.org release the " + c.Type + " reports; code added to a vendor directory after install.",
					Match:       rel + " is not among the " + strconv.Itoa(len(man.Files)) + " files of " + c.Slug + " " + c.Version,
				})
				return nil
			}
			h, err := fileSHA256(p)
			if err != nil || man.Accepts(rel, h) {
				return nil
			}
			modified++
			if modifiedSev == "" {
				return nil
			}
			found = append(found, Finding{
				File: c.Dir + "/" + rel, Path: p, RuleID: "integrity-modified-file",
				Name:   fmt.Sprintf("File differs from the %s release of %s %s", c.Type, c.Slug, c.Version),
				Family: "integrity", Severity: modifiedSev, SHA256: h, Source: "wordpress.org",
				Description: "The file's contents differ from the wordpress.org release the " + c.Type + " reports; an edit or an injection into vendor code.",
				Match:       rel + " sha256 " + h[:12] + " differs from release " + want[0][:12],
			})
			return nil
		})
		if isCluster(modified, len(man.Files)) {
			res.Mismatched++
			res.Findings = append(res.Findings, clusterNote(c, dir, man, "integrity-modified-file", modified))
			continue
		}
		unknown := 0
		for _, f := range found {
			if f.RuleID == "integrity-unknown-file" {
				unknown++
			}
		}
		if isCluster(unknown, len(man.Files)) {
			res.Generated++
			res.Findings = append(res.Findings, clusterNote(c, dir, man, "integrity-unknown-file", unknown))
			kept := found[:0]
			for _, f := range found {
				if f.RuleID != "integrity-unknown-file" {
					kept = append(kept, f)
				}
			}
			found = kept
		}
		for _, f := range found {
			switch f.RuleID {
			case "integrity-unknown-file":
				res.Unknown++
			case "integrity-modified-file":
				res.Modified++
			}
		}
		res.Findings = append(res.Findings, found...)
	}
	sortFindings(res.Findings)
	return res
}

// CheckPaths checks only the given files (absolute paths under root) against
// the release manifests of the components that contain them. Used by the
// nightly incremental scan, where only changed files are known.
func (m *ManifestStore) CheckPaths(root string, paths []string) IntegrityResult {
	res := IntegrityResult{KnownGood: map[string]bool{}}
	byDir := map[string]*Component{}
	manifests := map[string]*Manifest{}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		rel = filepath.ToSlash(rel)
		typ, dir := ComponentOf(rel)
		if dir == "" {
			continue
		}
		c, seen := byDir[dir]
		if !seen {
			res.Components++
			abs := filepath.Join(root, filepath.FromSlash(dir))
			var v string
			if typ == "plugin" {
				v = pluginVersion(abs)
			} else {
				v = themeVersion(abs)
			}
			c = &Component{Type: typ, Slug: path.Base(dir), Version: v, Dir: dir}
			byDir[dir] = c
			if v != "" {
				man, err := m.Get(typ, c.Slug, v)
				if err == nil {
					manifests[dir] = man
					res.Covered++
					for _, hs := range man.Files {
						for _, h := range hs {
							res.KnownGood[h] = true
						}
					}
				} else if !errors.Is(err, ErrNotOnWordPressOrg) {
					res.Errors = append(res.Errors, dir+"@"+v+": "+err.Error())
				}
			}
		}
		man := manifests[dir]
		if man == nil {
			continue
		}
		inner := strings.TrimPrefix(rel, dir+"/")
		want, inRelease := man.Files[inner]
		unknownSev, modifiedSev := integritySeverity(inner)
		h, err := fileSHA256(p)
		if err != nil {
			continue
		}
		if !inRelease {
			if unknownSev == "" || res.KnownGood[h] || vendorExtra(dir, inner) {
				continue
			}
			res.Unknown++
			res.Findings = append(res.Findings, Finding{
				File: rel, Path: p, RuleID: "integrity-unknown-file",
				Name:   fmt.Sprintf("File not in the %s release of %s %s", c.Type, c.Slug, c.Version),
				Family: "integrity", Severity: unknownSev, SHA256: h, Source: "wordpress.org",
				Description: "The file does not exist in the wordpress.org release the " + c.Type + " reports; code added to a vendor directory after install.",
				Match:       inner + " is not among the " + strconv.Itoa(len(man.Files)) + " files of " + c.Slug + " " + c.Version,
			})
			continue
		}
		if man.Accepts(inner, h) || modifiedSev == "" {
			continue
		}
		res.Modified++
		res.Findings = append(res.Findings, Finding{
			File: rel, Path: p, RuleID: "integrity-modified-file",
			Name:   fmt.Sprintf("File differs from the %s release of %s %s", c.Type, c.Slug, c.Version),
			Family: "integrity", Severity: modifiedSev, SHA256: h, Source: "wordpress.org",
			Description: "The file's contents differ from the wordpress.org release the " + c.Type + " reports; an edit or an injection into vendor code.",
			Match:       inner + " sha256 " + h[:12] + " differs from release " + want[0][:12],
		})
	}
	// A changed set that rewrites most of a component is an update to a
	// different build, and one that adds a whole bundle is the plugin
	// unpacking itself: keep the one-line note instead of a finding per file.
	byComp := map[string]map[string][]int{}
	for i, f := range res.Findings {
		if f.RuleID == "integrity-modified-file" || f.RuleID == "integrity-unknown-file" {
			_, dir := ComponentOf(f.File)
			if byComp[dir] == nil {
				byComp[dir] = map[string][]int{}
			}
			byComp[dir][f.RuleID] = append(byComp[dir][f.RuleID], i)
		}
	}
	drop := map[int]bool{}
	for dir, kinds := range byComp {
		man := manifests[dir]
		if man == nil {
			continue
		}
		for kind, idx := range kinds {
			if !isCluster(len(idx), len(man.Files)) {
				continue
			}
			for _, i := range idx {
				drop[i] = true
			}
			if kind == "integrity-modified-file" {
				res.Modified -= len(idx)
				res.Mismatched++
			} else {
				res.Unknown -= len(idx)
				res.Generated++
			}
			res.Findings = append(res.Findings, clusterNote(*byDir[dir], filepath.Join(root, filepath.FromSlash(dir)), man, kind, len(idx)))
		}
	}
	if len(drop) > 0 {
		kept := res.Findings[:0]
		for i, f := range res.Findings {
			if !drop[i] {
				kept = append(kept, f)
			}
		}
		res.Findings = kept
	}
	sortFindings(res.Findings)
	return res
}

// Escalate raises an integrity-modified-file finding to high when any rule
// also fired on the same file: a vendor file that was edited and now carries
// something a signature recognises, at whatever severity, is no longer a
// routine patch. The rule's name is appended to the description.
func Escalate(findings []Finding) []Finding {
	// Only a medium or stronger rule escalates: the low-severity rules flag
	// deprecated constructs (create_function, an old file manager), and a
	// vendor file edited to remove one of those is a PHP 8 patch, not an
	// injection.
	ruleOn := map[string]string{}
	for _, f := range findings {
		if f.Family != "integrity" && ruleOn[f.File] == "" && SeverityRank(f.Severity) >= SeverityRank("medium") {
			ruleOn[f.File] = f.Name
		}
	}
	for i, f := range findings {
		if f.RuleID != "integrity-modified-file" || SeverityRank(f.Severity) >= SeverityRank("high") {
			continue
		}
		if name, ok := ruleOn[f.File]; ok {
			findings[i].Severity = "high"
			findings[i].Description += " A signature also matched this file: " + name + "."
		}
	}
	return findings
}

// KnownGoodFunc adapts a hash set to Options.KnownGood.
func KnownGoodFunc(set map[string]bool) func(string) bool {
	if len(set) == 0 {
		return nil
	}
	return func(h string) bool { return set[h] }
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Summary is a one-line description of an integrity result.
func (r IntegrityResult) Summary() string {
	parts := []string{fmt.Sprintf("%d of %d components matched to wordpress.org", r.Covered, r.Components)}
	if r.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", r.Modified))
	}
	if r.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", r.Unknown))
	}
	if r.Mismatched > 0 {
		parts = append(parts, fmt.Sprintf("%d different build", r.Mismatched))
	}
	if r.Generated > 0 {
		parts = append(parts, fmt.Sprintf("%d with generated files", r.Generated))
	}
	sort.Strings(parts[1:])
	return strings.Join(parts, ", ")
}
