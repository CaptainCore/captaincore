package cmd

import "testing"

func TestProvenanceFindingsKeepsShellsDropsInfra(t *testing.T) {
	// Mirrors what fetch-site-data emits: root shells, host/WAF infra files,
	// a config backup, and a stray content-root file.
	raw := `[
		{"location":"root","path":"wp-locale-cache.php"},
		{"location":"root","path":"kr224ws_e30259e9.php"},
		{"location":"root","path":"lwHostsCheck.php"},
		{"location":"root","path":"wordfence-waf.php"},
		{"location":"root","path":"wp-config-orig.php"},
		{"location":"root","path":"WP-CONFIG.dev.PHP"},
		{"location":"content","path":"wp-content/wp-locale-cache.php"}
	]`
	f := provenanceFindings(raw)
	got := map[string]string{}
	for _, x := range f {
		got[x.Filename] = x.SignatureID
	}
	if got["wp-locale-cache.php"] != "provenance-unexpected-root-php" {
		t.Errorf("root shell not flagged: %v", got)
	}
	if got["kr224ws_e30259e9.php"] != "provenance-unexpected-root-php" {
		t.Errorf("arbitrarily named root shell not flagged: %v", got)
	}
	if got["wp-content/wp-locale-cache.php"] != "provenance-unexpected-content-php" {
		t.Errorf("content-root shell not flagged: %v", got)
	}
	for _, benign := range []string{"lwHostsCheck.php", "wordfence-waf.php", "wp-config-orig.php", "WP-CONFIG.dev.PHP"} {
		if _, ok := got[benign]; ok {
			t.Errorf("benign infra/config file must be filtered: %s", benign)
		}
	}
	if len(f) != 3 {
		t.Errorf("expected exactly 3 findings, got %d: %v", len(f), got)
	}
	for _, x := range f {
		if x.Severity != "high" || x.Family != "integrity" {
			t.Errorf("finding %s should be high/integrity, got %s/%s", x.Filename, x.Severity, x.Family)
		}
	}
}

func TestProvenanceFindingsEmptyAndGarbage(t *testing.T) {
	if f := provenanceFindings("[]"); len(f) != 0 {
		t.Errorf("empty list should yield no findings, got %d", len(f))
	}
	if f := provenanceFindings("not json"); f != nil {
		t.Errorf("malformed input should yield nil, got %v", f)
	}
}

func TestBenignRootFileRootDBIsNotBenign(t *testing.T) {
	// Adminer dropped as db.php at the WEB ROOT must stay a finding; db.php is
	// only a legitimate drop-in inside wp-content (handled by the collector).
	if benignRootFile("db.php") {
		t.Error("root-level db.php must not be treated as benign")
	}
	if !benignRootFile("cloner.php") || !benignRootFile("LWHOSTSCHECK.PHP") {
		t.Error("known infra names should be benign regardless of case")
	}
}
