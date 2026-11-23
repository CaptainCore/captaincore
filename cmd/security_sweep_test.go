package cmd

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestWPConfigFindings(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

	clean := `<?php
define('DB_NAME', 'wpdb');
define('DB_USER', 'wpuser');
define('DB_PASSWORD', 'REDACTED');
define('AUTH_KEY', 'REDACTED');
$table_prefix = 'wp_';
require_once ABSPATH . 'wp-settings.php';
`
	rawClean, _ := json.Marshal(map[string]any{
		"path":   "wp-config.php",
		"sha256": strings.Repeat("11", 32),
		"size":   len(clean),
		"b64":    b64(clean),
	})
	if f := wpConfigFindings(string(rawClean), "low"); len(f) != 0 {
		t.Errorf("clean wp-config.php must yield 0 findings, got %v", f)
	}

	infected := clean + "\n@eval(base64_decode($_POST['cmd']));\n"
	wantHash := strings.Repeat("22", 32)
	rawBad, _ := json.Marshal(map[string]any{
		"path":   "wp-config.php",
		"sha256": wantHash,
		"size":   len(infected),
		"b64":    b64(infected),
	})
	findings := wpConfigFindings(string(rawBad), "low")
	if len(findings) == 0 {
		t.Fatal("infected wp-config.php raised no findings")
	}
	for _, f := range findings {
		if f.Filename != "wp-config.php" {
			t.Errorf("filename = %q, want wp-config.php", f.Filename)
		}
		if f.ContentHash != wantHash {
			t.Errorf("content_hash = %q, want %q", f.ContentHash, wantHash)
		}
	}

	if wpConfigFindings("null", "low") != nil || wpConfigFindings("", "low") != nil || wpConfigFindings("bad", "low") != nil {
		t.Error("empty/null/malformed input must return nil")
	}
}

func TestCleanHashCacheLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean-scan-hashes.json")
	shaClean := strings.Repeat("aa", 32)
	shaDirty := strings.Repeat("bb", 32)

	c1 := loadCleanHashCache(path, "rules-v1")
	if c1.Has(shaClean) {
		t.Fatal("fresh cache should not contain hash")
	}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	rawClean, _ := json.Marshal(map[string]any{
		"path": "wp-config.php", "sha256": shaClean, "size": 30,
		"b64": b64("<?php define('DB_NAME','x');\n"),
	})
	if f := wpConfigFindingsWithCache(string(rawClean), "low", c1); len(f) != 0 {
		t.Fatalf("expected clean, got %v", f)
	}
	if !c1.Has(shaClean) {
		t.Fatal("clean wp-config sha256 must be cached")
	}

	rawBad, _ := json.Marshal(map[string]any{
		"path": "wp-config.php", "sha256": shaDirty, "size": 60,
		"b64": b64("<?php @eval(base64_decode($_POST['x']));\n"),
	})
	if f := wpConfigFindingsWithCache(string(rawBad), "low", c1); len(f) == 0 {
		t.Fatal("expected findings for infected wp-config")
	}
	if c1.Has(shaDirty) {
		t.Fatal("infected sha256 must never be cached as clean")
	}
	if err := c1.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload under same rulesHash: clean hash persists.
	c2 := loadCleanHashCache(path, "rules-v1")
	if !c2.Has(shaClean) {
		t.Fatal("clean hash should survive reload with same rulesHash")
	}
	// Reload under a new rulesHash: cache invalidates automatically.
	c3 := loadCleanHashCache(path, "rules-v2")
	if c3.Has(shaClean) {
		t.Fatal("clean hash must invalidate when rulesHash changes")
	}
}
