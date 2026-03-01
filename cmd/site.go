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

		captureCmd := exec.Command("captaincore", "capture", siteIDStr+"-production", "--captain-id="+captainID)
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

	var recipeIDs []uint
	var deploymentScript strings.Builder

	// Add global defaults
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	configValue, _ := models.GetConfiguration(uint(cid), "configurations")
	var globalDefaults struct {
		Timezone string `json:"timezone"`
		Email    string `json:"email"`
		Recipes  []uint `json:"recipes"`
	}
	if configValue != "" {
		var configObj map[string]json.RawMessage
		if json.Unmarshal([]byte(configValue), &configObj) == nil {
			if defaultsRaw, ok := configObj["defaults"]; ok {
				json.Unmarshal(defaultsRaw, &globalDefaults)
			}
		}
	}

	deploymentScript.WriteString("# Global Defaults\n")
	if globalDefaults.Timezone != "" {
		deploymentScript.WriteString(fmt.Sprintf("wp option set timezone_string %s\n", globalDefaults.Timezone))
	}
	if globalDefaults.Email != "" {
		deploymentScript.WriteString(fmt.Sprintf("wp option set admin_email %s\n", globalDefaults.Email))
	}
	deploymentScript.WriteString("\n")

	for _, rid := range globalDefaults.Recipes {
		recipeIDs = append(recipeIDs, rid)
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
				Recipes []uint `json:"recipes"`
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
				recipeIDs = append(recipeIDs, rid)
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

func init() {
	rootCmd.AddCommand(siteCmd)
	siteCmd.AddCommand(deleteCmd)
	siteCmd.AddCommand(getCmd)
	siteCmd.AddCommand(listCmd)
	siteCmd.AddCommand(keyGenerateCmd)
	siteCmd.AddCommand(sshFailCmd)
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
}
