package cmd

import (
	"encoding/json"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
)

// benignRootPHP lists root-level PHP files that hosts and security plugins
// legitimately place at the WordPress web root. fetch-site-data reports every
// non-core root file (the raw list is kept in environment details for
// forensics); these names are filtered out before a finding is raised so the
// first fleet sync does not raise a thousand alerts for healthcheck and WAF
// prepend files. Everything here was observed as legitimate infrastructure in
// the 2026-09 fleet sweep. Names are compared lowercase.
var benignRootPHP = map[string]bool{
	"lwhostscheck.php":         true, // Kinsta host health check
	"wordfence-waf.php":        true, // Wordfence WAF prepend
	"malcare-waf.php":          true, // MalCare WAF prepend
	"aios-bootstrap.php":       true, // All-In-One Security prepend
	"gd-config.php":            true, // GoDaddy managed WordPress
	"gd-preload-cli.php":       true, // GoDaddy managed WordPress
	"sucuri-version-check.php": true, // Sucuri
	"sucuri-db-cleanup.php":    true, // Sucuri
	"sucuri-cleanup.php":       true, // Sucuri
	"sucuri_listcleaned.php":   true, // Sucuri
	"wp-salt.php":              true, // salts split out of wp-config
	"cloner.php":               true, // WP Umbrella / ManageWP clone helper
	"webformmailer.php":        true, // GoDaddy web form mailer
}

// benignRootFile reports whether a root-level filename is a known legitimate
// placement. Any wp-config variant (wp-config-orig.php, wp-config.dev.php …)
// is a configuration backup: a credential-disclosure hygiene item handled
// elsewhere, not a provenance finding, so it is filtered here too.
func benignRootFile(name string) bool {
	lower := strings.ToLower(name)
	if benignRootPHP[lower] {
		return true
	}
	return strings.HasPrefix(lower, "wp-config")
}

// provenanceFindings turns the unexpected_root_php JSON emitted by
// fetch-site-data into malware findings, dropping known-benign root
// placements. Content-root rows are never filtered: only index.php and the
// drop-ins WordPress recognizes belong there, and the collector already
// excludes those. Severity is high: on a clean production root nothing
// survives the allowlists, and this is the class that pattern rules miss.
func provenanceFindings(raw string) []scan.LegacyFinding {
	var rows []struct {
		Location, Path string
	}
	if json.Unmarshal([]byte(raw), &rows) != nil {
		return nil
	}
	var findings []scan.LegacyFinding
	for _, r := range rows {
		base := r.Path
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		id, name, desc := "provenance-unexpected-content-php",
			"Unexpected PHP file at the content root",
			"A PHP file sits at the wp-content root but is not index.php or a drop-in WordPress recognizes."
		if r.Location == "root" {
			if benignRootFile(base) {
				continue
			}
			id, name, desc = "provenance-unexpected-root-php",
				"Unexpected PHP file at the WordPress root",
				"A PHP file sits at the web root but is not one WordPress core ships; stock installs never add PHP here."
		}
		findings = append(findings, scan.LegacyFinding{
			Filename:             r.Path,
			SignatureID:          id,
			SignatureName:        name,
			SignatureDescription: desc,
			Severity:             "high",
			Family:               "integrity",
		})
	}
	return findings
}
