package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
)

// The database scan. fetch-site-data (nightly) and lib/remote-scripts/db-scan
// (on demand) export the rows of a site's database that could carry an
// injection, plus a few fixed facts. This file judges that export on the core
// server: every exported row is written out as a file and run through the
// malware rules, so the whole rule set and the decode layer apply to option
// values, post content and comments; the fixed checks below cover what a
// string rule cannot see.

// dbScanExport mirrors the JSON the remote PHP emits.
type dbScanExport struct {
	SinceDays int  `json:"since_days"`
	Full      bool `json:"full"`
	Options   []struct {
		Name    string `json:"name"`
		Size    int    `json:"size"`
		Content string `json:"content"`
	} `json:"options"`
	Posts []struct {
		ID       int    `json:"id"`
		Type     string `json:"type"`
		Status   string `json:"status"`
		Modified string `json:"modified"`
		Title    string `json:"title"`
		Content  string `json:"content"`
	} `json:"posts"`
	Comments []struct {
		ID      int    `json:"id"`
		Post    int    `json:"post"`
		Content string `json:"content"`
		URL     string `json:"url"`
	} `json:"comments"`
	Admins []struct {
		ID         int    `json:"id"`
		Login      string `json:"login"`
		Email      string `json:"email"`
		Registered string `json:"registered"`
		Super      bool   `json:"super"`
	} `json:"admins"`
	PluginsMissing    []string       `json:"plugins_missing"`
	Triggers          []string       `json:"triggers"`
	Events            []string       `json:"events"`
	Routines          []string       `json:"routines"`
	UnknownTables     []string       `json:"unknown_tables"`
	SuspiciousOptions []string       `json:"suspicious_options"`
	ToolkitMarkers    []string       `json:"toolkit_markers"`
	Stats             map[string]int `json:"stats"`
	Truncated         bool           `json:"truncated"`
	Error             string         `json:"error"`
}

// dbScanSummary is what environment details keep: counts and names, never
// row contents.
type dbScanSummary struct {
	At                string         `json:"at"`
	Stats             map[string]int `json:"stats"`
	Findings          int            `json:"findings"`
	Admins            []string       `json:"admins"`
	PluginsMissing    []string       `json:"plugins_missing,omitempty"`
	Triggers          []string       `json:"triggers,omitempty"`
	Events            []string       `json:"events,omitempty"`
	Routines          []string       `json:"routines,omitempty"`
	UnknownTables     []string       `json:"unknown_tables,omitempty"`
	SuspiciousOptions []string       `json:"suspicious_options,omitempty"`
	ToolkitMarkers    []string       `json:"toolkit_markers,omitempty"`
	Truncated         bool           `json:"truncated,omitempty"`
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// dbScanFindings judges an export. knownUsers are the user logins the Manager
// already had for the environment (the previous sync), so an administrator
// that was not there before is reported. minSeverity is the alert floor for
// rule findings; the fixed checks carry their own severity and are filtered
// the same way.
func dbScanFindings(raw string, knownUsers map[string]bool, minSeverity string) ([]scan.LegacyFinding, dbScanSummary, error) {
	var ex dbScanExport
	if err := json.Unmarshal([]byte(raw), &ex); err != nil {
		return nil, dbScanSummary{}, fmt.Errorf("db_scan: %w", err)
	}
	if ex.Error != "" {
		return nil, dbScanSummary{}, fmt.Errorf("db_scan: %s", ex.Error)
	}
	min := scan.SeverityRank(minSeverity)
	var out []scan.LegacyFinding
	add := func(sev, where, id, name, desc, match string) {
		if scan.SeverityRank(sev) < min {
			return
		}
		out = append(out, scan.LegacyFinding{Filename: where, SignatureID: id, SignatureName: name, SignatureDescription: desc, MatchedText: match})
	}

	// Rule findings over the exported rows. Each row is written twice, as
	// .php and as .html, so both the PHP and the markup rule sets apply.
	dir, err := os.MkdirTemp("", "cc-dbscan-")
	if err == nil {
		defer os.RemoveAll(dir)
		label := map[string]string{}
		context := map[string]string{} // what the row is, for the finding description
		write := func(kind, key, content, about string) {
			base := kind + "-" + unsafeName.ReplaceAllString(key, "_")
			if len(base) > 120 {
				base = base[:120]
			}
			for _, ext := range []string{".php", ".html"} {
				p := filepath.Join(dir, base+ext)
				os.WriteFile(p, []byte(content), 0o600)
				label[base+ext] = "db:" + kind + "/" + key
				context[base+ext] = about
			}
		}
		for _, o := range ex.Options {
			write("option", o.Name, o.Content, fmt.Sprintf("Option %s (%d bytes)", o.Name, o.Size))
		}
		for _, p := range ex.Posts {
			write("post", fmt.Sprintf("%d", p.ID), p.Content, fmt.Sprintf("%s %d %q (%s, modified %s)", strings.Title(p.Type), p.ID, p.Title, p.Status, p.Modified))
		}
		for _, c := range ex.Comments {
			write("comment", fmt.Sprintf("%d", c.ID), c.Content, fmt.Sprintf("Comment %d on post %d", c.ID, c.Post))
		}
		if len(label) > 0 {
			if rs, err := scan.LoadDefaultRuleSet(); err == nil {
				s := scan.New(rs, scan.Options{Workers: 2})
				res := s.ScanDir(dir)
				seen := map[string]bool{}
				for _, f := range res.Findings {
					if scan.SeverityRank(f.Severity) < min {
						continue
					}
					where := label[f.File]
					key := where + "|" + f.RuleID
					if seen[key] {
						continue // the .php and .html copies fired the same rule
					}
					seen[key] = true
					l := f.Legacy()
					l.Filename = where
					if about := context[f.File]; about != "" {
						l.SignatureDescription = about + ": " + l.SignatureDescription
					}
					out = append(out, l)
				}
			}
		}
	}

	// Fixed checks.
	for _, t := range ex.Triggers {
		add("critical", "db:trigger/"+t, "db-trigger", "Database trigger", "WordPress never creates triggers; injected ones rewrite rows or recreate users on every write", t)
	}
	for _, e := range ex.Events {
		add("critical", "db:event/"+e, "db-event", "Database scheduled event", "WordPress never creates MySQL events; one here runs on a timer inside the database", e)
	}
	for _, r := range ex.Routines {
		add("high", "db:routine/"+r, "db-routine", "Stored routine in the database", "WordPress and its plugins do not use stored procedures or functions", r)
	}
	// A stale active_plugins entry is ordinary: plugins deleted over SFTP stay
	// listed until the plugins screen is opened, and WordPress skips them.
	// Only a path that escapes the plugin directory is worth an alert.
	for _, p := range ex.PluginsMissing {
		if strings.Contains(p, "..") || strings.HasPrefix(p, "/") {
			add("critical", "db:active_plugins/"+p, "db-active-plugin-path-escape", "Active plugin path outside the plugin directory", "active_plugins carries a path with .. or an absolute path, so WordPress loads a file from outside wp-content/plugins on every request", p)
		} else {
			add("low", "db:active_plugins/"+p, "db-active-plugin-missing", "Active plugin whose file is missing", "active_plugins names a file that no longer exists under the plugin directory; usually a plugin removed over SFTP, cleared the next time the plugins screen loads", p)
		}
	}
	for _, o := range ex.SuspiciousOptions {
		add("high", "db:option/"+o, "db-known-injection-option", "Option name from a known injection", "The option name matches names left by past SEO-spam and content injections", o)
	}
	if n := len(ex.ToolkitMarkers); n > 0 {
		sample := ex.ToolkitMarkers
		if len(sample) > 5 {
			sample = sample[:5]
		}
		add("high", "db:option/toolkit-markers", "db-toolkit-session-markers", "Backdoor session markers in wp_options",
			fmt.Sprintf("%d option(s) named wp_<md5 of an IP> holding a Unix timestamp, or prefixed __: the SMILODON toolkit records every admin session it sees this way, and the markers outlive file-only cleanups; a re-drop after cleanup starts by writing a new one", n),
			strings.Join(sample, ", "))
	}
	for _, a := range ex.Admins {
		if knownUsers != nil && !knownUsers[strings.ToLower(a.Login)] {
			add("high", "db:user/"+a.Login, "db-new-administrator", "Administrator the Manager had not seen",
				fmt.Sprintf("%s (%s) holds the administrator role and was not in the previously synced user list; registered %s", a.Login, a.Email, a.Registered), a.Login)
		}
	}
	sum := dbScanSummary{Stats: ex.Stats, Findings: len(out), PluginsMissing: ex.PluginsMissing, Triggers: ex.Triggers, Events: ex.Events,
		Routines: ex.Routines, UnknownTables: ex.UnknownTables, SuspiciousOptions: ex.SuspiciousOptions, ToolkitMarkers: ex.ToolkitMarkers, Truncated: ex.Truncated}
	for _, a := range ex.Admins {
		sum.Admins = append(sum.Admins, a.Login)
	}
	sort.Strings(sum.Admins)
	return out, sum, nil
}

// knownUserLogins reads the logins out of a synced `wp user list` JSON.
func knownUserLogins(usersJSON string) map[string]bool {
	var users []struct {
		Login string `json:"user_login"`
	}
	if json.Unmarshal([]byte(usersJSON), &users) != nil || len(users) == 0 {
		return nil
	}
	m := map[string]bool{}
	for _, u := range users {
		m[strings.ToLower(u.Login)] = true
	}
	return m
}
