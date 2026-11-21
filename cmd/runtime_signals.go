package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
)

// Behavioral signals emitted by fetch-site-data:
//   - security_log_events: high-risk rows from wp_captaincore_security_log (last 26h,
//     excluding wp_cli) — live theme/plugin file editor saves, WPCode snippet saves,
//     header/footer option injections, admin email changes, application password creation,
//     and super-admin grants.
//   - runtime_eval_errors: PHP "eval()'d code" errors tailed from the site's error logs.

type securityLogEvent struct {
	LogID     int64           `json:"log_id"`
	LogTime   string          `json:"log_time"`
	Action    string          `json:"action"`
	UserType  string          `json:"user_type"`
	UserID    int64           `json:"user_id"`
	UserLogin string          `json:"user_login"`
	IPAddress string          `json:"ip_address"`
	LogData   json.RawMessage `json:"log_data"`
}

var suspiciousSnippetTokens = []string{
	"<script", "<iframe", "eval(", "base64_decode", "atob(", "fromcharcode",
	"gzinflate", "str_rot13", "shell_exec", "passthru", "system(", "exec(",
	"create_function", "assert(", "document.write", "window.location",
	"$_post", "$_get", "$_request", "$_cookie", "file_put_contents", "curl_exec",
}

func looksSuspiciousCode(s string) bool {
	lower := strings.ToLower(s)
	for _, tok := range suspiciousSnippetTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func parseEventData(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	// log_data may arrive as an object or as a JSON-encoded string from MySQL.
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		json.Unmarshal([]byte(s), &out)
		return out
	}
	json.Unmarshal(raw, &out)
	return out
}

func strField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func actorSummary(ev securityLogEvent) string {
	who := ev.UserLogin
	if who == "" {
		who = ev.UserType
	}
	if who == "" {
		who = "unknown"
	}
	if ev.UserType != "" && ev.UserLogin != "" && ev.UserType != "wp_user" {
		who += " (" + ev.UserType + ")"
	}
	if ev.IPAddress != "" {
		who += " from " + ev.IPAddress
	}
	if ev.LogTime != "" {
		who += " at " + ev.LogTime + " UTC"
	}
	return who
}

// securityLogEventFindings turns high-risk rows from wp_captaincore_security_log
// into security signals for Manager review (source "security-log").
// Any high-risk action occurring with user_type "system_or_anonymous" (no logged-in
// WordPress user) is escalated to critical so it lands on the priority attention list.
func securityLogEventFindings(raw string) []scan.LegacyFinding {
	var events []securityLogEvent
	if json.Unmarshal([]byte(raw), &events) != nil || len(events) == 0 {
		return nil
	}
	var out []scan.LegacyFinding
	seen := map[string]bool{}

	for _, ev := range events {
		if strings.EqualFold(ev.UserType, "wp_cli") {
			continue
		}
		anon := strings.EqualFold(ev.UserType, "system_or_anonymous")
		data := parseEventData(ev.LogData)
		actor := actorSummary(ev)

		var path, sigID, sigName, desc, matched, sev, hash string

		switch ev.Action {
		case "file_editor_saved":
			fp := strField(data, "file_path")
			if fp == "" {
				fp = "unknown"
			}
			compType := strField(data, "component_type")
			compName := strField(data, "component_name")
			path = "log:file-editor/" + fp
			sigID = "seclog-file-editor-saved"
			sigName = "Theme or plugin file edited in wp-admin"
			sev = "high"
			if anon {
				sev = "critical"
			}
			desc = fmt.Sprintf("File %s (%s %s) was saved through the WordPress file editor by %s.", fp, compType, compName, actor)
			matched = fmt.Sprintf("action=file_editor_saved file=%s component=%s:%s actor=%s", fp, compType, compName, actor)
			hash = subjectHash(sigID, fp, ev.LogTime, ev.UserLogin, ev.IPAddress)

		case "wpcode_snippet_created", "wpcode_snippet_updated":
			sid := strField(data, "snippet_id")
			title := strField(data, "snippet_title")
			stype := strField(data, "snippet_type")
			status := strField(data, "status")
			preview := strField(data, "code_preview")
			if sid == "" {
				sid = "0"
			}
			path = "log:wpcode/" + sid
			sigID = "seclog-wpcode-snippet"
			sigName = "WPCode snippet saved"
			sev = "medium"
			if looksSuspiciousCode(preview) || strings.EqualFold(stype, "php") {
				sev = "high"
			}
			if anon {
				sev = "critical"
			}
			desc = fmt.Sprintf("WPCode snippet #%s (%q, type=%s, status=%s) was saved by %s.", sid, title, stype, status, actor)
			matched = fmt.Sprintf("action=%s snippet=%s (%s) actor=%s\n%s", ev.Action, sid, title, actor, preview)
			hash = subjectHash(sigID, sid, ev.Action, ev.LogTime, preview)

		case "app_password_created":
			target := strField(data, "target_user_login")
			appName := strField(data, "app_name")
			if target == "" {
				target = "unknown"
			}
			path = "log:app-password/" + target + "/" + appName
			sigID = "seclog-app-password-created"
			sigName = "Application password created"
			sev = "medium"
			if anon {
				sev = "critical"
			}
			desc = fmt.Sprintf("Application password %q was created for user %s by %s.", appName, target, actor)
			matched = fmt.Sprintf("action=app_password_created target=%s app=%s actor=%s", target, appName, actor)
			hash = subjectHash(sigID, target, appName, ev.LogTime)

		case "user_super_admin_granted":
			target := strField(data, "target_user_login")
			if target == "" {
				target = "unknown"
			}
			path = "log:super-admin/" + target
			sigID = "seclog-super-admin-granted"
			sigName = "Multisite super admin granted"
			sev = "high"
			if anon {
				sev = "critical"
			}
			desc = fmt.Sprintf("Super admin privileges were granted to %s by %s.", target, actor)
			matched = fmt.Sprintf("action=user_super_admin_granted target=%s actor=%s", target, actor)
			hash = subjectHash(sigID, target, ev.LogTime)

		case "option_added", "option_updated":
			optName := strField(data, "option_name")
			preview := strField(data, "new_value_preview")
			if preview == "" {
				preview = strField(data, "value_preview")
			}
			oldPreview := strField(data, "old_value_preview")
			switch optName {
			case "ihaf_insert_header", "ihaf_insert_footer", "ihaf_insert_body":
				path = "log:option/" + optName
				sigID = "seclog-header-footer-inject"
				sigName = "Header/footer injection option modified"
				sev = "medium"
				if looksSuspiciousCode(preview) {
					sev = "high"
				}
				if anon {
					sev = "critical"
				}
				desc = fmt.Sprintf("Option %s was modified by %s.", optName, actor)
				matched = fmt.Sprintf("option=%s actor=%s\n%s", optName, actor, preview)
				hash = subjectHash(sigID, optName, ev.LogTime, preview)
			case "admin_email", "new_admin_email":
				path = "log:option/" + optName
				sigID = "seclog-admin-email-changed"
				sigName = "Site admin email modified"
				sev = "high"
				if anon {
					sev = "critical"
				}
				desc = fmt.Sprintf("Option %s changed (%s -> %s) by %s.", optName, oldPreview, preview, actor)
				matched = fmt.Sprintf("option=%s old=%s new=%s actor=%s", optName, oldPreview, preview, actor)
				hash = subjectHash(sigID, optName, ev.LogTime, preview)
			default:
				continue
			}

		default:
			continue
		}

		dedupeKey := path + "|" + sigID + "|" + hash
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true
		out = append(out, scan.LegacyFinding{
			Filename:             path,
			SignatureID:          sigID,
			SignatureName:        sigName,
			SignatureDescription: desc,
			MatchedText:          matched,
			Severity:             sev,
			Family:               "behavior",
			ContentHash:          hash,
		})
	}
	return out
}

type runtimeEvalError struct {
	Log     string `json:"log"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Count   int    `json:"count"`
	Excerpt string `json:"excerpt"`
}

// knownEvalPlugins legitimately evaluate user-supplied snippets, math formulas,
// or templates with PHP eval(). Errors from these paths are kept at low severity
// rather than alerting as high/critical compromises.
var knownEvalPlugins = []string{
	"code-snippets/",
	"insert-headers-and-footers/",
	"wpcode-premium/",
	"wp-all-import/",
	"wp-all-import-pro/",
	"wp-all-export/",
	"wp-all-export-pro/",
	"calculated-fields-form/",
	"gravityforms/",
	"formidable/",
	"formidable-pro/",
	"woody-ad-snippets/",
	"insert-php/",
	"insert-php-code-snippet/",
}

func evalErrorSeverity(filePath string) string {
	lower := strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(filePath, "\\", "/"), "/"))
	// Inside plugins/: check if it belongs to a known eval-using plugin.
	if idx := strings.Index(lower, "plugins/"); idx >= 0 {
		rel := lower[idx+len("plugins/"):]
		for _, known := range knownEvalPlugins {
			if strings.HasPrefix(rel, known) {
				return "low"
			}
		}
		return "high"
	}
	// eval()'d code in WordPress core, uploads, cache, themes, mu-plugins,
	// wp-config.php, or the web root is an extremely high-precision compromise tell.
	return "critical"
}

// runtimeEvalFindings turns PHP "eval()'d code" error-log entries collected by
// fetch-site-data into security signals (source "runtime-log").
func runtimeEvalFindings(raw string) []scan.LegacyFinding {
	var rows []runtimeEvalError
	if json.Unmarshal([]byte(raw), &rows) != nil || len(rows) == 0 {
		return nil
	}
	var out []scan.LegacyFinding
	seen := map[string]bool{}
	for _, r := range rows {
		file := strings.TrimSpace(r.File)
		if file == "" {
			continue
		}
		sev := evalErrorSeverity(file)
		path := fmt.Sprintf("%s:%d", file, r.Line)
		if r.Line <= 0 {
			path = file
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		count := r.Count
		if count <= 0 {
			count = 1
		}
		desc := fmt.Sprintf("PHP error log (%s) recorded %d eval()'d code error(s) originating from %s. Legitimate WordPress core, themes, and uploads do not execute eval()'d code.", r.Log, count, path)
		matched := r.Excerpt
		if matched == "" {
			matched = fmt.Sprintf("%s (%d hit(s) in %s)", path, count, r.Log)
		}
		out = append(out, scan.LegacyFinding{
			Filename:             path,
			SignatureID:          "runtime-eval-error",
			SignatureName:        "PHP eval()'d code error in server log",
			SignatureDescription: desc,
			MatchedText:          matched,
			Severity:             sev,
			Family:               "runtime",
			ContentHash:          subjectHash("runtime-eval-error", path, r.Log),
		})
	}
	return out
}
