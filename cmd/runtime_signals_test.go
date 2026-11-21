package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSecurityLogEventFindings(t *testing.T) {
	events := []map[string]any{
		{
			"log_id":     10,
			"log_time":   "2026-11-21 12:00:00",
			"action":     "file_editor_saved",
			"user_type":  "wp_user",
			"user_login": "alice",
			"ip_address": "1.2.3.4",
			"log_data":   `{"file_path":"twentytwentyfour/functions.php","component_type":"theme","component_name":"twentytwentyfour"}`,
		},
		{
			"log_id":     11,
			"log_time":   "2026-11-21 12:05:00",
			"action":     "wpcode_snippet_created",
			"user_type":  "system_or_anonymous",
			"user_login": "",
			"ip_address": "5.6.7.8",
			"log_data":   `{"snippet_id":77,"snippet_title":"seo","snippet_type":"html","status":"publish","code_preview":"<script src=https://evil.invalid/s.js></script>"}`,
		},
		{
			"log_id":     12,
			"log_time":   "2026-11-21 12:10:00",
			"action":     "wpcode_snippet_updated",
			"user_type":  "wp_user",
			"user_login": "editor",
			"ip_address": "1.2.3.4",
			"log_data":   `{"snippet_id":12,"snippet_title":"Banner CSS","snippet_type":"css","status":"publish","code_preview":".banner { color: red; }"}`,
		},
		{
			"log_id":     13,
			"log_time":   "2026-11-21 12:15:00",
			"action":     "option_updated",
			"user_type":  "wp_user",
			"user_login": "alice",
			"ip_address": "1.2.3.4",
			"log_data":   `{"option_name":"admin_email","old_value_preview":"owner@example.com","new_value_preview":"attacker@evil.invalid"}`,
		},
		{
			"log_id":     14,
			"log_time":   "2026-11-21 12:20:00",
			"action":     "option_updated",
			"user_type":  "wp_user",
			"user_login": "alice",
			"ip_address": "1.2.3.4",
			"log_data":   `{"option_name":"users_can_register","old_value_preview":"0","new_value_preview":"1"}`,
		},
		{
			"log_id":     15,
			"log_time":   "2026-11-21 12:25:00",
			"action":     "file_editor_saved",
			"user_type":  "wp_cli",
			"user_login": "",
			"ip_address": "",
			"log_data":   `{"file_path":"cli-ignored.php","component_type":"plugin","component_name":"foo"}`,
		},
		{
			"log_id":     16,
			"log_time":   "2026-11-21 12:30:00",
			"action":     "app_password_created",
			"user_type":  "system_or_anonymous",
			"user_login": "",
			"ip_address": "9.9.9.9",
			"log_data":   `{"target_user_login":"admin","app_name":"persist"}`,
		},
	}
	raw, _ := json.Marshal(events)
	findings := securityLogEventFindings(string(raw))

	byPath := map[string]struct{ id, sev, hash, matched string }{}
	for _, f := range findings {
		byPath[f.Filename] = struct{ id, sev, hash, matched string }{f.SignatureID, f.Severity, f.ContentHash, f.MatchedText}
	}

	checks := []struct {
		path, wantID, wantSev string
	}{
		{"log:file-editor/twentytwentyfour/functions.php", "seclog-file-editor-saved", "high"},
		{"log:wpcode/77", "seclog-wpcode-snippet", "critical"},
		{"log:wpcode/12", "seclog-wpcode-snippet", "medium"},
		{"log:option/admin_email", "seclog-admin-email-changed", "high"},
		{"log:app-password/admin/persist", "seclog-app-password-created", "critical"},
	}
	for _, c := range checks {
		got, ok := byPath[c.path]
		if !ok {
			t.Errorf("missing finding for %s (got %v)", c.path, byPath)
			continue
		}
		if got.id != c.wantID || got.sev != c.wantSev {
			t.Errorf("%s: got %s/%s want %s/%s", c.path, got.id, got.sev, c.wantID, c.wantSev)
		}
		if len(got.hash) != 64 {
			t.Errorf("%s: expected 64-hex hash, got %q", c.path, got.hash)
		}
	}
	if _, ok := byPath["log:file-editor/cli-ignored.php"]; ok {
		t.Error("wp_cli events must be ignored")
	}
	if _, ok := byPath["log:option/users_can_register"]; ok {
		t.Error("users_can_register is handled by config state sync and must not emit a security-log finding")
	}
	if securityLogEventFindings("[]") != nil || securityLogEventFindings("bad") != nil {
		t.Error("empty or malformed input should return nil")
	}
}

func TestRuntimeEvalFindings(t *testing.T) {
	rows := []map[string]any{
		{"log": "~/logs/error.log", "file": "wp-content/themes/custom/header.php", "line": 42, "count": 7, "excerpt": "PHP Fatal error: ... in /www/x/public/wp-content/themes/custom/header.php(42) : eval()'d code on line 1"},
		{"log": "~/logs/error.log", "file": "wp-content/uploads/2026/05/cache.php", "line": 3, "count": 2, "excerpt": "PHP Warning: ... eval()'d code"},
		{"log": "~/logs/error.log", "file": "wp-content/plugins/mystery-plugin/init.php", "line": 99, "count": 1, "excerpt": "PHP Warning: ... eval()'d code"},
		{"log": "~/logs/error.log", "file": "wp-content/plugins/code-snippets/php/snippet-ops.php", "line": 582, "count": 4, "excerpt": "PHP Warning: ... eval()'d code"},
	}
	raw, _ := json.Marshal(rows)
	findings := runtimeEvalFindings(string(raw))
	got := map[string]string{}
	for _, f := range findings {
		got[f.Filename] = f.Severity
		if f.SignatureID != "runtime-eval-error" {
			t.Errorf("%s: bad sig %q", f.Filename, f.SignatureID)
		}
		if len(f.ContentHash) != 64 {
			t.Errorf("%s: expected 64-hex content hash", f.Filename)
		}
		if !strings.Contains(f.SignatureDescription, "eval()'d code") {
			t.Errorf("%s: bad description %q", f.Filename, f.SignatureDescription)
		}
	}
	want := map[string]string{
		"wp-content/themes/custom/header.php:42":             "critical",
		"wp-content/uploads/2026/05/cache.php:3":             "critical",
		"wp-content/plugins/mystery-plugin/init.php:99":      "high",
		"wp-content/plugins/code-snippets/php/snippet-ops.php:582": "low",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got severity %q want %q", k, got[k], v)
		}
	}
	if runtimeEvalFindings("[]") != nil || runtimeEvalFindings("bad") != nil {
		t.Error("empty or malformed input should return nil")
	}
}
