package cmd

import (
	"encoding/json"
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
