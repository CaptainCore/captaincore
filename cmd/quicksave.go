package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var quicksaveCmd = &cobra.Command{
	Use:   "quicksave",
	Short: "Quicksave commands",
}

var quicksaveListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List of quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveListNative)
	},
}

var quicksaveListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "Generate list of quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveListGenerateNative)
	},
}

var quicksaveGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "Generate new quicksave",
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

var quicksaveRestoreGitCmd = &cobra.Command{
	Use:   "restore-git <site>",
	Short: "Restores latest quicksave repo from restic repo",
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

var quicksaveGetCmd = &cobra.Command{
	Use:   "get <site> <hash>",
	Short: "Get quicksave for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveGetGenerateCmd = &cobra.Command{
	Use:   "get-generate <site> <hash>",
	Short: "Generate quicksave response",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveGetGenerateNative)
	},
}

var quicksaveListMissingCmd = &cobra.Command{
	Use:   "list-missing <site>",
	Short: "Generates list of quicksaves for a site that haven't been generated",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveListMissingNative)
	},
}

var quicksaveFileDiffCmd = &cobra.Command{
	Use:   "file-diff <site> <commit> <file>",
	Short: "Shows file diff between Quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <commit> <file> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveRollbackCmd = &cobra.Command{
	Use:   "rollback <site> <commit> [--plugin=<plugin>] [--theme=<theme>] [--all]",
	Short: "Rollback theme, plugin or file from a Quicksave on a site.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <site> <commit> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveLatestCmd = &cobra.Command{
	Use:   "latest <site>",
	Short: "Show most recent quicksave",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveLatestNative)
	},
}

var quicksaveSearchCmd = &cobra.Command{
	Use:   "search <site> <theme|plugin:title|name:search>",
	Short: "Searches Quicksaves for theme/plugin changes",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <theme|plugin:title|name:search> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveSearchNative)
	},
}

var quicksaveShowChangesCmd = &cobra.Command{
	Use:   "show-changes <site> <commit-hash> [<match>]",
	Short: "Shows file changes between Quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <commit-hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveSyncCmd = &cobra.Command{
	Use:   "sync <site>",
	Short: "Sync quicksaves to CaptainCore API",
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

var quicksaveUpdateUsageCmd = &cobra.Command{
	Use:   "update-usage <site>",
	Short: "Updates Quicksave usage stats",
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

var quicksaveAddCmd = &cobra.Command{
	Use:   "add <site>",
	Short: "Create quicksave commit",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveAddNative)
	},
}

// quicksaveListNative implements `captaincore quicksave list <site>` natively in Go.
func quicksaveListNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	listPath := filepath.Join(system.Path, siteDir, envName, "quicksaves", "list.json")

	// If file doesn't exist or is empty, regenerate
	info, statErr := os.Stat(listPath)
	if statErr != nil || info.Size() == 0 {
		siteEnvArg := fmt.Sprintf("%s-%s", site.Site, envName)
		listGenCmd := exec.Command("captaincore", "quicksave", "list-generate", siteEnvArg, "--captain-id="+captainID)
		listGenCmd.Stdout = os.Stdout
		listGenCmd.Stderr = os.Stderr
		listGenCmd.Run()
	}

	if flagField != "" {
		data, err := os.ReadFile(listPath)
		if err != nil {
			return
		}
		var items []map[string]interface{}
		if json.Unmarshal(data, &items) != nil {
			return
		}
		var values []string
		for _, item := range items {
			if val, ok := item[flagField]; ok {
				values = append(values, fmt.Sprint(val))
			}
		}
		fmt.Print(strings.Join(values, " "))
		return
	}

	data, err := os.ReadFile(listPath)
	if err != nil {
		return
	}
	fmt.Print(string(data))
}

// quicksaveAddNative implements `captaincore quicksave add <site>` natively in Go.
func quicksaveAddNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
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

	// Write versions files
	versionsDir := filepath.Join(sitePath, "versions")
	os.MkdirAll(versionsDir, 0755)

	// Pretty-print plugins JSON
	if env.Plugins != "" {
		var plugins interface{}
		if json.Unmarshal([]byte(env.Plugins), &plugins) == nil {
			prettyPlugins, _ := json.MarshalIndent(plugins, "", "    ")
			os.WriteFile(filepath.Join(versionsDir, "plugins.json"), prettyPlugins, 0644)
		}
	}

	// Pretty-print themes JSON
	if env.Themes != "" {
		var themes interface{}
		if json.Unmarshal([]byte(env.Themes), &themes) == nil {
			prettyThemes, _ := json.MarshalIndent(themes, "", "    ")
			os.WriteFile(filepath.Join(versionsDir, "themes.json"), prettyThemes, 0644)
		}
	}

	// Write core version
	os.WriteFile(filepath.Join(versionsDir, "core.json"), []byte(env.Core), 0644)

	// git add -A
	gitAdd := exec.Command("git", "add", "-A")
	gitAdd.Dir = sitePath
	gitAdd.Run()

	// git status -s
	gitStatus := exec.Command("git", "status", "-s")
	gitStatus.Dir = sitePath
	statusOutput, _ := gitStatus.Output()
	statusStr := strings.TrimSpace(string(statusOutput))

	if statusStr == "" && !flagForce {
		fmt.Println("Quicksave skipped as nothing changed")
		return
	}

	// git commit
	timeNow := time.Now().Format("2006-01-02 15:04:05")
	gitCommit := exec.Command("git", "commit", "-m", fmt.Sprintf("quicksave on %s", timeNow))
	gitCommit.Dir = sitePath
	gitCommit.Run()

	// Get hash
	gitLog := exec.Command("git", "log", "-n", "1", "--pretty=format:%H")
	gitLog.Dir = sitePath
	hashOutput, _ := gitLog.Output()
	gitHash := strings.TrimSpace(string(hashOutput))

	// Run malware scan on changed files
	quicksaveMalwareScan(sitePath, site, env, system, captain)

	// Shell out to quicksave get-generate
	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, envName)
	getGenCmd := exec.Command("captaincore", "quicksave", "get-generate", siteEnvArg, gitHash, "--captain-id="+captainID)
	getGenCmd.Stdout = os.Stdout
	getGenCmd.Stderr = os.Stderr
	getGenCmd.Run()

	// Count commits
	gitCount := exec.Command("git", "rev-list", "--all", "--count")
	gitCount.Dir = sitePath
	countOutput, _ := gitCount.Output()
	quicksaveCount := strings.TrimSpace(string(countOutput))

	// Calculate storage using filepath.Walk (cross-platform)
	var totalSize int64
	filepath.Walk(sitePath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	// Update environment details with quicksave_usage
	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
		"quicksave_usage": map[string]interface{}{
			"count":   quicksaveCount,
			"storage": fmt.Sprintf("%d", totalSize),
		},
	}, system, captain)

	// Shell out to capture
	captureCmd := exec.Command("captaincore", "capture", siteEnvArg, "--captain-id="+captainID)
	captureCmd.Stdout = os.Stdout
	captureCmd.Stderr = os.Stderr
	captureCmd.Run()
}

// quicksaveMalwareScan runs Wordfence CLI on files changed in the latest commit.
func quicksaveMalwareScan(sitePath string, site *models.Site, env *models.Environment, system *config.SystemConfig, captain *config.CaptainConfig) {
	// Get list of added/modified files from the latest commit
	gitDiff := exec.Command("git", "diff", "--name-only", "--diff-filter=AM", "HEAD~1", "HEAD")
	gitDiff.Dir = sitePath
	diffOutput, err := gitDiff.Output()
	if err != nil {
		return
	}

	changedFiles := strings.Split(strings.TrimSpace(string(diffOutput)), "\n")
	if len(changedFiles) == 0 || (len(changedFiles) == 1 && changedFiles[0] == "") {
		return
	}

	// Filter to scannable extensions
	scannableExts := map[string]bool{
		".php": true, ".js": true, ".html": true, ".htm": true,
		".svg": true, ".phtml": true, ".phar": true,
	}

	var filesToScan []string
	for _, f := range changedFiles {
		ext := strings.ToLower(filepath.Ext(f))
		if scannableExts[ext] {
			filesToScan = append(filesToScan, filepath.Join(sitePath, f))
		}
	}

	if len(filesToScan) == 0 {
		return
	}

	// Run wordfence malware-scan
	scanArgs := []string{"malware-scan", "--output-format", "csv", "--output-columns", "filename,signature_id,signature_name,signature_description,matched_text", "--output-headers", "--quiet", "--no-banner"}
	scanArgs = append(scanArgs, filesToScan...)

	scanCmd := exec.Command("wordfence", scanArgs...)
	var scanOut bytes.Buffer
	scanCmd.Stdout = &scanOut
	scanCmd.Stderr = os.Stderr
	scanCmd.Run()

	// Parse CSV output
	csvOutput := strings.TrimSpace(scanOut.String())
	if csvOutput == "" {
		return
	}

	reader := csv.NewReader(strings.NewReader(csvOutput))
	headers, err := reader.Read()
	if err != nil {
		return
	}

	// Build column index
	colIndex := make(map[string]int)
	for i, h := range headers {
		colIndex[h] = i
	}

	type finding struct {
		Filename             string `json:"filename"`
		SignatureID          string `json:"signature_id"`
		SignatureName        string `json:"signature_name"`
		SignatureDescription string `json:"signature_description"`
		MatchedText          string `json:"matched_text"`
	}

	var findings []finding
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		f := finding{}
		if i, ok := colIndex["filename"]; ok && i < len(record) {
			f.Filename = record[i]
		}
		if i, ok := colIndex["signature_id"]; ok && i < len(record) {
			f.SignatureID = record[i]
		}
		if i, ok := colIndex["signature_name"]; ok && i < len(record) {
			f.SignatureName = record[i]
		}
		if i, ok := colIndex["signature_description"]; ok && i < len(record) {
			f.SignatureDescription = record[i]
		}
		if i, ok := colIndex["matched_text"]; ok && i < len(record) {
			f.MatchedText = record[i]
		}
		findings = append(findings, f)
	}

	if len(findings) == 0 {
		return
	}

	// Print findings to stdout
	fmt.Printf("Malware scan: %d finding(s) on %s-%s\n", len(findings), site.Site, strings.ToLower(env.Environment))
	for _, f := range findings {
		fmt.Printf("  %s — %s\n", f.Filename, f.SignatureName)
	}

	// Determine home URL
	homeURL := env.HomeURL

	// POST alert to CaptainCore API
	client := newAPIClient(system, captain)
	client.Post("malware-alert", map[string]interface{}{
		"site_id": site.SiteID,
		"data": map[string]interface{}{
			"site_name":   site.Name,
			"environment": env.Environment,
			"home_url":    homeURL,
			"findings":    findings,
		},
	})
}

// quicksaveLatestNative implements `captaincore quicksave latest <site>` natively in Go.
func quicksaveLatestNative(cmd *cobra.Command, args []string) {
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

	listPath := filepath.Join(system.Path, fmt.Sprintf("%s_%d", site.Site, site.SiteID), sa.Environment, "quicksaves", "list.json")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return
	}

	var list []json.RawMessage
	if json.Unmarshal(data, &list) != nil || len(list) == 0 {
		return
	}

	if flagField != "" {
		var entry map[string]interface{}
		if json.Unmarshal(list[0], &entry) == nil {
			if val, ok := entry[flagField]; ok {
				fmt.Print(val)
			}
		}
		return
	}

	// Pretty-print the first entry
	var pretty interface{}
	if json.Unmarshal(list[0], &pretty) == nil {
		out, _ := json.MarshalIndent(pretty, "", "    ")
		fmt.Print(string(out))
	}
}

// quicksaveListGenerateNative implements `captaincore quicksave list-generate <site>` natively in Go.
func quicksaveListGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	quicksaveDir := filepath.Join(system.Path, siteDir, envName, "quicksave")
	quicksavesDir := filepath.Join(system.Path, siteDir, envName, "quicksaves")

	// Check if quicksave git repo exists
	if _, err := os.Stat(filepath.Join(quicksaveDir, ".git")); os.IsNotExist(err) {
		fmt.Printf("Skipping generationing of %s/%s/quicksaves/list.json as no quicksaves found.\n", siteDir, envName)
		return
	}

	os.MkdirAll(quicksavesDir, 0755)
	fmt.Printf("Generating %s/%s/quicksaves/list.json\n", siteDir, envName)

	// Run git log
	gitCmd := exec.Command("git", "log", "--pretty=format:%H %ct")
	gitCmd.Dir = quicksaveDir
	gitOutput, err := gitCmd.Output()
	if err != nil {
		return
	}

	type quicksaveListItem struct {
		Hash         string      `json:"hash"`
		CreatedAt    json.Number `json:"created_at"`
		Core         string      `json:"core,omitempty"`
		ThemeCount   int         `json:"theme_count,omitempty"`
		PluginCount  int         `json:"plugin_count,omitempty"`
		CorePrevious string      `json:"core_previous,omitempty"`
	}

	var quicksaves []quicksaveListItem
	lines := strings.Split(string(gitOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "-n" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			continue
		}

		item := quicksaveListItem{
			Hash:      parts[0],
			CreatedAt: json.Number(parts[1]),
		}

		// Try reading commit file for extra fields
		commitFile := filepath.Join(quicksavesDir, fmt.Sprintf("commit-%s.json", parts[0]))
		commitData, err := os.ReadFile(commitFile)
		if err == nil {
			var commitObj struct {
				Core         string `json:"core"`
				ThemeCount   int    `json:"theme_count"`
				PluginCount  int    `json:"plugin_count"`
				CorePrevious string `json:"core_previous"`
			}
			if json.Unmarshal(commitData, &commitObj) == nil {
				if commitObj.Core != "" {
					item.Core = commitObj.Core
				}
				if commitObj.ThemeCount > 0 {
					item.ThemeCount = commitObj.ThemeCount
				}
				if commitObj.PluginCount > 0 {
					item.PluginCount = commitObj.PluginCount
				}
				if commitObj.CorePrevious != "" {
					item.CorePrevious = commitObj.CorePrevious
				}
			}
		}

		quicksaves = append(quicksaves, item)
	}

	// Output JSON
	result, _ := json.MarshalIndent(quicksaves, "", "    ")
	listPath := filepath.Join(quicksavesDir, "list.json")
	os.WriteFile(listPath, result, 0644)
	fmt.Print(string(result))

	// Update environment details with quicksave_count
	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
		"quicksave_count": len(quicksaves),
	}, system, captain)
}

// quicksaveGetGenerateNative implements `captaincore quicksave get-generate <site> <hash>` natively in Go.
func quicksaveGetGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	hash := args[1]

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	quicksaveDir := filepath.Join(system.Path, siteDir, envName, "quicksave")
	quicksavesDir := filepath.Join(system.Path, siteDir, envName, "quicksaves")
	os.MkdirAll(quicksavesDir, 0755)

	fmt.Printf("Generating %s/%s/quicksaves/commit-%s.json\n", siteDir, envName, hash)

	// Helper to run git show in quicksave dir
	gitShow := func(gitArgs ...string) string {
		c := exec.Command("git", gitArgs...)
		c.Dir = quicksaveDir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	// Get current commit data
	currentCore := gitShow("show", hash+":versions/core.json")
	currentThemesRaw := gitShow("show", hash+":versions/themes.json")
	currentPluginsRaw := gitShow("show", hash+":versions/plugins.json")
	status := gitShow("show", hash, "--shortstat", "--format=")

	type themePlugin struct {
		Name           string      `json:"name"`
		Title          string      `json:"title,omitempty"`
		Status         string      `json:"status"`
		Version        string      `json:"version"`
		Changed        *bool       `json:"changed,omitempty"`
		New            *bool       `json:"new,omitempty"`
		ChangedVersion string      `json:"changed_version,omitempty"`
		ChangedStatus  string      `json:"changed_status,omitempty"`
		ChangedTitle   string      `json:"changed_title,omitempty"`
		Extra          map[string]json.RawMessage `json:"-"`
	}

	var currentThemes []map[string]interface{}
	var currentPlugins []map[string]interface{}
	json.Unmarshal([]byte(currentThemesRaw), &currentThemes)
	json.Unmarshal([]byte(currentPluginsRaw), &currentPlugins)

	// Get parent hash
	previousHash := gitShow("show", "-s", "--pretty=format:%P", hash)
	var previousCore string
	var previousCreatedAt string
	var themesDeleted []map[string]interface{}
	var pluginsDeleted []map[string]interface{}

	if previousHash != "" {
		previousCreatedAt = gitShow("show", "-s", "--pretty=format:%ct", previousHash)
		previousCore = gitShow("show", previousHash+":versions/core.json")
		previousThemesRaw := gitShow("show", previousHash+":versions/themes.json")
		previousPluginsRaw := gitShow("show", previousHash+":versions/plugins.json")

		var previousThemes []map[string]interface{}
		var previousPlugins []map[string]interface{}
		json.Unmarshal([]byte(previousThemesRaw), &previousThemes)
		json.Unmarshal([]byte(previousPluginsRaw), &previousPlugins)

		// Get changed files
		filesChangedStr := gitShow("diff", previousHash, hash, "--name-only")
		filesChanged := strings.Split(filesChangedStr, "\n")

		// Build lookup maps
		prevThemeMap := make(map[string]map[string]interface{})
		for _, t := range previousThemes {
			if name, ok := t["name"].(string); ok {
				prevThemeMap[name] = t
			}
		}
		prevPluginMap := make(map[string]map[string]interface{})
		for _, p := range previousPlugins {
			if name, ok := p["name"].(string); ok {
				prevPluginMap[name] = p
			}
		}

		currentThemeNames := make(map[string]bool)
		currentPluginNames := make(map[string]bool)

		// Compare themes
		for i, theme := range currentThemes {
			name, _ := theme["name"].(string)
			currentThemeNames[name] = true

			prev, existed := prevThemeMap[name]
			if !existed {
				currentThemes[i]["changed"] = true
				currentThemes[i]["new"] = true
				continue
			}

			changed := false
			if fmt.Sprint(theme["version"]) != fmt.Sprint(prev["version"]) {
				currentThemes[i]["changed_version"] = prev["version"]
				changed = true
			}
			if fmt.Sprint(theme["status"]) != fmt.Sprint(prev["status"]) {
				currentThemes[i]["changed_status"] = prev["status"]
				changed = true
			}
			if fmt.Sprint(theme["title"]) != fmt.Sprint(prev["title"]) {
				currentThemes[i]["changed_title"] = prev["title"]
				changed = true
			}
			if !changed {
				for _, file := range filesChanged {
					if strings.HasPrefix(file, "themes/"+name) {
						changed = true
						break
					}
				}
			}
			currentThemes[i]["changed"] = changed
		}

		// Compare plugins
		for i, plugin := range currentPlugins {
			name, _ := plugin["name"].(string)
			currentPluginNames[name] = true

			prev, existed := prevPluginMap[name]
			if !existed {
				currentPlugins[i]["changed"] = true
				currentPlugins[i]["new"] = true
				continue
			}

			changed := false
			if fmt.Sprint(plugin["version"]) != fmt.Sprint(prev["version"]) {
				currentPlugins[i]["changed_version"] = prev["version"]
				changed = true
			}
			if fmt.Sprint(plugin["status"]) != fmt.Sprint(prev["status"]) {
				currentPlugins[i]["changed_status"] = prev["status"]
				changed = true
			}
			if fmt.Sprint(plugin["title"]) != fmt.Sprint(prev["title"]) {
				currentPlugins[i]["changed_title"] = prev["title"]
				changed = true
			}
			if !changed {
				for _, file := range filesChanged {
					if strings.HasPrefix(file, "plugins/"+name) {
						changed = true
						break
					}
				}
			}
			currentPlugins[i]["changed"] = changed
		}

		// Find deleted themes
		for _, pt := range previousThemes {
			if name, ok := pt["name"].(string); ok && !currentThemeNames[name] {
				themesDeleted = append(themesDeleted, pt)
			}
		}

		// Find deleted plugins
		for _, pp := range previousPlugins {
			if name, ok := pp["name"].(string); ok && !currentPluginNames[name] {
				pluginsDeleted = append(pluginsDeleted, pp)
			}
		}
	}

	// Sort themes: changed first, then alphabetical
	sort.Slice(currentThemes, func(i, j int) bool {
		iChanged, _ := currentThemes[i]["changed"].(bool)
		jChanged, _ := currentThemes[j]["changed"].(bool)
		if iChanged != jChanged {
			if iChanged {
				return true
			}
			return false
		}
		iName, _ := currentThemes[i]["name"].(string)
		jName, _ := currentThemes[j]["name"].(string)
		return iName < jName
	})

	// Sort plugins: must-use first, then changed first, then alphabetical
	sort.Slice(currentPlugins, func(i, j int) bool {
		iStatus, _ := currentPlugins[i]["status"].(string)
		jStatus, _ := currentPlugins[j]["status"].(string)
		if iStatus == "must-use" || jStatus == "must-use" {
			return iStatus < jStatus
		}
		iChanged, _ := currentPlugins[i]["changed"].(bool)
		jChanged, _ := currentPlugins[j]["changed"].(bool)
		if iChanged != jChanged {
			if iChanged {
				return true
			}
			return false
		}
		iName, _ := currentPlugins[i]["name"].(string)
		jName, _ := currentPlugins[j]["name"].(string)
		return iName < jName
	})

	// Build output
	output := map[string]interface{}{
		"core":            currentCore,
		"core_previous":   previousCore,
		"theme_count":     len(currentThemes),
		"themes":          currentThemes,
		"themes_deleted":  themesDeleted,
		"plugin_count":    len(currentPlugins),
		"plugins":         currentPlugins,
		"plugins_deleted": pluginsDeleted,
		"status":          status,
	}
	if previousCreatedAt != "" {
		output["previous_created_at"] = previousCreatedAt
	}

	result, _ := json.MarshalIndent(output, "", "    ")

	// Write to commit file
	commitFile := filepath.Join(quicksavesDir, fmt.Sprintf("commit-%s.json", hash))
	os.WriteFile(commitFile, result, 0644)
}

// quicksaveListMissingNative implements `captaincore quicksave list-missing <site>` natively in Go.
func quicksaveListMissingNative(cmd *cobra.Command, args []string) {
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

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)

	// Run list-generate first
	listGenCmd := exec.Command("captaincore", "quicksave", "list-generate", siteEnvArg, "--captain-id="+captainID)
	listGenCmd.Stdout = os.Stdout
	listGenCmd.Stderr = os.Stderr
	listGenCmd.Run()

	// Read the list
	listPath := filepath.Join(system.Path, siteDir, sa.Environment, "quicksaves", "list.json")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return
	}

	var quicksaves []struct {
		Hash string `json:"hash"`
	}
	if json.Unmarshal(data, &quicksaves) != nil {
		return
	}

	for _, qs := range quicksaves {
		commitPath := filepath.Join(system.Path, siteDir, sa.Environment, "quicksaves", fmt.Sprintf("commit-%s.json", qs.Hash))
		if _, statErr := os.Stat(commitPath); os.IsNotExist(statErr) {
			fmt.Printf("Generating missing %s/%s/quicksaves/commit-%s.json\n", siteDir, sa.Environment, qs.Hash)
			getGenCmd := exec.Command("captaincore", "quicksave", "get-generate", siteEnvArg, qs.Hash, "--captain-id="+captainID)
			getGenCmd.Stdout = os.Stdout
			getGenCmd.Stderr = os.Stderr
			getGenCmd.Run()
		}
	}

	// Run list-generate again
	listGenCmd2 := exec.Command("captaincore", "quicksave", "list-generate", siteEnvArg, "--captain-id="+captainID)
	listGenCmd2.Stdout = os.Stdout
	listGenCmd2.Stderr = os.Stderr
	listGenCmd2.Run()
}

// quicksaveSearchNative implements `captaincore quicksave search <site> <type:field:value>` natively in Go.
func quicksaveSearchNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])

	// Decode search argument (may be base64 encoded)
	searchArg := args[1]
	if decoded, err := base64.StdEncoding.DecodeString(searchArg); err == nil {
		// Verify it was valid base64 by re-encoding
		if base64.StdEncoding.EncodeToString(decoded) == searchArg {
			searchArg = string(decoded)
		}
	}

	searchParts := strings.SplitN(searchArg, ":", 3)
	if len(searchParts) < 3 {
		return
	}
	searchType := searchParts[0] + "s" // "theme" -> "themes", "plugin" -> "plugins"
	searchField := searchParts[1]
	searchValue := searchParts[2]

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	quicksaveDir := filepath.Join(system.Path, siteDir, sa.Environment, "quicksave")
	quicksavesDir := filepath.Join(system.Path, siteDir, sa.Environment, "quicksaves")

	// Run git log to get commit hashes and timestamps
	gitCmd := exec.Command("git", "log", "--pretty=format:%H %ct")
	gitCmd.Dir = quicksaveDir
	gitOutput, err := gitCmd.Output()
	if err != nil {
		return
	}

	type quicksaveItem struct {
		Hash      string      `json:"hash"`
		CreatedAt json.Number `json:"created_at"`
		Item      interface{} `json:"item"`
	}

	var quicksaves []quicksaveItem
	lines := strings.Split(string(gitOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "-n" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			continue
		}

		qs := quicksaveItem{
			Hash:      parts[0],
			CreatedAt: json.Number(parts[1]),
		}

		commitFile := filepath.Join(quicksavesDir, fmt.Sprintf("commit-%s.json", parts[0]))
		commitData, err := os.ReadFile(commitFile)
		if err != nil {
			continue
		}

		var commitObj map[string]json.RawMessage
		if json.Unmarshal(commitData, &commitObj) != nil {
			continue
		}

		itemsRaw, ok := commitObj[searchType]
		if !ok {
			continue
		}

		var items []map[string]interface{}
		if json.Unmarshal(itemsRaw, &items) != nil {
			continue
		}

		found := false
		for _, item := range items {
			if val, ok := item[searchField]; ok && fmt.Sprint(val) == searchValue {
				// Remove changed_version and changed_title
				delete(item, "changed_version")
				delete(item, "changed_title")
				qs.Item = item
				found = true
				break
			}
		}
		if !found {
			qs.Item = ""
		}

		quicksaves = append(quicksaves, qs)
	}

	// Sort ascending by created_at
	sort.Slice(quicksaves, func(i, j int) bool {
		a, _ := quicksaves[i].CreatedAt.Int64()
		b, _ := quicksaves[j].CreatedAt.Int64()
		return a < b
	})

	// Deduplicate consecutive identical items
	if len(quicksaves) > 0 {
		var deduped []quicksaveItem
		deduped = append(deduped, quicksaves[0])
		for i := 1; i < len(quicksaves); i++ {
			prevJSON, _ := json.Marshal(deduped[len(deduped)-1].Item)
			currJSON, _ := json.Marshal(quicksaves[i].Item)
			if string(prevJSON) != string(currJSON) {
				deduped = append(deduped, quicksaves[i])
			}
		}
		quicksaves = deduped
	}

	// If only 1 result, output empty array (matches PHP behavior)
	if len(quicksaves) <= 1 {
		fmt.Print("[]")
		return
	}

	// Sort descending by created_at
	sort.Slice(quicksaves, func(i, j int) bool {
		a, _ := quicksaves[i].CreatedAt.Int64()
		b, _ := quicksaves[j].CreatedAt.Int64()
		return a > b
	})

	out, _ := json.MarshalIndent(quicksaves, "", "    ")
	fmt.Print(string(out))
}

func init() {
	rootCmd.AddCommand(quicksaveCmd)
	quicksaveCmd.AddCommand(quicksaveAddCmd)
	quicksaveCmd.AddCommand(quicksaveGetCmd)
	quicksaveCmd.AddCommand(quicksaveGetGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveLatestCmd)
	quicksaveCmd.AddCommand(quicksaveListCmd)
	quicksaveCmd.AddCommand(quicksaveListGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveListMissingCmd)
	quicksaveCmd.AddCommand(quicksaveFileDiffCmd)
	quicksaveCmd.AddCommand(quicksaveRestoreGitCmd)
	quicksaveCmd.AddCommand(quicksaveRollbackCmd)
	quicksaveCmd.AddCommand(quicksaveSearchCmd)
	quicksaveCmd.AddCommand(quicksaveShowChangesCmd)
	quicksaveCmd.AddCommand(quicksaveSyncCmd)
	quicksaveCmd.AddCommand(quicksaveUpdateUsageCmd)
	quicksaveFileDiffCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme slug")
	quicksaveFileDiffCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin slug")
	quicksaveLatestCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveListCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveRollbackCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagVersion, "version", "this", "Rollback to 'this' or 'previous' version")
	quicksaveRollbackCmd.Flags().StringVar(&flagFile, "file", "", "File to rollback")
	quicksaveRollbackCmd.Flags().BoolVar(&flagAll, "all", false, "All themes and plugins")
	quicksaveAddCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force even if no changes")
	quicksaveFileDiffCmd.Flags().BoolVar(&flagHtml, "html", false, "Returns HTML format")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force a new Quicksave")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
	quicksaveGenerateCmd.Flags().StringVarP(&flagSkipIfRecent, "skip-if-recent", "", "", "Skip if quicksave generated within timeframe (e.g. 24h)")
}
