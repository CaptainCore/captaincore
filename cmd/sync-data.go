package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
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
	for _, env := range environments {
		if strings.EqualFold(env.Environment, sa.Environment) {
			environmentID = env.EnvironmentID
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
	detailKeys := []string{"default_role", "registration", "restic_cache", "php_version", "db_size"}
	for _, key := range detailKeys {
		if v, ok := data[key]; ok && v != "" {
			details[key] = v
		}
	}
	// JSON detail fields (parse before storing)
	jsonDetailKeys := []string{"core_checksum_details", "plugin_checksum_details", "security_log", "error_logs", "mu_plugin_files", "core_file_hashes", "loose_file_hashes"}
	for _, key := range jsonDetailKeys {
		if v, ok := data[key]; ok && v != "" {
			var parsed interface{}
			if json.Unmarshal([]byte(v), &parsed) == nil {
				details[key] = parsed
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
}

// parseSiteData parses key:value lines from fetch-site-data into a map.
// Splits on the first colon only, so JSON values with colons are preserved.
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
