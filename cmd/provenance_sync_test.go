package cmd

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

type provCase struct {
	loc, path, wantID, wantSev string // wantID "" = must be filtered
}

// runProv builds the input the way fetch-site-data does (real JSON, so a
// backslash in a filename is escaped rather than corrupting the document).
func runProv(t *testing.T, cases []provCase) {
	t.Helper()
	type row struct {
		Location string `json:"location"`
		Path     string `json:"path"`
	}
	var rows []row
	for _, c := range cases {
		rows = append(rows, row{c.loc, c.path})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]string{}
	for _, f := range provenanceFindings(string(b)) {
		got[f.Filename] = [2]string{f.SignatureID, f.Severity}
		if f.Family != "integrity" {
			t.Errorf("%s: family %q, want integrity", f.Filename, f.Family)
		}
	}
	for _, c := range cases {
		g, ok := got[c.path]
		if c.wantID == "" {
			if ok {
				t.Errorf("%s: expected filtered, got %v", c.path, g)
			}
			continue
		}
		if !ok {
			t.Errorf("%s: expected %s/%s, got nothing", c.path, c.wantID, c.wantSev)
			continue
		}
		if g[0] != c.wantID || g[1] != c.wantSev {
			t.Errorf("%s: got %s/%s, want %s/%s", c.path, g[0], g[1], c.wantID, c.wantSev)
		}
	}
}

func TestProvenanceRootShellsAreHigh(t *testing.T) {
	runProv(t, []provCase{
		{"root", "wp-locale-cache.php", "provenance-unexpected-root-php", "high"},
		{"root", "kr224ws_e30259e9.php", "provenance-unexpected-root-php", "high"},
		{"root", "db.php", "provenance-unexpected-root-php", "high"}, // Adminer at the root is never benign
		{"root", "dl-file.php", "provenance-unexpected-root-php", "high"},
	})
}

func TestProvenanceRootInfraIsFiltered(t *testing.T) {
	runProv(t, []provCase{
		{"root", "lwHostsCheck.php", "", ""},
		{"root", "wordfence-waf.php", "", ""},
		{"root", "CLONER.PHP", "", ""},
		{"root", "wp-config-orig.php", "", ""},
		{"root", "WP-CONFIG.dev.PHP", "", ""},
		{"root", "core-preview-boot-bfbfd9a4.php", "", ""},
		{"root", "bv_connector_6a78e61788e06cd8896b5c6ac33c4a8f.php", "", ""},
		{"root", "sucuri-d14127c2d33c93d5b6d64b1cb521005e.php", "", ""},
		{"root", "MOJOWordpressInstaller-Sljb3I2c4C.php", "", ""},
	})
}

func TestProvenanceDevLeftoversAreMedium(t *testing.T) {
	runProv(t, []provCase{
		{"root", "phpinfo.php", "provenance-unexpected-root-php", "medium"},
		{"root", "t.php", "provenance-unexpected-root-php", "medium"},
		{"root", "500.php", "provenance-unexpected-root-php", "medium"},
		{"root", "wp-rss.php", "provenance-unexpected-root-php", "medium"}, // legacy core
		{"root", "local-xdebuginfo.php", "provenance-unexpected-root-php", "medium"},
	})
}

func TestProvenanceContentRoot(t *testing.T) {
	runProv(t, []provCase{
		// plugin-generated files belong there
		{"content", "wp-content/autoptimize_404_handler.php", "", ""},
		{"content", "wp-content/wp-defender-secrets.php", "", ""},
		{"content", "wp-content/wp-cache-config.php", "", ""},
		{"content", "app/freighter.php", "", ""},
		// real shells and misplaced code do not
		{"content", "wp-content/wp-locale-cache.php", "provenance-unexpected-content-php", "high"},
		{"content", "wp-content/uploads\\kon.php", "provenance-unexpected-content-php", "high"},
		{"content", "wp-content/wpn-sops.php", "provenance-unexpected-content-php", "high"},
		// dev-leftover names are a ROOT concept; at the content root they stay high
		{"content", "wp-content/test.php", "provenance-unexpected-content-php", "high"},
	})
}

func TestProvenanceFindingsEmptyAndGarbage(t *testing.T) {
	if f := provenanceFindings("[]"); len(f) != 0 {
		t.Errorf("empty list should yield no findings, got %d", len(f))
	}
	if f := provenanceFindings("not json"); f != nil {
		t.Errorf("malformed input should yield nil, got %v", f)
	}
}

func TestProvenanceEvidenceRidesAlong(t *testing.T) {
	raw := `[{"location":"root","path":"kwmailer.php","sha256":"7647582e6d1db60d8fd16671e22654623e6157809cef4ab8eca56c66eaa3ff85","size":458,"mtime":"2025-10-01 20:58","head":"<?php\nmail(\"info@example.com\", $_POST['Email']);"},
		{"location":"root","path":"legacy.php"}]`
	got := map[string]bool{}
	for _, f := range provenanceFindings(raw) {
		got[f.Filename] = true
		switch f.Filename {
		case "kwmailer.php":
			if f.ContentHash != "7647582e6d1db60d8fd16671e22654623e6157809cef4ab8eca56c66eaa3ff85" {
				t.Errorf("content hash %q", f.ContentHash)
			}
			want := "sha256 7647582e6d1db60d8fd16671e22654623e6157809cef4ab8eca56c66eaa3ff85, 458 bytes, modified 2025-10-01 20:58 UTC\n<?php"
			if len(f.MatchedText) < len(want) || f.MatchedText[:len(want)] != want {
				t.Errorf("matched text %q", f.MatchedText)
			}
		case "legacy.php":
			// rows from an older fetch-site-data carry no evidence: nothing invented
			if f.ContentHash != "" || f.MatchedText != "" {
				t.Errorf("legacy row got hash %q / text %q", f.ContentHash, f.MatchedText)
			}
		}
	}
	if !got["kwmailer.php"] || !got["legacy.php"] {
		t.Errorf("findings missing: %v", got)
	}
}

// A root webshell behind a decoy header must reach the review with its rule
// hits, under its own path and whole-file hash; a stock-looking file must not.
func TestRootPHPContentFindings(t *testing.T) {
	shell := "<?php\n/**\n * @package Joomla.Site\n * @subpackage com_contact\n */\n" +
		"if(isset($_POST['shnew'])){ $n = trim($_POST['shnew']).'.php'; copy(__FILE__, $n); unlink(__FILE__); exit; }\n" +
		"function pre_term_name($d) { $f = strrev('46esab').'_'.strrev('edoced'); $g = strrev('etalfnizg'); return @$g($f($d)); }\n"
	waf := "<?php\n// Before removing this file, please verify the PHP ini setting `auto_prepend_file` does not point to this.\n" +
		"if (file_exists(__DIR__.'/wp-content/plugins/wordfence/waf/bootstrap.php')) {\n\tdefine(\"WFWAF_LOG_PATH\", __DIR__.'/wp-content/wflogs/');\n\tinclude_once __DIR__.'/wp-content/plugins/wordfence/waf/bootstrap.php';\n}\n"
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	raw, _ := json.Marshal([]map[string]any{
		{"path": "image.php", "sha256": strings.Repeat("ab", 32), "b64": b64(shell)},
		{"path": "wordfence-waf.php", "sha256": strings.Repeat("cd", 32), "b64": b64(waf)},
		{"path": "wp-content/big.phtml", "sha256": strings.Repeat("ef", 32), "b64": b64(shell), "truncated": true},
	})
	got := map[string][]string{}
	for _, f := range rootPHPContentFindings(string(raw), "low") {
		got[f.Filename] = append(got[f.Filename], f.SignatureID)
		if f.Filename == "image.php" && f.ContentHash != strings.Repeat("ab", 32) {
			t.Errorf("finding must carry the whole file's sha256, got %q", f.ContentHash)
		}
		if f.Filename == "wp-content/big.phtml" && !strings.Contains(f.SignatureDescription, "content root (first 1 MB scanned)") {
			t.Errorf("description %q", f.SignatureDescription)
		}
	}
	if len(got["image.php"]) == 0 {
		t.Errorf("the decoy-header webshell raised no rule finding (all: %v)", got)
	}
	if len(got["wp-content/big.phtml"]) == 0 {
		t.Errorf("a .phtml file must be scanned as PHP (all: %v)", got)
	}
	if len(got["wordfence-waf.php"]) != 0 {
		t.Errorf("the Wordfence prepend must stay quiet, got %v", got["wordfence-waf.php"])
	}
	if rootPHPContentFindings(`[]`, "low") != nil || rootPHPContentFindings(`not json`, "low") != nil {
		t.Error("empty or bad input must yield nothing")
	}
}
