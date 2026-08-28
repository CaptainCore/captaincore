package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/CaptainCore/captaincore/providers"
	"github.com/spf13/cobra"
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Site commands",
}

var siteDeployKeysCmd = &cobra.Command{
	Use:   "deploy-keys <site>",
	Short: "Deploy keys to a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <site>",
	Short: "Delete a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteDeleteNative)
	},
}

var siteSearchCmd = &cobra.Command{
	Use:   "search <search-term>",
	Short: "Search for sites",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteSearchNative)
	},
}

var getCmd = &cobra.Command{
	Use:   "get <site>",
	Short: "Get details about a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteGetNative)
	},
}

var listCmd = &cobra.Command{
	Use:     "list [<@target> [--provider=<provider>] [--filter=<theme|plugin|core>] [--filter-name=<name>] [--filter-version=<version>] [--filter-status=<active|inactive|dropin|must-use>] [--field=<field>]",
	Example: "captaincore site list @production.updates-off --provider=kinsta",
	Short:   "List sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteListNative)
	},
}

var flagOrphansWriteList, flagOrphansFromList string
var flagOrphansIncludeStaleNames bool

var siteOrphansCmd = &cobra.Command{
	Use:   "orphans",
	Short: "Lists local site folders that do not match an active site",
	Long: `Scans the CaptainCore data path for site folders ({site}_{id}) that are not
tied to an active site in the local database. Useful for reclaiming disk space left
behind after sites are deleted or renamed.

Safety:
  - Only folders whose trailing site_id is NOT active are candidates (renamed
    active sites with a leftover old folder name are reported separately and
    skipped unless --include-stale-names).
  - Dry-run by default. --confirm re-scans and re-checks each folder before delete.
  - Prefer: dry-run with --write-list=FILE, review, then
    --confirm --from-list=FILE so only the reviewed set can be removed.
  - Deletes never leave system.path (path confinement).

Sync the local DB first (captaincore connect --sync) so the active set is current.`,
	Example: `  captaincore site orphans
  captaincore site orphans --write-list=/tmp/orphans.txt
  captaincore site orphans --confirm --from-list=/tmp/orphans.txt`,
	Run: func(cmd *cobra.Command, args []string) {
		siteOrphansNative(cmd, args)
	},
}

var keyGenerateCmd = &cobra.Command{
	Use:   "key-generate <site>",
	Short: "Generates SFTP/SSH Rclone configs",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var siteCopyProductionToStaging = &cobra.Command{
	Use:   "copy-to-staging <site>",
	Short: "Copy production to staging (Kinsta only)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}
var siteCopyStagingToProduction = &cobra.Command{
	Use:   "copy-to-production <site>",
	Short: "Copy staging to production (Kinsta only)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var siteStatsGenerateCmd = &cobra.Command{
	Use:   "stats-generate <site>",
	Short: "Generates Fathom tracker",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteStatsGenerateNative)
	},
}

var siteDeployDefaultsCmd = &cobra.Command{
	Use:   "deploy-defaults <site>",
	Short: "Deploy default plugins/themes/settings",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteDeployDefaultsNative)
	},
}

var sitePrepareCmd = &cobra.Command{
	Use:   "prepare <site> [--skip-deployment]",
	Short: "Preps new site configurations",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var sshFailCmd = &cobra.Command{
	Use:   "ssh-fail <site>",
	Short: "Flag site with SSH failure",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteSSHFailNative)
	},
}

var sshRefreshCmd = &cobra.Command{
	Use:   "ssh-refresh <site>",
	Short: "Refresh SSH credentials from hosting provider",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteSSHRefreshNative)
	},
}

var siteVulnScanCmd = &cobra.Command{
	Use:   "vuln-scan <site>",
	Short: "Run vulnerability scan on a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteVulnScanNative)
	},
}

var syncSiteCmd = &cobra.Command{
	Use:   "sync <site-id>",
	Short: "Sync site details with CaptainCore CLI",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, siteSyncNative)
	},
}

var syncBatchSiteCmd = &cobra.Command{
	Use:   "sync-batch <site-id>...",
	Short: "Sync multiple sites sequentially with CaptainCore CLI",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires at least one <site-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

// siteGetNative implements `captaincore site get <site>` natively in Go.
// Output must match site-get.php exactly for --bash, --field, and JSON formats.
func siteGetNative(cmd *cobra.Command, args []string) {
	siteArg := args[0]
	environment := ""
	provider := ""

	// Parse site-environment format (e.g. "mysite-staging")
	if strings.Contains(siteArg, "-") {
		parts := strings.SplitN(siteArg, "-", 2)
		siteArg = parts[0]
		environment = parts[1]
	}

	// Parse site@provider format
	if strings.Contains(siteArg, "@") {
		parts := strings.SplitN(siteArg, "@", 2)
		siteArg = parts[0]
		provider = parts[1]
	}
	if strings.Contains(environment, "@") {
		parts := strings.SplitN(environment, "@", 2)
		environment = parts[0]
		provider = parts[1]
	}

	// Look up the site
	var site *models.Site
	var err error

	if id, parseErr := strconv.ParseUint(siteArg, 10, 64); parseErr == nil {
		site, err = models.GetSiteByID(uint(id))
	} else if provider != "" {
		site, err = models.GetSiteByNameAndProvider(siteArg, provider)
	} else {
		site, err = models.GetSiteByName(siteArg)
	}

	if err != nil || site == nil {
		return // Match PHP behavior: return empty on not found
	}

	// Fetch environments
	environments, err := models.FindEnvironmentsBySiteID(site.SiteID)
	if err != nil || len(environments) == 0 {
		return
	}

	// Default to Production
	if environment == "" {
		environment = "Production"
	}

	// Find matching environment (case-insensitive)
	var env *models.Environment
	for i, e := range environments {
		if strings.EqualFold(e.Environment, environment) {
			env = &environments[i]
			break
		}
	}
	if env == nil {
		return // Environment not found
	}

	// Parse site details JSON
	siteDetails := site.ParseDetails()
	envDetails := env.ParseDetails()

	// Build environment_vars string
	wpContent := "wp-content"
	environmentVars := ""
	if siteDetails.EnvironmentVars != nil && string(siteDetails.EnvironmentVars) != "" && string(siteDetails.EnvironmentVars) != `""` && string(siteDetails.EnvironmentVars) != "null" {
		var envVarsList []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal(siteDetails.EnvironmentVars, &envVarsList) == nil && len(envVarsList) > 0 {
			var parts []string
			for _, item := range envVarsList {
				parts = append(parts, fmt.Sprintf("%s='%s'", item.Key, item.Value))
				if item.Key == "STACKED_ID" || item.Key == "STACKED_SITE_ID" {
					wpContent = "content/" + item.Value
				}
			}
			environmentVars = "export " + strings.Join(parts, " ")
		}
	}

	// Parse fathom from environment details
	fathomStr := ""
	if envDetails.Fathom != nil && string(envDetails.Fathom) != "null" && string(envDetails.Fathom) != "" {
		fathomStr = string(envDetails.Fathom)
	}

	// Parse capture_pages
	capturePages := env.CapturePages

	// Plugin-discovered pages (cached in env.Details, refreshed by sync-data)
	capturePluginPages := envDetails.CapturePluginPages

	// Parse updates_exclude fields
	updatesExcludeThemes := env.UpdatesExcludeThemes
	updatesExcludePlugins := env.UpdatesExcludePlugins

	// Fetch account defaults
	defaultsStr := "[]"
	defaultUsersStr := "[]"
	if site.AccountID > 0 {
		account, err := models.GetAccountByID(site.AccountID)
		if err == nil && account != nil && account.Defaults != "" {
			defaultsStr = account.Defaults
			var defaults struct {
				Users json.RawMessage `json:"users"`
			}
			if json.Unmarshal([]byte(account.Defaults), &defaults) == nil && defaults.Users != nil {
				usersVal := string(defaults.Users)
				// PHP outputs "[]" for falsy/empty values; match that behavior
				if usersVal != "false" && usersVal != "null" && usersVal != "" {
					defaultUsersStr = usersVal
				}
			}
		}
	}

	// Build the output array (matches PHP $array)
	monitorEnabled := 0
	if env.MonitorEnabled == "1" || env.MonitorEnabled == "true" {
		monitorEnabled = 1
	}
	updatesEnabled := env.UpdatesEnabled
	if updatesEnabled == "" || updatesEnabled == "false" {
		updatesEnabled = "0"
	}

	array := map[string]interface{}{
		"site_id":                 site.SiteID,
		"site":                    site.Site,
		"status":                  site.Status,
		"provider":                site.Provider,
		"key":                     siteDetails.Key,
		"environment_vars":        environmentVars,
		"name":                    site.Name,
		"home_url":                env.HomeURL,
		"defaults":                json.RawMessage(defaultsStr),
		"fathom":                  fathomStr,
		"wp_content":              wpContent,
		"capture_pages":           capturePages,
		"capture_plugin_pages":    capturePluginPages,
		"address":                 env.Address,
		"username":                env.Username,
		"password":                env.Password,
		"protocol":                env.Protocol,
		"port":                    env.Port,
		"home_directory":          env.HomeDirectory,
		"database_username":       env.DatabaseUsername,
		"database_password":       env.DatabasePassword,
		"monitor_enabled":         monitorEnabled,
		"updates_enabled":         updatesEnabled,
		"updates_exclude_themes":  updatesExcludeThemes,
		"updates_exclude_plugins": updatesExcludePlugins,
	}

	// Determine format
	format := "json"
	if flagBash {
		format = "bash"
	}
	if flagFormat != "" {
		format = flagFormat
	}

	// Handle --field
	if flagField != "" {
		if val, ok := array[flagField]; ok {
			fmt.Print(val)
		}
		return
	}

	// JSON output
	if format == "json" {
		// Build ordered JSON matching PHP's JSON_PRETTY_PRINT output
		output := buildSiteGetJSON(array)
		fmt.Print(output)
		return
	}

	// Bash output
	if format == "bash" {
		// Convert capture_pages to CSV for bash format
		capturePagesCSV := ""
		if capturePages != "" {
			var pages []struct {
				Page string `json:"page"`
			}
			if json.Unmarshal([]byte(capturePages), &pages) == nil {
				var pageStrs []string
				for _, p := range pages {
					pageStrs = append(pageStrs, p.Page)
				}
				capturePagesCSV = strings.Join(pageStrs, ",")
			}
		}

		// Plugin-discovered pages → CSV
		capturePluginPagesCSV := strings.Join(capturePluginPages, ",")

		// Handle fathom for bash format
		fathomBash := fathomStr
		if fathomBash != "" && fathomBash != "null" {
			var fathomArr []struct {
				Domain string `json:"domain"`
				Code   string `json:"code"`
			}
			if json.Unmarshal([]byte(fathomBash), &fathomArr) == nil {
				if len(fathomArr) == 0 || fathomArr[0].Domain == "" || fathomArr[0].Code == "" {
					fathomBash = ""
				}
			}
		}
		if fathomBash == "null" {
			fathomBash = ""
		}

		// Handle auth from environment details
		authStr := ""
		if envDetails.Auth != nil && envDetails.Auth.Username != "" {
			authStr = base64.StdEncoding.EncodeToString(
				[]byte(envDetails.Auth.Username + ":" + envDetails.Auth.Password))
		}

		// Handle updates_exclude for bash (already CSV in DB)
		excludeThemes := updatesExcludeThemes
		excludePlugins := updatesExcludePlugins

		// Backup settings
		backupActive := "1"
		backupInterval := "daily"
		backupMode := "direct"
		if siteDetails.BackupSettings != nil {
			if siteDetails.BackupSettings.Active {
				backupActive = "1"
			} else {
				backupActive = "0"
			}
			backupInterval = siteDetails.BackupSettings.Interval
			backupMode = siteDetails.BackupSettings.Mode
		}

		bash := fmt.Sprintf(`site_id=%d
domain=%s
key=%s
fathom=%s
capture_pages=%s
capture_plugin_pages=%s
site=%s
auth=%s
environment_vars=%s
wp_content=%s
status=%s
provider=%s
default_users=%s
home_url=%s
address=%s
username=%s
protocol=%s
port=%s
home_directory=%s
database_username=%s
database_password=%s
updates_enabled=%s
updates_exclude_themes=%s
updates_exclude_plugins=%s
backup_active=%s
backup_interval=%s
backup_mode=%s`,
			site.SiteID,
			site.Name,
			siteDetails.Key,
			fathomBash,
			capturePagesCSV,
			capturePluginPagesCSV,
			site.Site,
			authStr,
			environmentVars,
			wpContent,
			site.Status,
			site.Provider,
			defaultUsersStr,
			env.HomeURL,
			env.Address,
			env.Username,
			env.Protocol,
			env.Port,
			env.HomeDirectory,
			env.DatabaseUsername,
			env.DatabasePassword,
			updatesEnabled,
			excludeThemes,
			excludePlugins,
			backupActive,
			backupInterval,
			backupMode,
		)
		fmt.Print(bash)
	}
}

// buildSiteGetJSON produces JSON output matching PHP's json_encode with JSON_PRETTY_PRINT.
// We use an ordered approach to match the PHP key ordering.
func buildSiteGetJSON(data map[string]interface{}) string {
	// PHP json_encode preserves insertion order. We replicate that order.
	keys := []string{
		"site_id", "site", "status", "provider", "key", "environment_vars",
		"name", "home_url", "defaults", "fathom", "wp_content", "capture_pages",
		"capture_plugin_pages",
		"address", "username", "password", "protocol", "port", "home_directory",
		"database_username", "database_password", "monitor_enabled",
		"updates_enabled", "updates_exclude_themes", "updates_exclude_plugins",
	}

	var b strings.Builder
	b.WriteString("{\n")
	for i, k := range keys {
		val := data[k]
		jsonVal, _ := json.Marshal(val)

		// Special handling for defaults and fathom (already JSON strings)
		if k == "defaults" {
			if raw, ok := val.(json.RawMessage); ok {
				jsonVal = raw
			}
		}
		if k == "fathom" {
			if s, ok := val.(string); ok {
				if s == "" || s == "null" {
					jsonVal = []byte(`""`)
				} else {
					jsonVal = []byte(s)
				}
			}
		}
		if k == "capture_pages" {
			if s, ok := val.(string); ok && s != "" {
				jsonVal = []byte(s)
			} else {
				jsonVal = []byte(`""`)
			}
		}
		if k == "capture_plugin_pages" {
			if pages, ok := val.([]string); !ok || pages == nil {
				jsonVal = []byte(`[]`)
			}
		}

		b.WriteString(fmt.Sprintf("    %q: %s", k, string(jsonVal)))
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String()
}

// siteListNative implements `captaincore site list <target>` natively in Go.
// Output is space-separated site-environment strings matching site-list.php exactly.
func siteListNative(cmd *cobra.Command, args []string) {
	target := args[0]

	// Parse target string
	environment, minorTargets := models.ParseTargetString(target)

	// Build query arguments
	queryArgs := models.FetchSiteMatchingArgs{
		Environment: environment,
		Provider:    flagProvider,
		Field:       flagField,
		Targets:     minorTargets,
	}

	// Handle filter flags
	if flagFilter != "" {
		if flagFilter != "core" && flagFilter != "plugins" && flagFilter != "themes" {
			fmt.Print("Error: `--filter` can only be set to core, themes or plugins.")
			return
		}
		queryArgs.Filter = &models.SiteFilter{
			Type:    flagFilter,
			Name:    flagFilterName,
			Version: flagFilterVersion,
			Status:  flagFilterStatus,
		}
	}

	results, err := models.FetchSitesMatching(queryArgs)
	if err != nil {
		return
	}

	// Build output
	var output []string
	for _, r := range results {
		envLower := strings.ToLower(r.Environment)
		toAdd := fmt.Sprintf("%s-%s", r.Site, envLower)

		if flagField != "" {
			toAdd = getFieldValue(r, flagField)
			// Handle comma-separated fields
			if strings.Contains(flagField, ",") {
				fields := strings.Split(flagField, ",")
				var values []string
				for _, f := range fields {
					values = append(values, getFieldValue(r, strings.TrimSpace(f)))
				}
				toAdd = strings.Join(values, ",")
			}
		}

		if toAdd == "" {
			continue
		}
		output = append(output, toAdd)
	}

	// Unique and sorted (matches PHP array_unique + asort)
	output = uniqueStrings(output)
	sort.Strings(output)

	fmt.Print(strings.Join(output, " "))
}

// getFieldValue extracts a field value from a SiteEnvironmentResult by field name.
func getFieldValue(r models.SiteEnvironmentResult, field string) string {
	switch field {
	case "site":
		return r.Site
	case "ids", "site_id":
		return strconv.FormatUint(uint64(r.SiteID), 10)
	case "domain", "name":
		return r.Name
	case "environment":
		return r.Environment
	case "provider":
		return r.Provider
	case "home_url":
		return r.HomeURL
	case "address":
		return r.Address
	case "username":
		return r.Username
	case "port":
		return r.Port
	case "core":
		return r.Core
	case "storage":
		return r.Storage
	case "visits":
		return r.Visits
	case "home_directory":
		return r.HomeDirField
	case "database_username":
		return r.DatabaseUsername
	case "database_password":
		return r.DatabasePassword
	case "updates_enabled":
		return r.UpdatesEnabled
	case "monitor_enabled":
		return r.MonitorEnabled
	case "updates_exclude_themes":
		return r.UpdatesExcludeThemes
	case "updates_exclude_plugins":
		return r.UpdatesExcludePlugins
	}
	return ""
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// siteSyncNative implements `captaincore site sync <site-id>` natively in Go.
func siteSyncNative(cmd *cobra.Command, args []string) {
	siteIDStr := args[0]
	siteID, err := strconv.ParseUint(siteIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid site_id '%s'\n", siteIDStr)
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)

	if flagDebug {
		resp, err := client.PostSiteGetRaw(uint(siteID))
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return
		}
		var pretty interface{}
		if json.Unmarshal(resp, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "    ")
			fmt.Println(string(out))
		}
		return
	}

	resp, err := client.PostSiteGetRaw(uint(siteID))
	if err != nil {
		fmt.Printf("Error fetching site: %s\n", err)
		return
	}

	var wrapper struct {
		Site json.RawMessage `json:"site"`
	}
	if json.Unmarshal(resp, &wrapper) != nil || wrapper.Site == nil {
		fmt.Println("Error: Invalid API response")
		return
	}

	// Upsert site
	var siteData models.Site
	if json.Unmarshal(wrapper.Site, &siteData) != nil {
		fmt.Println("Error: Could not parse site data")
		return
	}

	existingSite, _ := models.GetSiteByID(siteData.SiteID)
	if existingSite == nil {
		fmt.Printf("Added site #%d\n", siteData.SiteID)
	} else {
		fmt.Printf("Updating site #%d\n", siteData.SiteID)
	}
	models.UpsertSite(siteData)

	// Parse environments and shared_with from within the site object
	var siteNested struct {
		Environments []json.RawMessage `json:"environments"`
		SharedWith   []json.RawMessage `json:"shared_with"`
	}
	json.Unmarshal(wrapper.Site, &siteNested)

	// Upsert environments
	var envIDs []uint
	for _, envRaw := range siteNested.Environments {
		var envData models.Environment
		if json.Unmarshal(envRaw, &envData) != nil {
			continue
		}
		envIDs = append(envIDs, envData.EnvironmentID)
		models.UpsertEnvironment(envData, true)
	}

	// Upsert shared_with (account_site records)
	for _, asRaw := range siteNested.SharedWith {
		var asData models.AccountSite
		if json.Unmarshal(asRaw, &asData) != nil {
			continue
		}
		models.UpsertAccountSite(asData)
	}

	// Delete environments not in API response
	currentEnvs, _ := models.FindEnvironmentsBySiteID(siteData.SiteID)
	for _, currentEnv := range currentEnvs {
		found := false
		for _, id := range envIDs {
			if currentEnv.EnvironmentID == id {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Removing environment %d\n", currentEnv.EnvironmentID)
			models.DeleteEnvironmentByID(currentEnv.EnvironmentID)
		}
	}

	// Generate rclone keys
	keyGenCmd := exec.Command("captaincore", "site", "key-generate", siteIDStr, "--captain-id="+captainID)
	keyGenCmd.Stdout = os.Stdout
	keyGenCmd.Stderr = os.Stderr
	keyGenCmd.Run()

	// Update extras if flag set
	if flagUpdateExtras {
		prepareCmd := exec.Command("captaincore", "site", "prepare", siteIDStr, "--captain-id="+captainID)
		prepareCmd.Stdout = os.Stdout
		prepareCmd.Stderr = os.Stderr
		prepareCmd.Run()

		deployCmd := exec.Command("captaincore", "site", "deploy-defaults", siteIDStr+"-production", "--global-only", "--captain-id="+captainID)
		deployCmd.Stdout = os.Stdout
		deployCmd.Stderr = os.Stderr
		deployCmd.Run()

		captureCmd := exec.Command("captaincore", "capture", "generate", siteIDStr+"-production", "--captain-id="+captainID)
		captureCmd.Stdout = os.Stdout
		captureCmd.Stderr = os.Stderr
		captureCmd.Run()
	}
}

// siteVulnScanNative implements `captaincore site vuln-scan <site>` natively in Go.
func siteVulnScanNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	// --cached: display stored results without re-scanning
	if flagCached {
		displayVulnResults(env, site.Site)
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	sitePath := filepath.Join(system.Path, siteDir, envName, "quicksave")

	fmt.Printf("Running Wordfence scan %s %s environment\n", site.Site, env.Environment)

	// Run wordfence vuln-scan
	scanCmd := exec.Command("bash", "-c", fmt.Sprintf(
		"if [ -d %s ]; then cd %s; wordfence vuln-scan --plugin-directory plugins/ --theme-directory themes/ --output-format csv --output-headers --no-banner --quiet 2>/dev/null; fi",
		sitePath, sitePath))
	scanOutput, _ := scanCmd.Output()
	responseStr := strings.TrimSpace(string(scanOutput))

	if responseStr == "" {
		fmt.Println("Discovered 0 vulnerabilities")
		updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
			"vuln_scan": []interface{}{},
		}, system, captain)
		return
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(responseStr))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 1 {
		fmt.Println("Discovered 0 vulnerabilities")
		updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
			"vuln_scan": []interface{}{},
		}, system, captain)
		return
	}

	headers := records[0]
	var data []map[string]string
	for _, row := range records[1:] {
		entry := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				entry[header] = row[i]
			}
		}
		data = append(data, entry)
	}

	fmt.Printf("Discovered %d vulnerabilities\n", len(data))

	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
		"vuln_scan": data,
	}, system, captain)

	// Re-fetch environment to get updated details
	env, _ = models.GetEnvironmentByID(env.EnvironmentID)
	displayVulnResults(env, site.Site)
}

// displayVulnResults reads the vuln_scan key from environment details and prints a formatted table.
func displayVulnResults(env *models.Environment, siteName string) {
	var details map[string]json.RawMessage
	if env.Details == "" {
		fmt.Println("No vulnerability data found.")
		return
	}
	if err := json.Unmarshal([]byte(env.Details), &details); err != nil {
		fmt.Println("No vulnerability data found.")
		return
	}
	raw, ok := details["vuln_scan"]
	if !ok {
		fmt.Println("No vulnerability data found.")
		return
	}

	var vulns []map[string]string
	if err := json.Unmarshal(raw, &vulns); err != nil {
		fmt.Println("No vulnerability data found.")
		return
	}

	if len(vulns) == 0 {
		fmt.Printf("\nVulnerabilities for %s (%s)\n\nNo vulnerabilities found.\n", siteName, env.Environment)
		return
	}

	// Sort by CVSS score descending (critical first)
	sort.Slice(vulns, func(i, j int) bool {
		scoreI, _ := strconv.ParseFloat(vulns[i]["cvss_score"], 64)
		scoreJ, _ := strconv.ParseFloat(vulns[j]["cvss_score"], 64)
		return scoreI > scoreJ
	})

	fmt.Printf("\nVulnerabilities for %s (%s)\n\n", siteName, env.Environment)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SEVERITY\tCVE\tSOFTWARE\tTITLE\tPATCHED")

	for _, v := range vulns {
		severity := v["cvss_rating"]
		if severity == "" {
			severity = "-"
		}
		cve := v["cve"]
		if cve == "" {
			cve = "-"
		}
		software := v["slug"]
		if ver := v["version"]; ver != "" {
			software += " " + ver
		}
		title := v["title"]
		if len(title) > 55 {
			title = title[:52] + "..."
		}
		patched := v["patched"]
		switch strings.ToLower(patched) {
		case "true", "1":
			patched = "Yes"
		case "false", "0":
			patched = "No"
		case "":
			patched = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", severity, cve, software, title, patched)
	}
	w.Flush()
}

// siteSSHFailNative implements `captaincore site ssh-fail <site>` natively in Go.
func siteSSHFailNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.", sa.SiteName)
		return
	}

	// Update site details with connection_errors
	var details map[string]interface{}
	if site.Details != "" {
		json.Unmarshal([]byte(site.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}
	details["connection_errors"] = "SSH failed"

	updatedDetails, _ := json.Marshal(details)
	site.Details = string(updatedDetails)
	models.DB.Model(site).Update("details", site.Details)

	// Post to API
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		return
	}

	client := newAPIClient(system, captain)
	siteUpdate := map[string]interface{}{
		"site_id": site.SiteID,
		"details": site.Details,
	}
	client.Post("update-site", map[string]interface{}{
		"site_id": site.SiteID,
		"data":    siteUpdate,
	})
}

// siteSSHRefreshNative implements `captaincore site ssh-refresh <site>` natively in Go.
// It fetches fresh SSH credentials from the hosting provider API and updates both the
// local CLI database and the Manager if any credentials have changed.
func siteSSHRefreshNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		os.Exit(1)
	}

	// Check if site has a provider linked
	if site.Provider == "" {
		fmt.Println("No provider linked to this site.")
		os.Exit(1)
	}
	if site.ProviderSiteID == "" {
		fmt.Printf("No provider_site_id set for site '%s'. Run a site sync or set it manually.\n", sa.SiteName)
		os.Exit(1)
	}

	// Parse ProviderID — default to 1 if empty (matches Manager behavior)
	providerIDStr := site.ProviderID
	providerIDDefaulted := false
	if providerIDStr == "" {
		providerIDStr = "1"
		providerIDDefaulted = true
	}
	providerID, err := strconv.ParseUint(providerIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid provider_id '%s' on site.\n", site.ProviderID)
		os.Exit(1)
	}

	p, err := models.GetProviderByID(uint(providerID))
	if err != nil {
		fmt.Printf("Error: Provider #%d not found.\n", providerID)
		os.Exit(1)
	}

	// Get provider implementation and credentials
	hp, err := providers.Get(p.Provider)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	creds := p.GetCredentialsMap()

	// Look up current environment
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Printf("Error: Environment '%s' not found for site '%s'.\n", sa.Environment, sa.SiteName)
		os.Exit(1)
	}

	// Fetch fresh SSH credentials from provider API
	remoteSite := providers.RemoteSite{
		RemoteID:    site.ProviderSiteID,
		Name:        site.Name,
		Environment: sa.Environment,
	}
	enriched, err := hp.EnrichSite(creds, remoteSite)
	if err != nil {
		fmt.Printf("Error fetching credentials from provider '%s' (#%d): %v\n", p.Name, providerID, err)
		if providerIDDefaulted {
			fmt.Println("Note: This site has no provider_id set — defaulted to provider #1. Try running 'captaincore connect --sync' to update site data from Manager.")
		}
		os.Exit(1)
	}

	// Compare enriched values with current environment
	changes := make(map[string]interface{})
	var changedFields []string

	if enriched.SSHAddress != "" && enriched.SSHAddress != env.Address {
		changes["address"] = enriched.SSHAddress
		changedFields = append(changedFields, fmt.Sprintf("address: %s -> %s", env.Address, enriched.SSHAddress))
	}
	if enriched.SSHPort != "" && enriched.SSHPort != env.Port {
		changes["port"] = enriched.SSHPort
		changedFields = append(changedFields, fmt.Sprintf("port: %s -> %s", env.Port, enriched.SSHPort))
	}
	if enriched.SSHUsername != "" && enriched.SSHUsername != env.Username {
		changes["username"] = enriched.SSHUsername
		changedFields = append(changedFields, fmt.Sprintf("username: %s -> %s", env.Username, enriched.SSHUsername))
	}
	if enriched.SSHPassword != "" && enriched.SSHPassword != env.Password {
		changes["password"] = enriched.SSHPassword
		changedFields = append(changedFields, "password: [changed]")
	}

	if len(changes) == 0 {
		fmt.Println("Credentials already current.")
		os.Exit(1)
	}

	// Update environment in local CLI DB
	models.DB.Model(&models.Environment{}).Where("environment_id = ?", env.EnvironmentID).Updates(changes)

	// Push update to Manager via API
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	envUpdate := make(map[string]interface{})
	envUpdate["environment_id"] = env.EnvironmentID
	for k, v := range changes {
		envUpdate[k] = v
	}
	client := newAPIClient(system, captain)
	client.Post("update-environment", map[string]interface{}{
		"site_id": site.SiteID,
		"data":    envUpdate,
	})

	// Clear connection_errors from site details
	updateSiteDetails(site.SiteID, map[string]interface{}{
		"connection_errors": nil,
	}, system, captain)

	// Regenerate rclone config with new credentials
	siteIDStr := fmt.Sprintf("%d", site.SiteID)
	keyGenCmd := exec.Command("captaincore", "site", "key-generate", siteIDStr, "--captain-id="+captainID)
	keyGenCmd.Stdout = os.Stdout
	keyGenCmd.Stderr = os.Stderr
	keyGenCmd.Run()

	fmt.Println("SSH credentials updated:")
	for _, c := range changedFields {
		fmt.Printf("  %s\n", c)
	}
}

// siteStatsGenerateNative implements `captaincore site stats-generate <site>` natively in Go.
func siteStatsGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	// Check skip-already-generated flag
	if flagSkipAlreadyGenerated {
		envDetails := env.ParseDetails()
		if envDetails.Fathom != nil && string(envDetails.Fathom) != "null" && string(envDetails.Fathom) != "" {
			var fathomArr []struct {
				Code string `json:"code"`
			}
			if json.Unmarshal(envDetails.Fathom, &fathomArr) == nil && len(fathomArr) > 0 && fathomArr[0].Code != "" {
				fmt.Printf("Skipping %s-%s as tracking ID already exists\n", site.Site, sa.Environment)
				return
			}
		}
	}

	if env.HomeURL == "" {
		fmt.Printf("Error: WordPress not found for %s-%s\n", site.Site, sa.Environment)
		return
	}

	// Get site name for Fathom
	siteName := site.Name
	if strings.EqualFold(sa.Environment, "staging") {
		sshCmd := exec.Command("captaincore", "ssh", fmt.Sprintf("%s-%s", site.Site, sa.Environment),
			"--command=wp option get home --skip-plugins --skip-themes", "--captain-id="+captainID)
		output, err := sshCmd.Output()
		if err == nil {
			siteName = strings.TrimSpace(string(output))
			siteName = strings.TrimPrefix(siteName, "http://")
			siteName = strings.TrimPrefix(siteName, "https://")
		}
	}

	if siteName == "" || strings.Contains(siteName, ":") {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		return
	}

	// Create Fathom tracking site via API
	fathomReqBody, _ := json.Marshal(map[string]string{"name": siteName})
	req, _ := http.NewRequest("POST", "https://api.usefathom.com/v1/sites", bytes.NewReader(fathomReqBody))
	req.Header.Set("Authorization", "Bearer "+system.FathomAPIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("Error: Could not fetch tracking ID from Fathom for %s-%s\n", site.Site, sa.Environment)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var fathomResp struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(respBody, &fathomResp) != nil || fathomResp.ID == "" {
		fmt.Printf("Error: Could not fetch tracking ID from Fathom for %s-%s\n", site.Site, sa.Environment)
		return
	}

	// Update environment details with fathom tracking info
	var details map[string]interface{}
	if env.Details != "" {
		json.Unmarshal([]byte(env.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}

	fathomData := []map[string]string{{"domain": siteName, "code": fathomResp.ID}}
	details["fathom"] = fathomData

	detailsJSON, _ := json.Marshal(details)
	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	models.DB.Model(&models.Environment{}).Where("environment_id = ?", env.EnvironmentID).Updates(map[string]interface{}{
		"details":    string(detailsJSON),
		"updated_at": timeNow,
	})

	// Post update-fathom to CaptainCore API
	client := newAPIClient(system, captain)
	apiResp, err := client.Post("update-fathom", map[string]interface{}{
		"site_id": site.SiteID,
		"data": map[string]interface{}{
			"fathom":         fathomData,
			"environment_id": env.EnvironmentID,
		},
	})
	if err == nil {
		fmt.Print(string(apiResp))
		fmt.Println()
	}

	// Deploy tracker
	deployCmd := exec.Command("captaincore", "stats-deploy", fmt.Sprintf("%s-%s", site.Site, sa.Environment), "--captain-id="+captainID)
	deployCmd.Stdout = os.Stdout
	deployCmd.Stderr = os.Stderr
	deployCmd.Run()
}

// siteDeployDefaultsNative implements `captaincore site deploy-defaults <site>` natively in Go.
func siteDeployDefaultsNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Fetch accounts associated with this site
	var accountSites []models.AccountSite
	models.DB.Where("site_id = ?", site.SiteID).Find(&accountSites)

	// Fallback: if no account_site records, use the site's direct account_id
	if len(accountSites) == 0 && site.AccountID > 0 {
		accountSites = append(accountSites, models.AccountSite{AccountID: site.AccountID, SiteID: site.SiteID})
	}

	var recipeIDs []uint
	var deploymentScript strings.Builder

	// Add global defaults
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	defaultsValue, _ := models.GetConfiguration(uint(cid), "defaults")
	var globalDefaults struct {
		Timezone string `json:"timezone"`
		Email    string `json:"email"`
		Users    []struct {
			Username  string `json:"username"`
			Email     string `json:"email"`
			Role      string `json:"role"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"users"`
		Recipes []recipeID `json:"recipes"`
	}
	if defaultsValue != "" {
		json.Unmarshal([]byte(defaultsValue), &globalDefaults)
	}

	deploymentScript.WriteString("# Global Defaults\n")
	if globalDefaults.Timezone != "" {
		deploymentScript.WriteString(fmt.Sprintf("wp option set timezone_string %s\n", globalDefaults.Timezone))
	}
	if globalDefaults.Email != "" {
		deploymentScript.WriteString(fmt.Sprintf("wp option set admin_email %s\n", globalDefaults.Email))
	}
	for _, user := range globalDefaults.Users {
		deploymentScript.WriteString(fmt.Sprintf("wp user create %s %s --role=%s --first_name='%s' --last_name='%s' --send-email\n",
			user.Username, user.Email, user.Role, user.FirstName, user.LastName))
	}
	deploymentScript.WriteString("\n")

	for _, rid := range globalDefaults.Recipes {
		if rid == 0 {
			continue
		}
		recipeIDs = append(recipeIDs, uint(rid))
	}

	// Add account defaults (unless --global-only)
	if !flagGlobalOnly {
		for _, as := range accountSites {
			account, err := models.GetAccountByID(as.AccountID)
			if err != nil || account == nil {
				continue
			}

			var defaults struct {
				Timezone string `json:"timezone"`
				Email    string `json:"email"`
				Users    []struct {
					Username  string `json:"username"`
					Email     string `json:"email"`
					Role      string `json:"role"`
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				} `json:"users"`
				Recipes []recipeID `json:"recipes"`
			}
			if account.Defaults != "" {
				json.Unmarshal([]byte(account.Defaults), &defaults)
			}

			deploymentScript.WriteString(fmt.Sprintf("# Defaults for account: '%s'\n", account.Name))
			if defaults.Timezone != "" {
				deploymentScript.WriteString(fmt.Sprintf("wp option set timezone_string %s\n", defaults.Timezone))
			}
			if defaults.Email != "" {
				deploymentScript.WriteString(fmt.Sprintf("wp option set admin_email %s\n", defaults.Email))
			}
			for _, user := range defaults.Users {
				deploymentScript.WriteString(fmt.Sprintf("wp user create %s %s --role=%s --first_name='%s' --last_name='%s' --send-email\n",
					user.Username, user.Email, user.Role, user.FirstName, user.LastName))
			}
			deploymentScript.WriteString("\n")

			for _, rid := range defaults.Recipes {
				if rid == 0 {
					continue
				}
				recipeIDs = append(recipeIDs, uint(rid))
			}
		}
	}

	// Deduplicate recipe IDs
	recipeIDs = uniqueUints(recipeIDs)

	// Write deployment script to temp file
	timestamp := time.Now().Format("2006-01-02-03-04-05")
	token := fmt.Sprintf("%x", time.Now().UnixNano())
	scriptFile := fmt.Sprintf("%s/%s-%s-%s.sh", system.PathTmp, captainID, timestamp, token)
	os.WriteFile(scriptFile, []byte(deploymentScript.String()), 0644)

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)

	if flagDebug {
		fmt.Printf("captaincore ssh %s --script=%s --captain-id=%s\n", siteEnvArg, scriptFile, captainID)
		for _, rid := range recipeIDs {
			fmt.Printf("captaincore ssh %s --recipe=%d --captain-id=%s\n", siteEnvArg, rid, captainID)
		}
		return
	}

	fmt.Println("Deploying default configurations")
	sshCmd := exec.Command("captaincore", "ssh", siteEnvArg, "--script="+scriptFile, "--captain-id="+captainID)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Run()

	for _, rid := range recipeIDs {
		recipe, err := models.GetRecipeByID(rid)
		title := fmt.Sprintf("%d", rid)
		if err == nil && recipe != nil {
			title = recipe.Title
		}
		fmt.Printf("Deploying recipe '%s'\n", title)
		recipeCmd := exec.Command("captaincore", "ssh", siteEnvArg, fmt.Sprintf("--recipe=%d", rid), "--captain-id="+captainID)
		recipeCmd.Stdout = os.Stdout
		recipeCmd.Stderr = os.Stderr
		recipeCmd.Run()
	}
}

// siteDeleteNative implements `captaincore site delete <site>` natively in Go.
func siteDeleteNative(cmd *cobra.Command, args []string) {
	siteArg := args[0]
	var site *models.Site
	var err error

	// If numeric, treat as site_id; otherwise parse site argument
	if id, parseErr := strconv.ParseUint(siteArg, 10, 64); parseErr == nil {
		site, err = models.GetSiteByID(uint(id))
	} else {
		sa := parseSiteArgument(siteArg)
		site, err = sa.LookupSite()
	}

	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", siteArg)
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Delete from local database
	models.DeleteSiteByID(site.SiteID)

	// Post to CaptainCore API
	client := newAPIClient(system, captain)
	resp, err := client.Post("site-delete", map[string]interface{}{
		"site_id": site.SiteID,
	})
	if err == nil {
		fmt.Print(string(resp))
	}
}

// siteSearchNative implements `captaincore site search <search-term>` natively in Go.
func siteSearchNative(cmd *cobra.Command, args []string) {
	search := args[0]
	sites, err := models.SearchSites(search, flagSearchField)
	if err != nil {
		return
	}

	var results []string
	for _, site := range sites {
		if flagField == "domain" || flagField == "name" {
			results = append(results, site.Name)
		} else if flagField != "" {
			// Support other fields from the site struct
			switch flagField {
			case "site_id":
				results = append(results, strconv.FormatUint(uint64(site.SiteID), 10))
			case "provider":
				results = append(results, site.Provider)
			case "status":
				results = append(results, site.Status)
			default:
				results = append(results, site.Site)
			}
		} else {
			results = append(results, site.Site)
		}
	}

	fmt.Print(strings.Join(results, " "))
}

// siteOrphanFolder is a disk folder classified relative to active sites.
type siteOrphanFolder struct {
	Name       string
	SiteID     uint
	Size       int64
	// Expected is set when site_id is still active under a different folder name.
	Expected string
}

// loadActiveSiteFolders returns expected folder name by exact name and by site_id.
func loadActiveSiteFolders() (byName map[string]bool, byID map[uint]string, err error) {
	sites, err := models.GetAllActiveSites()
	if err != nil {
		return nil, nil, err
	}
	byName = make(map[string]bool, len(sites))
	byID = make(map[uint]string, len(sites))
	for _, s := range sites {
		folder := fmt.Sprintf("%s_%d", s.Site, s.SiteID)
		byName[folder] = true
		byID[s.SiteID] = folder
	}
	return byName, byID, nil
}

// parseSiteFolderName returns site_id if name looks like {slug}_{id}.
func parseSiteFolderName(folder string) (siteID uint, ok bool) {
	// Reject path separators / traversal in the name itself.
	if folder == "" || folder != filepath.Base(folder) || strings.Contains(folder, "..") {
		return 0, false
	}
	parts := strings.Split(folder, "_")
	if len(parts) < 2 {
		return 0, false
	}
	id, err := strconv.ParseUint(parts[len(parts)-1], 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// pathUnderRoot reports whether path resolves inside root (no escape via .. or symlink tricks).
func pathUnderRoot(path, root string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	// Prefer resolved paths when possible; if the target is already gone, Abs is enough.
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}
	if resolvedPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolvedPath
	}
	sep := string(os.PathSeparator)
	return absPath == absRoot || strings.HasPrefix(absPath, absRoot+sep)
}

// readOrphanAllowlist loads a newline-separated folder list (one name per line; # comments ok).
func readOrphanAllowlist(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Allow "folder\tsize" lines from --write-list.
		if fields := strings.Fields(line); len(fields) > 0 {
			line = fields[0]
		}
		if _, ok := parseSiteFolderName(line); ok {
			out[line] = true
		}
	}
	return out, nil
}

// scanSiteOrphans classifies disk folders under system.Path.
func scanSiteOrphans(dataPath string, byName map[string]bool, byID map[uint]string) (orphans []siteOrphanFolder, stale []siteOrphanFolder, err error) {
	entries, err := os.ReadDir(dataPath)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folder := entry.Name()
		siteID, ok := parseSiteFolderName(folder)
		if !ok {
			continue
		}
		if byName[folder] {
			continue // current active folder name
		}
		folderPath := filepath.Join(dataPath, folder)
		if !pathUnderRoot(folderPath, dataPath) {
			fmt.Printf("Skipping %s (resolves outside data path)\n", folder)
			continue
		}
		size, _ := dirSize(folderPath)
		item := siteOrphanFolder{Name: folder, SiteID: siteID, Size: size}
		if expected, active := byID[siteID]; active {
			item.Expected = expected
			stale = append(stale, item)
			continue
		}
		orphans = append(orphans, item)
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Name < orphans[j].Name })
	sort.Slice(stale, func(i, j int) bool { return stale[i].Name < stale[j].Name })
	return orphans, stale, nil
}

// siteOrphansNative implements `captaincore site orphans`.
// Scans system.Path for {site}_{id} folders that do not match an active site.
func siteOrphansNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	if system.Path == "" {
		fmt.Println("Error: system.path is empty in config.")
		return
	}

	dataPath, err := filepath.Abs(system.Path)
	if err != nil {
		fmt.Printf("Error resolving data path: %v\n", err)
		return
	}

	byName, byID, err := loadActiveSiteFolders()
	if err != nil {
		fmt.Printf("Error fetching sites: %v\n", err)
		return
	}

	if len(byName) == 0 {
		fmt.Println("Error: No active sites found in database. Aborting to prevent accidental deletion.")
		return
	}

	fmt.Printf("Scanning %s\n", dataPath)
	fmt.Printf("Found %d active site folders in database\n", len(byName))

	orphans, stale, err := scanSiteOrphans(dataPath, byName, byID)
	if err != nil {
		fmt.Printf("Error reading data path: %v\n", err)
		return
	}

	// Optional allowlist filter (used for reviewed deletes).
	var allowlist map[string]bool
	if flagOrphansFromList != "" {
		allowlist, err = readOrphanAllowlist(flagOrphansFromList)
		if err != nil {
			fmt.Printf("Error reading --from-list: %v\n", err)
			return
		}
		if len(allowlist) == 0 {
			fmt.Println("Error: --from-list is empty or has no valid folder names.")
			return
		}
		filtered := orphans[:0]
		for _, o := range orphans {
			if allowlist[o.Name] {
				filtered = append(filtered, o)
			}
		}
		orphans = filtered

		if flagOrphansIncludeStaleNames {
			filteredStale := stale[:0]
			for _, s := range stale {
				if allowlist[s.Name] {
					filteredStale = append(filteredStale, s)
				}
			}
			stale = filteredStale
		}
	}

	candidates := orphans
	if flagOrphansIncludeStaleNames {
		candidates = append(append([]siteOrphanFolder{}, orphans...), stale...)
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name < candidates[j].Name })
	}

	if len(stale) > 0 && !flagOrphansIncludeStaleNames {
		fmt.Printf("\nSkipping %d stale-name folders (site_id still active under a different name):\n", len(stale))
		fmt.Printf("%-50s %-12s %s\n", "Folder", "Size", "Active folder")
		for _, s := range stale {
			fmt.Printf("%-50s %-12s %s\n", s.Name, formatBytes(strconv.FormatInt(s.Size, 10)), s.Expected)
		}
		fmt.Println("(Use --include-stale-names only after verifying data lives under the active folder name.)")
	}

	if len(candidates) == 0 {
		if len(stale) == 0 {
			fmt.Println("No orphaned folders found.")
		} else {
			fmt.Println("\nNo deletable orphaned folders (only stale-name leftovers remain).")
		}
		return
	}

	label := "orphaned"
	if flagOrphansIncludeStaleNames {
		label = "orphaned/stale-name"
	}
	fmt.Printf("\nFound %d %s folders:\n\n", len(candidates), label)
	fmt.Printf("%-50s %s\n", "Folder", "Size")

	var totalSize int64
	for _, c := range candidates {
		totalSize += c.Size
		fmt.Printf("%-50s %s\n", c.Name, formatBytes(strconv.FormatInt(c.Size, 10)))
	}

	fmt.Printf("\nTotal reclaimable: %s across %d folders\n", formatBytes(strconv.FormatInt(totalSize, 10)), len(candidates))

	if flagOrphansWriteList != "" {
		var b strings.Builder
		b.WriteString("# captaincore site orphans allowlist\n")
		b.WriteString("# Review this file, then: captaincore site orphans --confirm --from-list=" + flagOrphansWriteList + "\n")
		for _, c := range candidates {
			b.WriteString(c.Name + "\n")
		}
		if err := os.WriteFile(flagOrphansWriteList, []byte(b.String()), 0644); err != nil {
			fmt.Printf("Error writing --write-list: %v\n", err)
			return
		}
		fmt.Printf("Wrote %d folder names to %s\n", len(candidates), flagOrphansWriteList)
	}

	if !flagConfirm {
		fmt.Println("\nDry-run only. Recommended:")
		fmt.Println("  1. captaincore connect --sync   # refresh active site set")
		fmt.Println("  2. captaincore site orphans --write-list=/tmp/orphans.txt")
		fmt.Println("  3. Review /tmp/orphans.txt (remove any rows you want to keep)")
		fmt.Println("  4. captaincore site orphans --confirm --from-list=/tmp/orphans.txt")
		return
	}

	if flagOrphansFromList == "" {
		fmt.Println("\nError: --confirm requires --from-list=<file> so only a reviewed allowlist is deleted.")
		fmt.Println("Re-run dry-run with --write-list, review the file, then pass it to --from-list.")
		return
	}

	// Re-load active set and re-scan so we never act on a stale in-memory list alone.
	byName, byID, err = loadActiveSiteFolders()
	if err != nil || len(byName) == 0 {
		fmt.Println("Error: Could not re-load active sites before delete. Aborting.")
		return
	}
	orphansNow, staleNow, err := scanSiteOrphans(dataPath, byName, byID)
	if err != nil {
		fmt.Printf("Error re-scanning before delete: %v\n", err)
		return
	}
	deletable := make(map[string]siteOrphanFolder)
	for _, o := range orphansNow {
		deletable[o.Name] = o
	}
	if flagOrphansIncludeStaleNames {
		for _, s := range staleNow {
			deletable[s.Name] = s
		}
	}

	fmt.Println()
	deleted := 0
	skipped := 0
	for _, c := range candidates {
		// Must still be on the reviewed allowlist (candidates already filtered).
		if !allowlist[c.Name] {
			fmt.Printf("Skipping %s (not in --from-list)\n", c.Name)
			skipped++
			continue
		}
		// Must still classify as deletable on re-scan.
		if _, ok := deletable[c.Name]; !ok {
			fmt.Printf("Skipping %s (no longer an orphan on re-scan)\n", c.Name)
			skipped++
			continue
		}
		siteID, ok := parseSiteFolderName(c.Name)
		if !ok {
			fmt.Printf("Skipping %s (invalid folder name)\n", c.Name)
			skipped++
			continue
		}
		if expected, active := byID[siteID]; active && !flagOrphansIncludeStaleNames {
			fmt.Printf("Skipping %s (site_id %d still active as %s)\n", c.Name, siteID, expected)
			skipped++
			continue
		}
		if byName[c.Name] {
			fmt.Printf("Skipping %s (matches active folder)\n", c.Name)
			skipped++
			continue
		}

		folderPath := filepath.Join(dataPath, c.Name)
		if !pathUnderRoot(folderPath, dataPath) {
			fmt.Printf("Skipping %s (path escapes data directory)\n", c.Name)
			skipped++
			continue
		}
		info, err := os.Lstat(folderPath)
		if err != nil {
			fmt.Printf("Skipping %s (%v)\n", c.Name, err)
			skipped++
			continue
		}
		if !info.IsDir() {
			fmt.Printf("Skipping %s (not a directory)\n", c.Name)
			skipped++
			continue
		}

		fmt.Printf("Deleting %s...\n", c.Name)
		if err := os.RemoveAll(folderPath); err != nil {
			fmt.Printf("Error deleting %s: %v\n", c.Name, err)
			continue
		}
		deleted++
	}

	fmt.Printf("\nDeleted %d orphaned folders (%d skipped).\n", deleted, skipped)
}

// uniqueUints returns unique uint values preserving order.
func uniqueUints(input []uint) []uint {
	seen := make(map[uint]bool)
	var result []uint
	for _, v := range input {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// recipeID unmarshals a recipe identifier from JSON that may encode the value
// as either a number (e.g. 7) or a quoted numeric string (e.g. "7"). The
// CaptainCore Manager UI persists recipe IDs as strings, so a plain uint target
// would silently zero them out.
type recipeID uint

func (r *recipeID) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*r = 0
		return nil
	}
	unquoted := bytes.Trim(trimmed, `"`)
	if len(unquoted) == 0 {
		*r = 0
		return nil
	}
	v, err := strconv.ParseUint(string(unquoted), 10, 64)
	if err != nil {
		return fmt.Errorf("recipeID: %w", err)
	}
	*r = recipeID(v)
	return nil
}

func init() {
	rootCmd.AddCommand(siteCmd)
	siteCmd.AddCommand(deleteCmd)
	siteCmd.AddCommand(getCmd)
	siteCmd.AddCommand(listCmd)
	siteCmd.AddCommand(keyGenerateCmd)
	siteCmd.AddCommand(sshFailCmd)
	siteCmd.AddCommand(sshRefreshCmd)
	siteCmd.AddCommand(siteCopyProductionToStaging)
	siteCmd.AddCommand(siteCopyStagingToProduction)
	siteCmd.AddCommand(sitePrepareCmd)
	siteCmd.AddCommand(siteDeployDefaultsCmd)
	siteCmd.AddCommand(siteDeployKeysCmd)
	siteCmd.AddCommand(siteStatsGenerateCmd)
	siteCmd.AddCommand(siteVulnScanCmd)
	siteCmd.AddCommand(syncSiteCmd)
	siteCmd.AddCommand(syncBatchSiteCmd)
	siteCmd.AddCommand(siteSearchCmd)
	siteCmd.AddCommand(siteOrphansCmd)
	getCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	getCmd.Flags().BoolVarP(&flagBash, "bash", "", false, "Return bash format")
	getCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Output format (json)")
	siteStatsGenerateCmd.Flags().BoolVarP(&flagSkipAlreadyGenerated, "skip-already-generated", "", false, "Skips if already has tracking")
	siteDeployDefaultsCmd.Flags().BoolVarP(&flagGlobalOnly, "global-only", "", false, "Deploy global only configurations")
	syncSiteCmd.Flags().BoolVarP(&flagDebug, "debug", "", false, "Debug response")
	syncSiteCmd.Flags().BoolVarP(&flagUpdateExtras, "update-extras", "", false, "Runs prepare site, deploy global defaults and capture screenshot")
	syncBatchSiteCmd.Flags().BoolVarP(&flagUpdateExtras, "update-extras", "", false, "Runs prepare site, deploy global defaults and capture screenshot")
	siteCopyProductionToStaging.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	siteCopyStagingToProduction.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	listCmd.Flags().StringVarP(&flagProvider, "provider", "p", "", "Filter by host provider")
	listCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Filter by <theme|plugin|core>")
	listCmd.Flags().StringVarP(&flagFilterName, "filter-name", "n", "", "Filter name")
	listCmd.Flags().StringVarP(&flagFilterVersion, "filter-version", "v", "", "Filter version")
	listCmd.Flags().StringVarP(&flagFilterStatus, "filter-status", "s", "", "Filter by status <active|inactive|dropin|must-use>")
	listCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	siteSearchCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	siteSearchCmd.Flags().StringVarP(&flagSearchField, "search-field", "", "", "Search specific field")
	siteVulnScanCmd.Flags().BoolVarP(&flagCached, "cached", "", false, "Display stored results without re-scanning")
	siteOrphansCmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Actually delete orphaned folders (requires --from-list)")
	siteOrphansCmd.Flags().StringVar(&flagOrphansWriteList, "write-list", "", "Write candidate folder names to a file for review")
	siteOrphansCmd.Flags().StringVar(&flagOrphansFromList, "from-list", "", "Only consider/delete folders named in this allowlist file")
	siteOrphansCmd.Flags().BoolVar(&flagOrphansIncludeStaleNames, "include-stale-names", false, "Also include folders whose site_id is still active under a different name")
}
