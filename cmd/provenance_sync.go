package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
)

// The lists below were built from a full production fleet sweep (2026-09) of
// every non-core PHP file at the web root and content root, with each name
// bucket read on disk before it was classified. Names are compared lowercase.

// benignRootPHP: root-level PHP that hosts and security plugins legitimately
// place at the WordPress web root. Filtered before a finding is raised so the
// first fleet sync does not alert on healthcheck and WAF prepend files. The
// raw list still lands in environment details for forensics.
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
	"ss_installhelper.php":     true, // SiteGround installer helper
	"wp-magic-login.php":       true, // host one-time login helper
}

// benignRootPrefixes: root-level files that carry a per-install token in the
// name, so they can only be matched by prefix. wp-config* covers every config
// backup (wp-config-orig.php, wp-config.dev.php …): a credential-disclosure
// hygiene item handled elsewhere, not a provenance finding.
var benignRootPrefixes = []string{
	"wp-config",              // configuration backups
	"core-preview-boot-",     // CaptainCore preview links
	"bv_connector_",          // BlogVault connector
	"sucuri-",                // Sucuri per-site helper
	"mojowordpressinstaller", // MOJO Marketplace installer
}

// devLeftoverRootPHP: root files that are not malware but are not supposed to
// be on a production site either: info-disclosure probes, installer residue,
// and core files WordPress removed years ago. Reported at medium so they reach
// the daily review instead of paging anyone.
var devLeftoverRootPHP = map[string]bool{
	"phpinfo.php": true, "info.php": true, "test.php": true, "t.php": true,
	"500.php": true, "clean.php": true, "default.php": true,
	"local-xdebuginfo.php": true, // Local by Flywheel
	"fantversion.php":      true, // Fantastico
	"ssv3_directory.php":   true, // SiteGround
	// legacy core, removed from WordPress long ago
	"wp-rss.php": true, "wp-rss2.php": true, "wp-atom.php": true, "wp-feed.php": true,
	"wp-rdf.php": true, "wp-commentsrss2.php": true, "wp-register.php": true,
}

// benignContentPHP: files that plugins legitimately write to the content root
// next to index.php and the core drop-ins. The collector already excludes the
// drop-ins WordPress itself recognizes; this covers the plugin-generated rest.
var benignContentPHP = map[string]bool{
	"autoptimize_404_handler.php":     true, // Autoptimize
	"freighter.php":                   true, // WP Freighter
	"wp-defender-secrets.php":         true, // WP Defender
	"wp-cache-config.php":             true, // WP Super Cache
	"backup-migration-config.php":     true, // Backup Migration
	"sgs_encrypt_key.php":             true, // SiteGround Security
	"sgo-config.php":                  true, // SiteGround Optimizer
	"advanced-headers.php":            true, // Really Simple Security
	"advanced-headers-test.php":       true, // Really Simple Security
	"firewall.php":                    true, // Really Simple Security Pro
	"advanced-cache-backup.php":       true, // cache plugin drop-in backup
	"object-cache-backup.php":         true, // object cache drop-in backup
	"object-cache-socket-missing.php": true, // Object Cache Pro
	"akeebabackup_secretkey.php":      true, // Akeeba Backup
	"wp-link-status-salt.php":         true, // WP Link Status
}

// benignRootFile reports whether a root-level filename is a known legitimate
// placement. Root-level db.php is deliberately NOT here: Adminer dropped at the
// web root must stay a finding; db.php is only a drop-in inside the content dir.
func benignRootFile(name string) bool {
	lower := strings.ToLower(name)
	if benignRootPHP[lower] {
		return true
	}
	for _, p := range benignRootPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// provenanceFindings turns the unexpected_root_php JSON emitted by
// fetch-site-data into malware findings. Known host and plugin placements are
// dropped, dev leftovers and legacy core files are reported at medium (daily
// review), and everything else is high: on a clean production root nothing
// survives these lists, and this is the class that pattern rules miss.
func provenanceFindings(raw string) []scan.LegacyFinding {
	var rows []struct {
		Location, Path string
		// Evidence from fetch-site-data (absent on older remote scripts).
		Sha256, Mtime, Head string
		Size                int64
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
		lower := strings.ToLower(base)
		severity := "high"
		id, name, desc := "provenance-unexpected-content-php",
			"Unexpected PHP file at the content root",
			"A PHP file sits at the wp-content root but is not index.php, a drop-in WordPress recognizes, or a known plugin-generated file."
		switch r.Location {
		case "root":
			if benignRootFile(base) {
				continue
			}
			id, name, desc = "provenance-unexpected-root-php",
				"Unexpected PHP file at the WordPress root",
				"A PHP file sits at the web root but is not one WordPress core ships; stock installs never add PHP here."
			if devLeftoverRootPHP[lower] {
				severity = "medium"
				desc = "A development leftover or legacy core file at the web root: not malware, but not something a production site should serve."
			}
		default:
			if benignContentPHP[lower] {
				continue
			}
		}
		// The file's identity and first bytes ride along as the matched text so
		// the daily review can judge the file without a login, and the sha256
		// lets the Manager reopen a reviewed file only when its content changes.
		matched := ""
		if r.Sha256 != "" {
			matched = fmt.Sprintf("sha256 %s, %d bytes, modified %s UTC\n%s", r.Sha256, r.Size, r.Mtime, r.Head)
		}
		findings = append(findings, scan.LegacyFinding{
			Filename:             r.Path,
			SignatureID:          id,
			SignatureName:        name,
			SignatureDescription: desc,
			MatchedText:          matched,
			Severity:             severity,
			Family:               "integrity",
			ContentHash:          r.Sha256,
		})
	}
	return findings
}

// rootPHPContentFindings runs the malware rule set over the root and
// content-root PHP that fetch-site-data sends next to the provenance list
// (unexpected_root_php_content). Quicksaves hold only wp-content, so before
// this a root file was judged by its name and first 600 bytes, and a webshell
// behind a decoy comment header passed review as benign. Every file is
// scanned, including the host and plugin placements provenanceFindings drops
// by name, so a trusted name cannot hide altered content. Rule severities are
// kept as they are, and each finding carries the whole file's sha256, so a
// benign verdict sticks until the file changes.
func rootPHPContentFindings(raw string, minSeverity string) []scan.LegacyFinding {
	var files []struct {
		Path, Sha256, B64 string
		Truncated         bool
	}
	if json.Unmarshal([]byte(raw), &files) != nil || len(files) == 0 {
		return nil
	}
	rs, err := scan.LoadDefaultRuleSet()
	if err != nil {
		return nil
	}
	dir, err := os.MkdirTemp("", "cc-rootphp-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	label := map[string]int{}
	for i, f := range files {
		body, err := base64.StdEncoding.DecodeString(f.B64)
		if err != nil || len(body) == 0 {
			continue
		}
		// The scanner picks PHP rules by extension; .phtml and friends are PHP too.
		name := fmt.Sprintf("%03d-%s", i, unsafeName.ReplaceAllString(filepath.Base(f.Path), "_"))
		if !strings.HasSuffix(strings.ToLower(name), ".php") {
			name += ".php"
		}
		if os.WriteFile(filepath.Join(dir, name), body, 0o600) == nil {
			label[name] = i
		}
	}
	min := scan.SeverityRank(minSeverity)
	seen := map[string]bool{}
	var out []scan.LegacyFinding
	for _, f := range scan.New(rs, scan.Options{Workers: 2}).ScanDir(dir).Findings {
		i, ok := label[filepath.Base(f.File)]
		if !ok || scan.SeverityRank(f.Severity) < min {
			continue
		}
		l := f.Legacy()
		l.Filename = files[i].Path
		if files[i].Sha256 != "" {
			l.ContentHash = files[i].Sha256
		}
		where := "PHP file at the web root"
		if strings.Contains(files[i].Path, "/") {
			where = "PHP file at the content root"
		}
		if files[i].Truncated {
			where += " (first 1 MB scanned)"
		}
		l.SignatureDescription = where + ": " + l.SignatureDescription
		if key := l.Filename + "|" + l.SignatureID; !seen[key] {
			seen[key] = true
			out = append(out, l)
		}
	}
	return out
}
