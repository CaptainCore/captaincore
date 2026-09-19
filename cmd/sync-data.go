package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/CaptainCore/captaincore/scan"
	"github.com/spf13/cobra"
)

var flagSyncDataJSON bool

var syncDataCmd = &cobra.Command{
	Use:   "sync-data <site|target>",
	Short: "Sync website data for one or more sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Count non-flag targets
		targetCount := 0
		for _, arg := range args {
			if !strings.HasPrefix(arg, "--") {
				targetCount++
			}
		}
		// Multiple targets or bulk targets go through bash
		if targetCount > 1 || (len(args) > 0 && (strings.HasPrefix(args[0], "@production") || strings.HasPrefix(args[0], "@staging") || strings.HasPrefix(args[0], "@all"))) {
			resolveCommand(cmd, args)
			return
		}
		resolveNativeOrWP(cmd, args, syncDataNative)
	},
}

// syncDataNative implements `captaincore sync-data <site>` natively in Go.
func syncDataNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])

	if !flagSyncDataJSON {
		fmt.Printf("Syncing %s...\n", args[0])
	}

	// Fetch site details via `captaincore site get`
	siteGetCmd := exec.Command("captaincore", "site", "get", args[0], "--captain-id="+captainID)
	siteGetOutput, err := siteGetCmd.Output()
	if err != nil || len(siteGetOutput) == 0 {
		fmt.Printf("Error: Site '%s' not found.\n", args[0])
		return
	}

	var siteDetails struct {
		SiteID uint `json:"site_id"`
	}
	if json.Unmarshal(siteGetOutput, &siteDetails) != nil {
		fmt.Printf("Error: Site '%s' not found.\n", args[0])
		return
	}

	// Run fetch-site-data script
	if !flagSyncDataJSON {
		fmt.Print("Fetching site data via SSH... ")
	}
	sshCmd := exec.Command("captaincore", "ssh", args[0], "--script=fetch-site-data", "--captain-id="+captainID)
	sshOutput, err := sshCmd.Output()
	if err != nil || len(sshOutput) == 0 {
		if !flagSyncDataJSON {
			fmt.Println("failed")
		}
		return
	}
	if !flagSyncDataJSON {
		fmt.Println("done")
	}

	// Parse response into key:value map (split on first colon)
	data := parseSiteData(string(sshOutput))

	// Find environment ID
	environments, err := models.FindEnvironmentsBySiteID(siteDetails.SiteID)
	if err != nil {
		return
	}
	var environmentID uint
	var matchedEnv models.Environment
	for _, env := range environments {
		if strings.EqualFold(env.Environment, sa.Environment) {
			environmentID = env.EnvironmentID
			matchedEnv = env
			break
		}
	}
	if environmentID == 0 {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		return
	}
	client := newAPIClient(system, captain)

	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Handle "WordPress not found" case
	if strings.TrimSpace(string(sshOutput)) == "WordPress not found" {
		environmentUpdate := map[string]interface{}{
			"environment_id": environmentID,
			"token":          "basic",
			"updated_at":     timeNow,
		}
		resp, err := client.Post("sync-data", map[string]interface{}{
			"site_id": siteDetails.SiteID,
			"data":    environmentUpdate,
		})
		if err == nil {
			if flagSyncDataJSON {
				fmt.Print(string(resp))
			} else {
				fmt.Println("WordPress not found")
			}
		}
		return
	}

	// Validate plugins and themes JSON
	var testJSON interface{}
	if data["plugins"] == "" || json.Unmarshal([]byte(data["plugins"]), &testJSON) != nil {
		fmt.Println("Response not valid")
		return
	}
	if data["themes"] == "" || json.Unmarshal([]byte(data["themes"]), &testJSON) != nil {
		fmt.Println("Response not valid")
		return
	}

	// Merge component hashes into plugins and themes JSON
	if hashJSON, ok := data["component_hashes"]; ok && hashJSON != "" {
		var hashMap map[string]string
		if json.Unmarshal([]byte(hashJSON), &hashMap) == nil {
			data["plugins"] = mergeComponentHashes(data["plugins"], hashMap)
			data["themes"] = mergeComponentHashes(data["themes"], hashMap)

			// Merge per-component mu-plugin hashes (mu: prefix) into mu_plugins JSON
			// and also into plugins array (WP-CLI includes must-use in plugins list)
			muHashMap := make(map[string]string)
			for k, v := range hashMap {
				if strings.HasPrefix(k, "mu:") {
					muHashMap[strings.TrimPrefix(k, "mu:")] = v
				}
			}
			if len(muHashMap) > 0 {
				if muJSON, ok := data["mu_plugins"]; ok && muJSON != "" && muJSON != "[]" {
					data["mu_plugins"] = mergeComponentHashes(muJSON, muHashMap)
				}
				data["plugins"] = mergeComponentHashes(data["plugins"], muHashMap)
			}
		}
	}

	environmentUpdate := map[string]interface{}{
		"environment_id":        environmentID,
		"plugins":               data["plugins"],
		"themes":                data["themes"],
		"core":                  data["core"],
		"home_url":              data["home_url"],
		"users":                 data["users"],
		"database_name":         data["database_name"],
		"database_username":     data["database_username"],
		"database_password":     data["database_password"],
		"core_verify_checksums": data["core_verify_checksums"],
		"subsite_count":         data["subsite_count"],
		"php_memory":            data["php_memory"],
		"token":                 data["token"],
		"updated_at":            timeNow,
	}

	// Load existing environment details and merge extra fields
	envRecord, err := models.GetEnvironmentByID(environmentID)
	if err != nil {
		return
	}

	var details map[string]interface{}
	if envRecord.Details != "" {
		json.Unmarshal([]byte(envRecord.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}

	// Fields stored in the details JSON
	detailKeys := []string{"default_role", "registration", "restic_cache", "php_version", "db_size", "wp_content"}
	for _, key := range detailKeys {
		if v, ok := data[key]; ok && v != "" {
			details[key] = v
		}
	}
	// JSON detail fields (parse before storing)
	jsonDetailKeys := []string{"core_checksum_details", "plugin_checksum_details", "security_log", "error_logs", "mu_plugin_files", "core_file_hashes", "loose_file_hashes", "capture_plugin_pages", "hidden_plugins", "media_payloads"}
	for _, key := range jsonDetailKeys {
		if v, ok := data[key]; ok && v != "" {
			var parsed interface{}
			if json.Unmarshal([]byte(v), &parsed) == nil {
				details[key] = parsed
			}
		}
	}

	// Plugins the site lists with plugin code disabled but not with it
	// enabled are hiding themselves (see fetch-site-data). The list alone
	// is not enough to email about: hiddenPluginsToAlert keeps the ones that
	// look like a backdoor and drops the known self-hiders and the cases
	// where the whole list vanished.
	if v := strings.TrimSpace(data["hidden_plugins"]); v != "" && v != "[]" {
		var hidden []string
		if json.Unmarshal([]byte(v), &hidden) == nil && len(hidden) > 0 {
			if site, err := sa.LookupSite(); err == nil && site != nil {
				quicksave := filepath.Join(system.Path, fmt.Sprintf("%s_%d", site.Site, site.SiteID), strings.ToLower(matchedEnv.Environment), "quicksave")
				alertable, skipped := hiddenPluginsToAlert(hidden, quicksave)
				if !flagSyncDataJSON {
					fmt.Printf("Hidden plugin(s) reported by the site: %s\n", strings.Join(hidden, ", "))
					for _, r := range skipped {
						fmt.Printf("  not alerting: %s\n", r)
					}
				}
				if len(alertable) > 0 {
					var findings []scan.LegacyFinding
					for _, h := range alertable {
						findings = append(findings, scan.LegacyFinding{
							Filename:             "wp-content/plugins/" + h,
							SignatureID:          "hidden-plugin",
							SignatureName:        "Plugin hidden from WordPress",
							SignatureDescription: fmt.Sprintf("%s is installed but disappears from the plugin list once plugin code runs, and its code filters the plugin list; self-hiding backdoors work this way", h),
						})
					}
					postMalwareAlert(site, &matchedEnv, system, captain, findings, "hidden-plugin")
				}
			}
		}
	}

	// Database scan (see fetch-site-data and lib/remote-scripts/db-scan): the
	// exported rows are judged here with the malware rules, the fixed checks
	// use the previous sync's user list, and the summary (no row contents)
	// goes into environment details.
	if v := strings.TrimSpace(data["db_scan"]); v != "" && v != "[]" {
		findings, summary, err := dbScanFindings(v, knownUserLogins(envRecord.Users), nativeAlertSeverity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Database scan: %v\n", err)
		} else {
			summary.At = timeNow
			details["db_scan"] = summary
			if !flagSyncDataJSON {
				fmt.Printf("  Database scan: %d option(s), %d post(s) exported; %d finding(s)\n", summary.Stats["options_exported"], summary.Stats["posts_exported"], len(findings))
			}
			if len(findings) > 0 {
				if site, err := sa.LookupSite(); err == nil && site != nil {
					postMalwareAlert(site, &matchedEnv, system, captain, findings, "db-scan")
				}
			}
		}
	}

	// Payloads stashed in recently changed media (see fetch-site-data): alert
	// on the critical and high rows the same way file findings alert.
	if v := strings.TrimSpace(data["media_payloads"]); v != "" && v != "[]" {
		var rows []struct {
			Severity, Type, Path, Detail string
		}
		if json.Unmarshal([]byte(v), &rows) == nil {
			var findings []scan.LegacyFinding
			for _, r := range rows {
				if r.Severity != "CRITICAL" && r.Severity != "HIGH" {
					continue
				}
				findings = append(findings, scan.LegacyFinding{
					Filename:             r.Path,
					SignatureID:          "media-" + strings.ToLower(r.Type),
					SignatureName:        "Media file: " + strings.ReplaceAll(strings.ToLower(r.Type), "_", " "),
					SignatureDescription: r.Detail,
				})
			}
			if len(findings) > 0 {
				if site, err := sa.LookupSite(); err == nil && site != nil {
					if !flagSyncDataJSON {
						fmt.Printf("Media payload finding(s): %d\n", len(findings))
					}
					postMalwareAlert(site, &matchedEnv, system, captain, findings, "media")
				}
			}
		}
	}

	// Store mu_plugins array (with per-component hashes) in details
	if muJSON, ok := data["mu_plugins"]; ok && muJSON != "" && muJSON != "[]" {
		var parsed interface{}
		if json.Unmarshal([]byte(muJSON), &parsed) == nil {
			details["mu_plugins"] = parsed
		}
	}

	detailsJSON, _ := json.Marshal(details)
	environmentUpdate["details"] = string(detailsJSON)

	// Update environment in DB
	if !flagSyncDataJSON {
		fmt.Print("Updating local database... ")
	}
	updates := map[string]interface{}{
		"plugins":               data["plugins"],
		"themes":                data["themes"],
		"core":                  data["core"],
		"home_url":              data["home_url"],
		"users":                 data["users"],
		"database_name":         data["database_name"],
		"database_username":     data["database_username"],
		"database_password":     data["database_password"],
		"core_verify_checksums": data["core_verify_checksums"],
		"subsite_count":         data["subsite_count"],
		"php_memory":            data["php_memory"],
		"token":                 data["token"],
		"details":               string(detailsJSON),
		"updated_at":            timeNow,
	}
	models.DB.Model(&models.Environment{}).Where("environment_id = ?", environmentID).Updates(updates)
	if !flagSyncDataJSON {
		fmt.Println("done")
	}

	// Post to API
	if !flagSyncDataJSON {
		fmt.Print("Syncing to API... ")
	}
	resp, err := client.Post("sync-data", map[string]interface{}{
		"site_id": siteDetails.SiteID,
		"data":    environmentUpdate,
	})
	if err == nil {
		if flagSyncDataJSON {
			fmt.Print(string(resp))
		} else {
			fmt.Println("done")
			syncDataPrintSummary(args[0], data)
		}
	} else if !flagSyncDataJSON {
		fmt.Println("failed")
	}

	// Post the session / privilege signal as a separate append-only session-snapshot
	// record (WP Registry compromise telemetry, Phase 1: store-only). Best-effort — a
	// failure here must never disrupt the core sync-data flow above. Sites running an
	// older fetch-site-data simply omit the key and this is skipped.
	if signal, ok := data["session_signal"]; ok && signal != "" && signal != "{}" {
		client.Post("session-snapshot", map[string]interface{}{
			"site_id":     siteDetails.SiteID,
			"environment": sa.Environment,
			"data":        signal,
		})
	}
}

// parseSiteData parses key:value lines from fetch-site-data into a map.
// Splits on the first colon only, so JSON values with colons are preserved.
// knownSelfHidingPlugins remove themselves from the plugin list by design:
// host-managed updaters and remote-management workers. Not backdoors.
var knownSelfHidingPlugins = map[string]string{
	"autoupdater":           "Flywheel Managed Plugin Updates hides itself by design",
	"worker":                "ManageWP Worker can be set to hide itself",
	"wpmudev-updates":       "WPMU DEV Dashboard white-label hides itself",
	"kinsta-mu-plugins":     "Kinsta must-use plugin",
	"captaincore-helper":    "CaptainCore helper",
	"captaincore-analytics": "CaptainCore analytics",
}

// hiddenPluginListCap: more than this many plugins missing from the list
// means the listing itself broke (a fatal in one plugin, a custom plugin
// directory, a multisite quirk), not that they are all hiding.
const hiddenPluginListCap = 5

// hiddenPluginsToAlert decides which reported hidden plugins deserve a
// malware alert. A plugin is alertable when it is not a known self-hider and
// its quicksave copy contains code that filters the plugin list (the only
// way a plugin can make itself disappear); everything else is printed and
// kept in environment details for review.
func hiddenPluginsToAlert(hidden []string, quicksave string) (alert []string, skipped []string) {
	if len(hidden) > hiddenPluginListCap {
		return nil, []string{fmt.Sprintf("%d plugins vanished from the list at once: the listing broke, not a hidden plugin", len(hidden))}
	}
	for _, h := range hidden {
		name := strings.TrimSuffix(h, ".php")
		if why, ok := knownSelfHidingPlugins[name]; ok {
			skipped = append(skipped, h+": "+why)
			continue
		}
		dir := filepath.Join(quicksave, "plugins", name)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			// Not in the quicksave tree (a drop-in file, a plugin outside
			// plugins/): nothing to corroborate with, alert as reported.
			alert = append(alert, h)
			continue
		}
		if dirMatches(dir, allPluginsFilter) {
			alert = append(alert, h)
		} else {
			skipped = append(skipped, h+": its code never hooks the plugin list, so the listing itself is inconsistent")
		}
	}
	return alert, skipped
}

// allPluginsFilter is the hook registration that lets a plugin edit the
// plugin list; "$all_plugins = get_plugins()" is ordinary and does not count.
var allPluginsFilter = regexp.MustCompile(`add_filter\s*\(\s*['"]all_plugins['"]`)

// dirMatches reports whether any PHP file under dir matches re.
func dirMatches(dir string, re *regexp.Regexp) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if scan.ExtOf(d.Name()) != ".php" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err == nil && re.Match(b) {
			found = true
		}
		return nil
	})
	return found
}

func parseSiteData(output string) map[string]string {
	data := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(line, ":")
		if found {
			data[key] = value
		}
	}
	return data
}

// syncDataPrintSummary prints a human-readable summary of the synced data.
func syncDataPrintSummary(site string, data map[string]string) {
	// Count plugins
	var plugins []interface{}
	json.Unmarshal([]byte(data["plugins"]), &plugins)

	// Count themes
	var themes []interface{}
	json.Unmarshal([]byte(data["themes"]), &themes)

	// Count users
	var users []interface{}
	json.Unmarshal([]byte(data["users"]), &users)

	checksumStatus := "pass"
	if data["core_verify_checksums"] == "0" {
		checksumStatus = "fail"
	}

	fmt.Printf("\n%s (%s)\n", site, data["home_url"])
	fmt.Printf("  WordPress %-8s Checksums %s\n", data["core"], checksumStatus)
	fmt.Printf("  %s plugins, %s themes, %s users\n",
		formatNumber(len(plugins)),
		formatNumber(len(themes)),
		formatNumber(len(users)),
	)
	fmt.Printf("  PHP memory %s\n", data["php_memory"])
}

// formatNumber adds comma separators to a number (e.g. 3641 -> "3,641").
func formatNumber(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

// mergeComponentHashes takes a JSON array of plugin/theme objects and a hash map,
// and injects the "hash" field into each object whose "name" matches a key in the map.
func mergeComponentHashes(componentsJSON string, hashMap map[string]string) string {
	var components []map[string]interface{}
	if json.Unmarshal([]byte(componentsJSON), &components) != nil {
		return componentsJSON
	}
	for i, component := range components {
		name, _ := component["name"].(string)
		if hash, ok := hashMap[name]; ok {
			components[i]["hash"] = hash
		}
	}
	merged, err := json.Marshal(components)
	if err != nil {
		return componentsJSON
	}
	return string(merged)
}

func init() {
	rootCmd.AddCommand(syncDataCmd)
	syncDataCmd.Flags().BoolVarP(&flagSkipScreenshot, "skip-screenshot", "c", false, "Skip screenshot")
	syncDataCmd.Flags().BoolVar(&flagSyncDataJSON, "json", false, "Output raw JSON response")
	syncDataCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 0, "Number of sites to run at same time")
}
