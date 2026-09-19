package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDBScanFindings(t *testing.T) {
	export := map[string]any{
		"since_days": 2,
		"options": []map[string]any{
			{"name": "widget_text", "size": 400, "content": "a:1:{i:2;a:1:{s:4:\"text\";s:80:\"<script>eval(atob('ZG9jdW1lbnQud3JpdGUoJzxpZnJhbWUgc3JjPWh0dHA6Ly9leGFtcGxlLmludmFsaWQvPicp'))</script>\";}}"},
			{"name": "cron", "size": 3000, "content": "a:2:{i:1789000000;a:1:{s:17:\"wp_version_check\";a:1:{s:32:\"40cd750bba9870f18aada2478b24840a\";a:2:{s:8:\"schedule\";s:10:\"twicedaily\";}}}}"},
		},
		"posts": []map[string]any{
			{"id": 42, "type": "post", "status": "publish", "title": "Hello", "content": "<p>fine</p><div style=\"position:absolute; left:-9999px;\"><a href=\"https://example.invalid/a\">one</a><a href=\"https://example.invalid/b\">two</a><a href=\"https://example.invalid/c\">three</a></div>"},
			{"id": 43, "type": "page", "status": "publish", "title": "About", "content": "<p>nothing to see</p>"},
		},
		"comments": []map[string]any{},
		"admins": []map[string]any{
			{"id": 1, "login": "austin", "email": "a@example.invalid", "registered": "2020-01-01 00:00:00"},
			{"id": 99, "login": "wpsupp-user", "email": "x@example.invalid", "registered": "2026-09-18 03:12:00"},
		},
		"plugins_missing":    []string{"wp-cache-helper/loader.php"},
		"triggers":           []string{"after_user_insert"},
		"events":             []string{},
		"routines":           []string{},
		"unknown_tables":     []string{"wp_html_injections"},
		"suspicious_options": []string{"wp_html_inject_code"},
		"stats":              map[string]int{"options_total": 900, "options_exported": 2, "posts_checked": 12, "posts_exported": 1},
	}
	raw, _ := json.Marshal(export)
	known := knownUserLogins(`[{"ID":1,"user_login":"austin","roles":"administrator"},{"ID":5,"user_login":"editor"}]`)
	findings, sum, err := dbScanFindings(string(raw), known, "high")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range findings {
		got[f.Filename] = f.SignatureID
	}
	want := map[string]string{
		"db:trigger/after_user_insert":                 "db-trigger",
		"db:active_plugins/wp-cache-helper/loader.php": "db-active-plugin-missing",
		"db:option/wp_html_inject_code":                "db-known-injection-option",
		"db:user/wpsupp-user":                          "db-new-administrator",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}
	if _, ok := got["db:user/austin"]; ok {
		t.Error("a previously synced administrator must not be reported")
	}
	if _, ok := got["db:post/43"]; ok {
		t.Error("a clean post must not be reported")
	}
	// The rule engine must have run over the rows: the off-screen link block
	// in post 42 and the eval(atob(...)) widget are corpus shapes.
	ruleHits := 0
	for _, f := range findings {
		if strings.HasPrefix(f.Filename, "db:post/42") || strings.HasPrefix(f.Filename, "db:option/widget_text") {
			ruleHits++
		}
	}
	if ruleHits == 0 {
		t.Errorf("expected rule findings on the injected rows, got %v", got)
	}
	if sum.Findings != len(findings) || len(sum.Admins) != 2 || sum.Stats["posts_checked"] != 12 {
		t.Errorf("summary: %+v", sum)
	}
	// No previous user list: nothing is "new".
	findings, _, _ = dbScanFindings(string(raw), nil, "high")
	for _, f := range findings {
		if f.SignatureID == "db-new-administrator" {
			t.Error("without a previous user list no administrator can be new")
		}
	}
	if _, _, err := dbScanFindings(`{"error":"db-scan produced no output"}`, nil, "high"); err == nil {
		t.Error("an export error must surface")
	}
}
