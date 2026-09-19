package scan

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// fakeWordPressOrg serves plugin-checksums JSON and theme zips for a fixed
// catalogue and counts requests.
func fakeWordPressOrg(t *testing.T, plugins map[string]map[string]string, themes map[string]map[string]string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/plugin-checksums/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/plugin-checksums/"), ".json") // slug/version
		files, ok := plugins[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		doc := map[string]any{"plugin": strings.Split(key, "/")[0], "files": map[string]any{}}
		for rel, content := range files {
			if strings.Contains(content, "|") { // two accepted contents, as wordpress.org lists for readme.txt sometimes
				a, b, _ := strings.Cut(content, "|")
				doc["files"].(map[string]any)[rel] = map[string]any{"md5": []string{"x", "y"}, "sha256": []string{sha(a), sha(b)}}
				continue
			}
			doc["files"].(map[string]any)[rel] = map[string]string{"md5": "x", "sha256": sha(content)}
		}
		json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/theme/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/theme/"), ".zip") // slug.version
		i := strings.Index(name, ".")
		files, ok := themes[name[:i]+"/"+name[i+1:]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for rel, content := range files {
			f, _ := zw.Create(name[:i] + "/" + rel)
			f.Write([]byte(content))
		}
		zw.Close()
		w.Write(buf.Bytes())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestIntegrityCheckTree(t *testing.T) {
	akismet := map[string]string{
		"akismet.php":       "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\n*/\n",
		"class.akismet.php": "<?php class Akismet {}\n",
		"_inc/akismet.js":   "var a = 1;\n",
		"readme.txt":        "=== Akismet ===\n",
		"views/two.php":     "<?php // zip build\n|<?php // tagged build\n",
	}
	twenty := map[string]string{
		"style.css":     "/*\nTheme Name: Twenty Twenty-Four\nVersion: 1.2\n*/\n",
		"functions.php": "<?php add_action('init', 'tt4');\n",
		"index.php":     "<?php get_header();\n",
	}
	srv, hits := fakeWordPressOrg(t,
		map[string]map[string]string{"akismet/5.3": akismet, "better-search-replace/1.4.10": {"better-search-replace.php": "<?php\n/*\nPlugin Name: Better Search Replace\nVersion: 1.4.10\n*/\n"}},
		map[string]map[string]string{"twentytwentyfour/1.2": twenty})

	root := t.TempDir()
	write(t, root, "plugins/akismet/akismet.php", akismet["akismet.php"])
	write(t, root, "plugins/akismet/class.akismet.php", akismet["class.akismet.php"]+"eval($_POST['x']);\n") // modified
	write(t, root, "plugins/akismet/_inc/akismet.js", akismet["_inc/akismet.js"])
	write(t, root, "plugins/akismet/readme.txt", "changed readme\n")              // modified but not reportable
	write(t, root, "plugins/akismet/wp-cache.php", "<?php system($_GET['c']);\n") // unknown
	write(t, root, "plugins/akismet/_inc/copy.php", akismet["class.akismet.php"]) // a release file under another name
	write(t, root, "plugins/akismet/notes.md", "hi\n")                            // unknown, not reportable
	write(t, root, "plugins/better-search-replace/better-search-replace.php", "<?php\n/*\nPlugin Name: Better Search Replace\nVersion: 1.4.10\n*/\n")
	write(t, root, "plugins/better-search-replace/ext/class-bsr-plugin-updater.php", "<?php // WP Engine build updater\n")
	write(t, root, "plugins/akismet/views/two.php", "<?php // tagged build\n")    // the second accepted hash
	write(t, root, "plugins/premium-thing/premium-thing.php", "<?php\n/*\nPlugin Name: Premium\nVersion: 2.0\n*/\n")
	write(t, root, "plugins/no-header/lib.php", "<?php\n")
	write(t, root, "themes/twentytwentyfour/style.css", twenty["style.css"])
	write(t, root, "themes/twentytwentyfour/functions.php", twenty["functions.php"]+"include 'x';\n") // modified
	write(t, root, "themes/twentytwentyfour/index.php", twenty["index.php"])

	comps := FindComponents(root)
	if len(comps) != 4 {
		t.Fatalf("components: %+v", comps)
	}
	store := NewManifestStore(filepath.Join(root, "cache"))
	store.BaseURL = srv.URL
	res := store.CheckTree(root, comps)
	if len(res.Errors) != 0 {
		t.Fatalf("errors: %v", res.Errors)
	}
	if res.Components != 4 || res.Covered != 3 {
		t.Fatalf("covered %d of %d", res.Covered, res.Components)
	}
	got := map[string]string{}
	for _, f := range res.Findings {
		got[f.File] = f.RuleID + ":" + f.Severity
	}
	want := map[string]string{
		"plugins/akismet/class.akismet.php":     "integrity-modified-file:medium",
		"plugins/akismet/wp-cache.php":          "integrity-unknown-file:high",
		"themes/twentytwentyfour/functions.php": "integrity-modified-file:medium",
	}
	if len(got) != len(want) {
		t.Fatalf("findings: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q", k, got[k], v)
		}
	}
	if res.Modified != 2 || res.Unknown != 1 {
		t.Errorf("counts modified=%d unknown=%d", res.Modified, res.Unknown)
	}
	// The edited class file carries eval($_POST): with the rule finding on
	// the same file the medium modified finding becomes high.
	s := shippedRules(t)
	all := Escalate(append(s.ScanDir(root).Findings, res.Findings...))
	sev := map[string]string{}
	for _, f := range all {
		if f.RuleID == "integrity-modified-file" {
			sev[f.File] = f.Severity
		}
	}
	if sev["plugins/akismet/class.akismet.php"] != "high" || sev["themes/twentytwentyfour/functions.php"] != "medium" {
		t.Errorf("escalation: %v", sev)
	}
	if !res.KnownGood[sha(akismet["class.akismet.php"])] || !res.KnownGood[sha(twenty["index.php"])] {
		t.Error("known-good set lacks release hashes")
	}
	// Cached now: a second store on the same directory makes no requests,
	// and the premium plugin's 404 is remembered too.
	before := atomic.LoadInt32(hits)
	store2 := NewManifestStore(filepath.Join(root, "cache"))
	store2.BaseURL = srv.URL
	res2 := store2.CheckTree(root, comps)
	if atomic.LoadInt32(hits) != before {
		t.Errorf("cache miss: %d requests after warm cache", atomic.LoadInt32(hits)-before)
	}
	if len(res2.Findings) != len(res.Findings) {
		t.Errorf("cached run differs: %d vs %d findings", len(res2.Findings), len(res.Findings))
	}
	if _, err := os.Stat(filepath.Join(root, "cache", "plugins", "premium-thing", "2.0.json.missing")); err != nil {
		t.Errorf("missing marker not written: %v", err)
	}
	// An expired missing marker is asked again.
	store3 := NewManifestStore(filepath.Join(root, "cache"))
	store3.BaseURL = srv.URL
	store3.MissingTTL = time.Nanosecond
	time.Sleep(2 * time.Millisecond)
	store3.CheckTree(root, comps)
	if atomic.LoadInt32(hits) != before+1 {
		t.Errorf("expired marker not refetched: %d", atomic.LoadInt32(hits)-before)
	}
}

// A commercial build under a wordpress.org slug differs everywhere: one low
// note, not a finding per file.
func TestIntegrityDifferentBuild(t *testing.T) {
	files := map[string]string{"main.php": "<?php\n/*\nPlugin Name: Big\nVersion: 1.0\n*/\n"}
	for i := 0; i < 20; i++ {
		files["inc/f"+string(rune('a'+i))+".php"] = "<?php // release " + string(rune('a'+i)) + "\n"
	}
	srv, _ := fakeWordPressOrg(t, map[string]map[string]string{"big/1.0": files}, nil)
	root := t.TempDir()
	write(t, root, "plugins/big/main.php", files["main.php"])
	for i := 0; i < 20; i++ {
		write(t, root, "plugins/big/inc/f"+string(rune('a'+i))+".php", "<?php // premium "+string(rune('a'+i))+"\n")
	}
	store := NewManifestStore("")
	store.BaseURL = srv.URL
	res := store.CheckTree(root, FindComponents(root))
	if len(res.Findings) != 1 || res.Findings[0].RuleID != "integrity-different-build" || res.Findings[0].Severity != "low" {
		t.Fatalf("findings: %+v", res.Findings)
	}
	if res.Mismatched != 1 || res.Modified != 0 {
		t.Errorf("counts: %+v", res)
	}
}

// A plugin that unpacks a bundle into its own directory (phpMyAdmin, a
// template cache) has dozens of unknown PHP files: one medium note.
func TestIntegrityGeneratedFilesCluster(t *testing.T) {
	files := map[string]string{"pma.php": "<?php\n/*\nPlugin Name: PMA\nVersion: 1.0\n*/\n", "lib/loader.php": "<?php\n"}
	plugins := map[string]map[string]string{"wp-pma/1.0": files}
	srv, _ := fakeWordPressOrg(t, plugins, nil)
	root := t.TempDir()
	for rel, c := range files {
		write(t, root, "plugins/wp-pma/"+rel, c)
	}
	for i := 0; i < 12; i++ {
		write(t, root, fmt.Sprintf("plugins/wp-pma/lib/pma/tmp/twig/%02d.php", i), "<?php // cache\n")
	}
	// The same shape inside a release of thousands of files is still a
	// cluster once it passes the absolute count.
	big := map[string]string{"big.php": "<?php\n/*\nPlugin Name: Big\nVersion: 1.0\n*/\n"}
	for i := 0; i < 400; i++ {
		big[fmt.Sprintf("lib/f%03d.php", i)] = fmt.Sprintf("<?php // %d\n", i)
	}
	plugins["big/1.0"] = big
	write(t, root, "plugins/big/big.php", big["big.php"])
	for i := 0; i < 30; i++ {
		write(t, root, fmt.Sprintf("plugins/big/tmp/%02d.php", i), "<?php // cache\n")
	}
	write(t, root, "plugins/wp-pma/lib/loader.php", "<?php // edited\n")
	store := NewManifestStore("")
	store.BaseURL = srv.URL
	for _, res := range []IntegrityResult{
		store.CheckTree(root, FindComponents(root)),
		store.CheckPaths(root, allFiles(t, filepath.Join(root, "plugins"))),
	} {
		ids := map[string]int{}
		for _, f := range res.Findings {
			ids[f.RuleID+":"+f.Severity]++
		}
		if ids["integrity-generated-files:medium"] != 2 || ids["integrity-modified-file:medium"] != 1 || len(res.Findings) != 3 {
			t.Errorf("findings: %v", ids)
		}
		if res.Generated != 2 || res.Unknown != 0 || res.Modified != 1 {
			t.Errorf("counts: generated=%d unknown=%d modified=%d", res.Generated, res.Unknown, res.Modified)
		}
	}
}

func allFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestIntegrityCheckPaths(t *testing.T) {
	akismet := map[string]string{
		"akismet.php":       "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\n*/\n",
		"class.akismet.php": "<?php class Akismet {}\n",
	}
	srv, hits := fakeWordPressOrg(t, map[string]map[string]string{"akismet/5.3": akismet}, nil)
	root := t.TempDir()
	write(t, root, "plugins/akismet/akismet.php", akismet["akismet.php"])
	write(t, root, "plugins/akismet/class.akismet.php", akismet["class.akismet.php"]+"// injected\n")
	write(t, root, "plugins/akismet/views/new.php", "<?php echo 1;\n")
	write(t, root, "plugins/other/x.php", "<?php\n")
	write(t, root, "uploads/2024/a.php", "<?php\n")
	store := NewManifestStore("")
	store.BaseURL = srv.URL
	paths := []string{
		filepath.Join(root, "plugins/akismet/akismet.php"),
		filepath.Join(root, "plugins/akismet/class.akismet.php"),
		filepath.Join(root, "plugins/akismet/views/new.php"),
		filepath.Join(root, "plugins/other/x.php"),
		filepath.Join(root, "uploads/2024/a.php"),
	}
	res := store.CheckPaths(root, paths)
	if atomic.LoadInt32(hits) != 1 {
		t.Errorf("expected one manifest request, got %d", atomic.LoadInt32(hits))
	}
	got := map[string]string{}
	for _, f := range res.Findings {
		got[f.File] = f.RuleID
	}
	if got["plugins/akismet/class.akismet.php"] != "integrity-modified-file" || got["plugins/akismet/views/new.php"] != "integrity-unknown-file" || len(got) != 2 {
		t.Fatalf("findings: %v", got)
	}
	if !res.KnownGood[sha(akismet["akismet.php"])] {
		t.Error("known-good set missing")
	}
	// A file identical to its release skips the rules even when its contents
	// would otherwise match one.
	s := shippedRules(t)
	if r := s.ScanPaths(paths[1:2], root); len(r.Findings) != 0 {
		t.Fatalf("the modified file must be scanned: %+v", r)
	}
	hot := "<?php eval(base64_decode($_POST['x']));\n"
	write(t, root, "plugins/akismet/hot.php", hot)
	hotPath := filepath.Join(root, "plugins/akismet/hot.php")
	if r := s.ScanPaths([]string{hotPath}, root); len(r.Findings) == 0 {
		t.Fatal("fixture should trip a rule without a known-good set")
	}
	s.opts.KnownGood = KnownGoodFunc(map[string]bool{sha(hot): true})
	if r := s.ScanPaths([]string{hotPath}, root); len(r.Findings) != 0 {
		t.Errorf("known-good file still matched: %+v", r.Findings)
	}
}

func TestComponentOf(t *testing.T) {
	cases := map[string][2]string{
		"plugins/akismet/x.php":                {"plugin", "plugins/akismet"},
		"wp-content/plugins/akismet/inc/y.php": {"plugin", "wp-content/plugins/akismet"},
		"themes/astra/functions.php":           {"theme", "themes/astra"},
		"plugins/akismet":                      {"", ""},
		"uploads/plugins/fake/x.php":           {"plugin", "uploads/plugins/fake"},
		"mu-plugins/x.php":                     {"", ""},
		"app/themes/sage/index.php":            {"theme", "app/themes/sage"},
		"myplugins/akismet/x.php":              {"", ""},
	}
	for rel, want := range cases {
		typ, dir := ComponentOf(rel)
		if typ != want[0] || dir != want[1] {
			t.Errorf("%s: got %q %q want %q %q", rel, typ, dir, want[0], want[1])
		}
	}
}

func TestHeaderVersion(t *testing.T) {
	root := t.TempDir()
	write(t, root, "p/loader.php", "<?php // helper\n")
	write(t, root, "p/plugin.php", "<?php\n/**\n * Plugin Name: Thing\n * Version:     1.2.3-beta\n * Author: x\n */\n")
	if v := pluginVersion(filepath.Join(root, "p")); v != "1.2.3-beta" {
		t.Errorf("plugin version %q", v)
	}
	write(t, root, "t/style.css", "/*\nTheme Name: T\nVersion: 2.0\n*/\n")
	if v := themeVersion(filepath.Join(root, "t")); v != "2.0" {
		t.Errorf("theme version %q", v)
	}
	write(t, root, "q/q.php", "<?php\n/*\nPlugin Name: NoVersion\n*/\n")
	if v := pluginVersion(filepath.Join(root, "q")); v != "" {
		t.Errorf("expected empty version, got %q", v)
	}
	if _, err := NewManifestStore("").Get("plugin", "../etc", "1"); err != ErrNotOnWordPressOrg {
		t.Errorf("unsafe slug accepted: %v", err)
	}
}
