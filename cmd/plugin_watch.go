package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CaptainCore/captaincore/scan"
)

// Security plugins switched off between two syncs. On one fleet site the
// SMILODON backdoor flipped WP 2FA from active to inactive by writing the
// active_plugins option directly, leaving no deactivation event anywhere;
// the nightly plugin list still recorded the change. A control that stops
// working is worth an alert on its own, whoever turned it off.
var securityPlugins = map[string]string{
	"wp-2fa":                              "WP 2FA",
	"two-factor":                          "Two-Factor",
	"wordfence":                           "Wordfence",
	"wordfence-login-security":            "Wordfence Login Security",
	"sucuri-scanner":                      "Sucuri Scanner",
	"better-wp-security":                  "Solid Security",
	"solid-security":                      "Solid Security",
	"all-in-one-wp-security-and-firewall": "All In One WP Security",
	"wp-cerber":                           "WP Cerber",
	"limit-login-attempts-reloaded":       "Limit Login Attempts Reloaded",
	"loginizer":                           "Loginizer",
	"wp-simple-firewall":                  "Shield Security",
	"miniorange-2-factor-authentication":  "miniOrange 2FA",
	"google-authenticator":                "Google Authenticator",
	"wp-security-audit-log":               "WP Activity Log",
	"simple-history":                      "Simple History",
	"really-simple-ssl":                   "Really Simple Security",
	"malcare-security":                    "MalCare",
	"jetpack-protect":                     "Jetpack Protect",
	"passwords-evolved":                   "Passwords Evolved",
}

type pluginRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// securityPluginChanges compares the previous and current synced plugin
// lists and reports every security plugin that was active before and is
// inactive or gone now.
func securityPluginChanges(prevJSON, curJSON string) []scan.LegacyFinding {
	var prev, cur []pluginRow
	if json.Unmarshal([]byte(prevJSON), &prev) != nil || json.Unmarshal([]byte(curJSON), &cur) != nil || len(prev) == 0 || len(cur) == 0 {
		return nil
	}
	now := map[string]string{}
	for _, p := range cur {
		now[p.Name] = p.Status
	}
	var out []scan.LegacyFinding
	for _, p := range prev {
		title, watched := securityPlugins[p.Name]
		if !watched || p.Status != "active" {
			continue
		}
		status, present := now[p.Name]
		if present && status == "active" {
			continue
		}
		what := "deactivated"
		if !present {
			what = "removed"
		}
		out = append(out, scan.LegacyFinding{
			Filename:             "plugin:" + p.Name,
			SignatureID:          "security-plugin-" + what,
			SignatureName:        title + " " + what,
			SignatureDescription: fmt.Sprintf("%s was active at the previous sync and is %s now. A backdoor that writes active_plugins directly leaves no deactivation event; confirm a person did this.", title, strings.TrimSuffix(what, "d")+"d"),
			MatchedText:          p.Name,
			Severity:             "high",
			Family:               "control",
		})
	}
	return out
}
