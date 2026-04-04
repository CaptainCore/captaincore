package cmd

import (
	"bufio"
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
	"regexp"
	"sort"
	"strconv"
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
		if flagDryRun {
			dryRunGenerate(args[0], "quicksaves")
			return
		}
		resolveCommand(cmd, args)
	},
}

var quicksaveBackupCmd = &cobra.Command{
	Use:   "backup <site>",
	Short: "Backup quicksave git repo to restic",
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

	// Pretty-print plugins JSON (strip hash field — it's infrastructure metadata, not relevant to quicksave diffs)
	if env.Plugins != "" {
		var plugins []map[string]interface{}
		if json.Unmarshal([]byte(env.Plugins), &plugins) == nil {
			for i := range plugins {
				delete(plugins[i], "hash")
			}
			prettyPlugins, _ := json.MarshalIndent(plugins, "", "    ")
			os.WriteFile(filepath.Join(versionsDir, "plugins.json"), prettyPlugins, 0644)
		}
	}

	// Pretty-print themes JSON (strip hash field)
	if env.Themes != "" {
		var themes []map[string]interface{}
		if json.Unmarshal([]byte(env.Themes), &themes) == nil {
			for i := range themes {
				delete(themes[i], "hash")
			}
			prettyThemes, _ := json.MarshalIndent(themes, "", "    ")
			os.WriteFile(filepath.Join(versionsDir, "themes.json"), prettyThemes, 0644)
		}
	}

	// Write core version with checksum details
	coreJSON := buildCoreJSON(env)
	os.WriteFile(filepath.Join(versionsDir, "core.json"), coreJSON, 0644)

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
		fmt.Println("  Quicksave skipped as nothing changed")
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

	// Scan core checksum extra/modified files via SSH + local Wordfence
	quicksaveCoreChecksumScan(site, env, system, captain)

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

	// Shell out to capture generate
	captureCmd := exec.Command("captaincore", "capture", "generate", siteEnvArg, "--captain-id="+captainID)
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

// extractCoreVersion extracts just the version string from core.json content.
// Handles both old format ("6.9.4") and new format ({"version":"6.9.4","checksums":{...}}).
func extractCoreVersion(coreStr string) string {
	coreStr = strings.TrimSpace(coreStr)
	if coreStr == "" {
		return ""
	}
	// Try parsing as JSON object with a "version" field
	var obj struct {
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(coreStr), &obj) == nil && obj.Version != "" {
		return obj.Version
	}
	// Already a plain version string
	return coreStr
}

// buildCoreJSON creates the versions/core.json content with version and checksum details.
func buildCoreJSON(env *models.Environment) []byte {
	// Parse core_checksum_details from environment details
	var details map[string]interface{}
	if env.Details != "" {
		json.Unmarshal([]byte(env.Details), &details)
	}

	coreData := map[string]interface{}{
		"version": env.Core,
	}

	if details != nil {
		if checksumDetails, ok := details["core_checksum_details"]; ok {
			coreData["checksums"] = checksumDetails
		}
	}

	pretty, err := json.MarshalIndent(coreData, "", "    ")
	if err != nil {
		return []byte(env.Core)
	}
	return pretty
}

// quicksaveCoreChecksumScan checks core checksum details for extra/modified files,
// SCPs them to the local backup directory, and runs Wordfence malware-scan locally.
func quicksaveCoreChecksumScan(site *models.Site, env *models.Environment, system *config.SystemConfig, captain *config.CaptainConfig) {
	// Parse core_checksum_details from environment details
	var details map[string]interface{}
	if env.Details != "" {
		json.Unmarshal([]byte(env.Details), &details)
	}
	if details == nil {
		return
	}

	checksumRaw, ok := details["core_checksum_details"]
	if !ok {
		return
	}

	// Re-marshal and unmarshal to get typed access
	checksumJSON, _ := json.Marshal(checksumRaw)
	var checksumDetails struct {
		Status   string   `json:"status"`
		Modified []string `json:"modified"`
		Extra    []string `json:"extra"`
		Missing  []string `json:"missing"`
	}
	if json.Unmarshal(checksumJSON, &checksumDetails) != nil {
		return
	}

	// Collect files that need scanning (extra and modified)
	var remoteFiles []string
	remoteFiles = append(remoteFiles, checksumDetails.Extra...)
	remoteFiles = append(remoteFiles, checksumDetails.Modified...)
	if len(remoteFiles) == 0 {
		return
	}

	// Filter to scannable extensions
	scannableExts := map[string]bool{
		".php": true, ".js": true, ".html": true, ".htm": true,
		".svg": true, ".phtml": true, ".phar": true,
	}
	var filesToFetch []string
	for _, f := range remoteFiles {
		ext := strings.ToLower(filepath.Ext(f))
		if scannableExts[ext] {
			filesToFetch = append(filesToFetch, f)
		}
	}
	if len(filesToFetch) == 0 {
		return
	}

	// Build SSH connection details
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	backupDir := filepath.Join(system.Path, siteDir, envName, "backup")
	os.MkdirAll(backupDir, 0755)

	// Determine SSH key
	key := ""
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	configValue, _ := models.GetConfiguration(uint(cid), "configurations")
	if configValue != "" {
		var configObj map[string]json.RawMessage
		if json.Unmarshal([]byte(configValue), &configObj) == nil {
			if defaultKeyRaw, ok := configObj["default_key"]; ok {
				json.Unmarshal(defaultKeyRaw, &key)
			}
		}
	}
	if key == "" {
		return
	}
	keyPath := filepath.Join(system.PathKeys, captainID, key)

	// Determine remote root directory
	remoteRoot := "public"
	switch site.Provider {
	case "rocketdotnet":
		remoteRoot = env.HomeDirectory
	default:
		if env.HomeDirectory != "" {
			remoteRoot = env.HomeDirectory
		}
	}

	// SCP each file to the backup directory
	sshOptions := fmt.Sprintf("-q -oStrictHostKeyChecking=no -oConnectTimeout=15 -oPreferredAuthentications=publickey -i %s", keyPath)
	var localFiles []string

	for _, remoteFile := range filesToFetch {
		remotePath := fmt.Sprintf("%s@%s:%s/%s", env.Username, env.Address, remoteRoot, remoteFile)

		// Ensure local subdirectory exists (for files like wp-admin/foo.php)
		localPath := filepath.Join(backupDir, remoteFile)
		os.MkdirAll(filepath.Dir(localPath), 0755)

		// SCP the file
		scpArgs := strings.Fields(sshOptions)
		scpArgs = append(scpArgs, "-P", env.Port, remotePath, localPath)
		scpCmd := exec.Command("scp", scpArgs...)
		if err := scpCmd.Run(); err != nil {
			fmt.Printf("  Core checksum scan: failed to SCP %s\n", remoteFile)
			continue
		}
		localFiles = append(localFiles, localPath)
	}

	if len(localFiles) == 0 {
		return
	}

	// Run Wordfence malware-scan on the local copies
	scanArgs := []string{"malware-scan", "--output-format", "csv", "--output-columns", "filename,signature_id,signature_name,signature_description,matched_text", "--output-headers", "--quiet", "--no-banner"}
	scanArgs = append(scanArgs, localFiles...)

	scanCmd := exec.Command("wordfence", scanArgs...)
	var scanOut bytes.Buffer
	scanCmd.Stdout = &scanOut
	scanCmd.Stderr = os.Stderr
	scanCmd.Run()

	// Parse CSV output
	csvOutput := strings.TrimSpace(scanOut.String())
	if csvOutput == "" {
		fmt.Printf("  Core checksum scan: %d file(s) checked, clean\n", len(localFiles))
		return
	}

	reader := csv.NewReader(strings.NewReader(csvOutput))
	headers, err := reader.Read()
	if err != nil {
		return
	}

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
			// Replace local backup path with the original remote filename for clarity
			f.Filename = record[i]
			for _, rf := range filesToFetch {
				if strings.HasSuffix(f.Filename, rf) {
					f.Filename = rf
					break
				}
			}
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
		fmt.Printf("  Core checksum scan: %d file(s) checked, clean\n", len(localFiles))
		return
	}

	// Print findings
	fmt.Printf("Core checksum scan: %d finding(s) on %s-%s\n", len(findings), site.Site, strings.ToLower(env.Environment))
	for _, f := range findings {
		fmt.Printf("  %s — %s\n", f.Filename, f.SignatureName)
	}

	// POST alert to CaptainCore API
	client := newAPIClient(system, captain)
	client.Post("malware-alert", map[string]interface{}{
		"site_id": site.SiteID,
		"data": map[string]interface{}{
			"site_name":   site.Name,
			"environment": env.Environment,
			"home_url":    env.HomeURL,
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
					item.Core = extractCoreVersion(commitObj.Core)
				}
				if commitObj.ThemeCount > 0 {
					item.ThemeCount = commitObj.ThemeCount
				}
				if commitObj.PluginCount > 0 {
					item.PluginCount = commitObj.PluginCount
				}
				if commitObj.CorePrevious != "" {
					item.CorePrevious = extractCoreVersion(commitObj.CorePrevious)
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

	// Build output (extract version string from core.json — full checksums live in versions/core.json)
	output := map[string]interface{}{
		"core":            extractCoreVersion(currentCore),
		"core_previous":   extractCoreVersion(previousCore),
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

var quicksaveMalwareScanCmd = &cobra.Command{
	Use:   "malware-scan <site>",
	Short: "Scan quicksave files for malware signatures",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveMalwareScanNative)
	},
}

// malwareSignature represents a single malware detection rule.
type malwareSignature struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Severity     string   `json:"severity"`
	Patterns     []string `json:"patterns"`
	Description  string   `json:"description"`
	ExcludePaths []string `json:"exclude_paths"`
}

// malwareFinding represents a single match found during scanning.
type malwareFinding struct {
	SignatureID   string `json:"signature_id"`
	SignatureName string `json:"signature_name"`
	Severity      string `json:"severity"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Match         string `json:"match"`
}

// loadMalwareSignatures reads the signatures JSON file.
func loadMalwareSignatures() ([]malwareSignature, error) {
	home, _ := os.UserHomeDir()
	sigPath := filepath.Join(home, ".captaincore", "lib", "malware-signatures.json")
	data, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, fmt.Errorf("could not read malware signatures: %w", err)
	}
	var sigs []malwareSignature
	if err := json.Unmarshal(data, &sigs); err != nil {
		return nil, fmt.Errorf("could not parse malware signatures: %w", err)
	}
	return sigs, nil
}

// compiledSig holds a malware signature with pre-compiled regex patterns.
type compiledSig struct {
	Sig      malwareSignature
	Patterns []*regexp.Regexp
}

// scanFileForMalware checks a single file against all compiled signature patterns.
func scanFileForMalware(filePath string, relPath string, compiledSigs []compiledSig) []malwareFinding {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	// Skip binary files (check first 512 bytes for null bytes)
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for _, b := range data[:checkLen] {
		if b == 0 {
			return nil
		}
	}

	lines := strings.Split(string(data), "\n")
	var findings []malwareFinding

	for _, cs := range compiledSigs {
		// Check exclude paths
		excluded := false
		for _, ep := range cs.Sig.ExcludePaths {
			if strings.Contains(relPath, ep) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		for lineNum, line := range lines {
			for _, pat := range cs.Patterns {
				if pat.MatchString(line) {
					matchText := line
					if len(matchText) > 200 {
						matchText = matchText[:200] + "..."
					}
					findings = append(findings, malwareFinding{
						SignatureID:   cs.Sig.ID,
						SignatureName: cs.Sig.Name,
						Severity:      cs.Sig.Severity,
						File:          relPath,
						Line:          lineNum + 1,
						Match:         strings.TrimSpace(matchText),
					})
					// One match per signature per file is enough
					goto nextSig
				}
			}
		}
	nextSig:
	}

	return findings
}

// malwareScanTarget represents a site environment to scan for malware.
type malwareScanTarget struct {
	Label    string
	ScanPath string
}

// quicksaveMalwareScanNative implements `captaincore quicksave malware-scan <site>` natively in Go.
func quicksaveMalwareScanNative(cmd *cobra.Command, args []string) {
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Determine which sites to scan
	var targets []malwareScanTarget

	if strings.HasPrefix(args[0], "@") {
		// Fleet mode: scan all matching sites
		sites, err := models.GetAllActiveSites()
		if err != nil {
			fmt.Printf("Error fetching sites: %v\n", err)
			return
		}

		environment, _ := models.ParseTargetString(args[0])

		for _, site := range sites {
			envs, err := models.FindEnvironmentsBySiteID(site.SiteID)
			if err != nil {
				continue
			}
			for _, env := range envs {
				if environment != "" && environment != "all" && !strings.EqualFold(env.Environment, environment) {
					continue
				}
				envName := strings.ToLower(env.Environment)
				siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
				scanPath := filepath.Join(system.Path, siteDir, envName, "quicksave")
				if _, err := os.Stat(scanPath); os.IsNotExist(err) {
					continue
				}
				targets = append(targets, malwareScanTarget{
					Label:    fmt.Sprintf("%s-%s", site.Site, envName),
					ScanPath: scanPath,
				})
			}
		}
		scanType := "malware signatures"
		if flagFull {
			scanType = "malware (Wordfence CLI)"
		}
		fmt.Printf("Scanning %d environments for %s...\n\n", len(targets), scanType)
	} else {
		// Single site mode
		sa := parseSiteArgument(args[0])
		site, err := sa.LookupSite()
		if err != nil || site == nil {
			fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
			return
		}
		env, err := sa.LookupEnvironment(site.SiteID)
		if err != nil || env == nil {
			fmt.Printf("Error: Environment not found.\n")
			return
		}
		envName := strings.ToLower(env.Environment)
		siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
		scanPath := filepath.Join(system.Path, siteDir, envName, "quicksave")
		if _, err := os.Stat(scanPath); os.IsNotExist(err) {
			fmt.Printf("No quicksave found at %s\n", scanPath)
			return
		}
		targets = append(targets, malwareScanTarget{
			Label:    fmt.Sprintf("%s-%s", site.Site, envName),
			ScanPath: scanPath,
		})
	}

	if flagFull {
		quicksaveMalwareScanFull(targets)
	} else {
		quicksaveMalwareScanSignatures(targets)
	}
}

// quicksaveMalwareScanFull runs Wordfence CLI malware-scan against entire quicksave directories.
func quicksaveMalwareScanFull(targets []malwareScanTarget) {
	isFleet := len(targets) > 1
	totalFindings := 0
	infectedSites := 0

	for i, target := range targets {
		if isFleet && flagFormat != "json" {
			if flagLabel {
				fmt.Printf("\033[36m%s\033[0m\n", target.Label)
			} else {
				fmt.Printf("\r\033[K\033[90mScanning [%d/%d] %s...\033[0m", i+1, len(targets), target.Label)
			}
		}

		// Run wordfence malware-scan on the entire quicksave directory
		scanArgs := []string{"malware-scan", "--output-format", "csv", "--output-columns", "filename,signature_id,signature_name,signature_description,matched_text", "--output-headers", "--quiet", "--no-banner", target.ScanPath}
		scanCmd := exec.Command("wordfence", scanArgs...)
		var scanOut bytes.Buffer
		scanCmd.Stdout = &scanOut
		scanCmd.Stderr = io.Discard
		scanCmd.Run()

		csvOutput := strings.TrimSpace(scanOut.String())
		if csvOutput == "" {
			if flagLabel && isFleet {
				fmt.Printf("  \033[32m✓\033[0m No findings\n\n")
			}
			continue
		}

		// Parse CSV output
		reader := csv.NewReader(strings.NewReader(csvOutput))
		headers, err := reader.Read()
		if err != nil {
			continue
		}
		colIndex := make(map[string]int)
		for idx, h := range headers {
			colIndex[h] = idx
		}

		type wfFinding struct {
			Filename             string `json:"filename"`
			SignatureID          string `json:"signature_id"`
			SignatureName        string `json:"signature_name"`
			SignatureDescription string `json:"signature_description"`
			MatchedText          string `json:"matched_text"`
		}

		var findings []wfFinding
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			f := wfFinding{}
			if idx, ok := colIndex["filename"]; ok && idx < len(record) {
				// Make path relative to quicksave dir for readability
				f.Filename, _ = filepath.Rel(target.ScanPath, record[idx])
			}
			if idx, ok := colIndex["signature_id"]; ok && idx < len(record) {
				f.SignatureID = record[idx]
			}
			if idx, ok := colIndex["signature_name"]; ok && idx < len(record) {
				f.SignatureName = record[idx]
			}
			if idx, ok := colIndex["signature_description"]; ok && idx < len(record) {
				f.SignatureDescription = record[idx]
			}
			if idx, ok := colIndex["matched_text"]; ok && idx < len(record) {
				f.MatchedText = record[idx]
			}
			findings = append(findings, f)
		}

		if len(findings) == 0 {
			if flagLabel && isFleet {
				fmt.Printf("  \033[32m✓\033[0m No findings\n\n")
			}
			continue
		}

		infectedSites++
		totalFindings += len(findings)

		if !flagLabel && isFleet && flagFormat != "json" {
			fmt.Print("\r\033[K")
		}

		if flagFormat == "json" {
			jsonOut, _ := json.MarshalIndent(map[string]interface{}{
				"site":     target.Label,
				"findings": findings,
			}, "", "    ")
			fmt.Println(string(jsonOut))
		} else {
			if !flagLabel {
				fmt.Printf("\033[31m✗\033[0m %s — %d finding(s)\n", target.Label, len(findings))
			}
			for _, f := range findings {
				fmt.Printf("  \033[31m[%s]\033[0m %s — %s\n", f.SignatureID, f.SignatureName, f.Filename)
			}
			fmt.Println()
		}
	}

	if flagFormat != "json" {
		if isFleet && !flagLabel {
			fmt.Print("\r\033[K")
		}
		if totalFindings == 0 {
			fmt.Printf("\033[32m✓\033[0m No malware found across %d environment(s).\n", len(targets))
		} else {
			fmt.Printf("Scan complete: %d finding(s) across %d infected environment(s) out of %d scanned.\n", totalFindings, infectedSites, len(targets))
		}
	}
}

// quicksaveMalwareScanSignatures runs the built-in signature scanner against quicksave directories.
func quicksaveMalwareScanSignatures(targets []malwareScanTarget) {
	sigs, err := loadMalwareSignatures()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Compile all regex patterns once
	var compiled []compiledSig
	for _, sig := range sigs {
		var patterns []*regexp.Regexp
		for _, p := range sig.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				fmt.Printf("Warning: invalid regex in signature %s: %s\n", sig.ID, p)
				continue
			}
			patterns = append(patterns, re)
		}
		if len(patterns) > 0 {
			compiled = append(compiled, compiledSig{Sig: sig, Patterns: patterns})
		}
	}

	// Scan each target
	totalFindings := 0
	infectedSites := 0

	// File extensions to scan
	scannableExts := map[string]bool{
		".php": true, ".phtml": true, ".phar": true, ".php5": true,
	}

	isFleet := len(targets) > 1

	for i, target := range targets {
		// Print progress for fleet scans
		if isFleet && flagFormat != "json" {
			if flagLabel {
				fmt.Printf("\033[36m%s\033[0m\n", target.Label)
			} else {
				fmt.Printf("\r\033[K\033[90mScanning [%d/%d] %s...\033[0m", i+1, len(targets), target.Label)
			}
		}

		var siteFindings []malwareFinding

		// Walk plugins/, themes/, mu-plugins/
		for _, subdir := range []string{"plugins", "themes", "mu-plugins"} {
			dirPath := filepath.Join(target.ScanPath, subdir)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				continue
			}

			filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if !scannableExts[ext] {
					return nil
				}
				relPath, _ := filepath.Rel(target.ScanPath, path)
				findings := scanFileForMalware(path, relPath, compiled)
				siteFindings = append(siteFindings, findings...)
				return nil
			})
		}

		if len(siteFindings) > 0 {
			infectedSites++
			totalFindings += len(siteFindings)

			// Clear progress line before printing findings
			if !flagLabel && isFleet && flagFormat != "json" {
				fmt.Print("\r\033[K")
			}

			if flagFormat == "json" {
				jsonOut, _ := json.MarshalIndent(map[string]interface{}{
					"site":     target.Label,
					"findings": siteFindings,
				}, "", "    ")
				fmt.Println(string(jsonOut))
			} else {
				if !flagLabel {
					fmt.Printf("\033[31m✗\033[0m %s — %d finding(s)\n", target.Label, len(siteFindings))
				}
				for _, f := range siteFindings {
					severityColor := "\033[31m" // red for critical
					switch f.Severity {
					case "high":
						severityColor = "\033[33m" // yellow
					case "medium":
						severityColor = "\033[33m"
					case "low":
						severityColor = "\033[34m" // blue
					}
					fmt.Printf("  %s[%s]\033[0m %s — %s:%d\n", severityColor, f.Severity, f.SignatureName, f.File, f.Line)
				}
				fmt.Println()
			}
		} else if flagLabel && isFleet {
			fmt.Printf("  \033[32m✓\033[0m No findings\n\n")
		}
	}

	if flagFormat != "json" {
		// Clear progress line
		if isFleet && !flagLabel {
			fmt.Print("\r\033[K")
		}
		if totalFindings == 0 {
			fmt.Printf("\033[32m✓\033[0m No malware signatures found across %d environment(s).\n", len(targets))
		} else {
			fmt.Printf("Scan complete: %d finding(s) across %d infected environment(s) out of %d scanned.\n", totalFindings, infectedSites, len(targets))
		}
	}
}

var quicksaveArchiveCmd = &cobra.Command{
	Use:   "archive <site> <hash> [--plugin=<name>] [--theme=<name>]",
	Short: "Extract a plugin or theme zip from a quicksave commit",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveArchiveNative)
	},
}

func quicksaveArchiveNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	hash := args[1]

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintln(os.Stderr, "Error: Environment not found.")
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
		return
	}

	// Determine type and name
	var typePrefix, name string
	if flagPlugin != "" {
		typePrefix = "plugins/"
		name = flagPlugin
	} else if flagTheme != "" {
		typePrefix = "themes/"
		name = flagTheme
	} else {
		fmt.Fprintln(os.Stderr, "Error: Must specify --plugin or --theme.")
		return
	}

	// Sanitize name to prevent path traversal
	sanitized := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(name, "")
	if sanitized != name || name == "" {
		fmt.Fprintln(os.Stderr, "Error: Invalid name. Only alphanumeric characters, hyphens, and underscores are allowed.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	quicksaveDir := filepath.Join(system.Path, siteDir, envName, "quicksave")

	gitCmd := exec.Command("git", "archive", "--format=zip", "--prefix="+name+"/", hash+":"+typePrefix+name+"/")
	gitCmd.Dir = quicksaveDir
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	if err := gitCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create archive: %v\n", err)
	}
}

var quicksaveDatabaseCmd = &cobra.Command{
	Use:   "database <site> <hash>",
	Short: "Extract and sanitize database SQL from the nearest backup snapshot for a quicksave",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveDatabaseNative)
	},
}

func quicksaveDatabaseNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	hash := args[1]

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintln(os.Stderr, "Error: Environment not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	quicksaveDir := filepath.Join(system.Path, siteDir, envName, "quicksave")

	// 1. Get quicksave timestamp from git log
	gitLogCmd := exec.Command("git", "log", "--format=%ct", hash, "-n", "1")
	gitLogCmd.Dir = quicksaveDir
	tsOutput, err := gitLogCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not get timestamp for commit %s: %v\n", hash, err)
		return
	}

	qsTimestamp, err := strconv.ParseInt(strings.TrimSpace(string(tsOutput)), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid timestamp for commit %s.\n", hash)
		return
	}

	// 2. Read backup list.json to find nearest snapshot by timestamp
	backupsDir := filepath.Join(system.Path, siteDir, envName, "backups")
	listPath := filepath.Join(backupsDir, "list.json")

	// Regenerate list.json if missing
	if info, statErr := os.Stat(listPath); statErr != nil || info.Size() == 0 {
		siteEnvArg := fmt.Sprintf("%s-%s", site.Site, envName)
		listGenCmd := exec.Command("captaincore", "backup", "list-generate", siteEnvArg, "--captain-id="+captainID)
		listGenCmd.Stderr = os.Stderr
		listGenCmd.Run()
	}

	listData, err := os.ReadFile(listPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: No backup list found. Run 'captaincore backup list-generate' first.")
		return
	}

	var snapshots []struct {
		ShortID string `json:"short_id"`
		ID      string `json:"id"`
		Time    string `json:"time"`
	}
	if json.Unmarshal(listData, &snapshots) != nil || len(snapshots) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No backup snapshots found.")
		return
	}

	// Find nearest snapshot by timestamp
	var bestID, bestShortID string
	bestDelta := int64(1<<63 - 1)
	for _, snap := range snapshots {
		t, parseErr := time.Parse(time.RFC3339Nano, snap.Time)
		if parseErr != nil {
			t, parseErr = time.Parse("2006-01-02T15:04:05Z07:00", snap.Time)
		}
		if parseErr != nil {
			continue
		}
		delta := qsTimestamp - t.Unix()
		if delta < 0 {
			delta = -delta
		}
		if delta < bestDelta {
			bestDelta = delta
			bestID = snap.ID
			bestShortID = snap.ShortID
		}
	}

	if bestID == "" {
		fmt.Fprintln(os.Stderr, "Error: Could not find a matching backup snapshot.")
		return
	}

	fmt.Fprintf(os.Stderr, "Using backup snapshot %s (delta: %s)\n", bestShortID, secondsToTimeString(bestDelta))

	// 3. Check cache first
	cacheDir := filepath.Join(system.Path, siteDir, envName, "sandbox-cache")
	cachePath := filepath.Join(cacheDir, bestShortID+".sql")

	if _, err := os.Stat(cachePath); err == nil {
		// Cache hit — stream cached file to stdout
		f, err := os.Open(cachePath)
		if err == nil {
			io.Copy(os.Stdout, f)
			f.Close()
			return
		}
	}

	// 4. Restore database-backup.sql from restic to temp dir
	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)

	tmpDir, err := os.MkdirTemp("", "captaincore-db-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not create temp directory: %v\n", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	restoreCmd := exec.Command("restic", "restore", bestID,
		"--include=/database-backup.sql",
		"--repo", resticRepo,
		"--password-file="+resticKey,
		"--target", tmpDir,
	)
	restoreCmd.Stderr = os.Stderr
	if err := restoreCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to restore from restic: %v\n", err)
		return
	}

	sqlPath := filepath.Join(tmpDir, "database-backup.sql")
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Error: database-backup.sql not found in snapshot.")
		return
	}

	// 5. Sanitize SQL and stream to stdout, also cache the result
	os.MkdirAll(cacheDir, 0755)
	cacheFile, cacheErr := os.Create(cachePath)
	if cacheErr != nil {
		cacheFile = nil
	}
	defer func() {
		if cacheFile != nil {
			cacheFile.Close()
		}
	}()

	sanitizeDatabaseSQL(sqlPath, os.Stdout, cacheFile)
}

// sanitizeDatabaseSQL reads a WordPress SQL dump line-by-line, sanitizes it for
// WordPress Playground, and writes the result to the provided writers.
func sanitizeDatabaseSQL(sqlPath string, stdout io.Writer, cache *os.File) {
	f, err := os.Open(sqlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not open SQL file: %v\n", err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow up to 10MB per line for large INSERT statements
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	detectedPrefix := ""
	inMultiLineComment := false
	inSkippedInsert := false
	inMultiValueInsert := false   // inside a multi-value INSERT VALUES block
	multiValueInsertPrefix := ""  // "INSERT INTO `table` VALUES " for rewriting rows

	// Regex patterns for prefix detection from known WP tables
	// Captures prefix WITH trailing underscore (e.g. "wp_", "custom_")
	prefixRe := regexp.MustCompile(`(?:CREATE TABLE|INSERT INTO)\s+` + "`?" + `([a-zA-Z0-9_]+_)(options|posts|users|postmeta|comments|terms|usermeta)` + "`?")
	// Strips everything after ) ENGINE= on CREATE TABLE closing lines
	engineRe := regexp.MustCompile(`\)\s*ENGINE=.*;\s*$`)

	// Patterns for wp_options rows to skip (transients, caches, large ephemeral data)
	optionsSkipPatterns := []string{
		"'_transient_",
		"'_site_transient_",
		"'_transient_timeout_",
		"'_site_transient_timeout_",
		"'edd_sl_",
		"'_wc_session_",
	}

	// Max line length for value rows (50KB) — safety net for parser memory
	const maxValueLineLen = 50 * 1024

	writeLine := func(line string) {
		fmt.Fprintln(stdout, line)
		if cache != nil {
			fmt.Fprintln(cache, line)
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Handle multi-line /* ... */ comment blocks
		if inMultiLineComment {
			if strings.Contains(trimmed, "*/") {
				inMultiLineComment = false
			}
			continue
		}

		// Skip SQL single-line comments
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Skip MySQL/MariaDB comment-directives: /*!...*/ and /*M!...*/
		if strings.HasPrefix(trimmed, "/*!") || strings.HasPrefix(trimmed, "/*M!") || strings.HasPrefix(trimmed, "/*M") {
			if !strings.Contains(trimmed, "*/") {
				inMultiLineComment = true
			}
			continue
		}

		// Skip any other multi-line comment start
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inMultiLineComment = true
			}
			continue
		}

		// Auto-detect prefix from CREATE TABLE or INSERT INTO
		if detectedPrefix == "" {
			if m := prefixRe.FindStringSubmatch(line); m != nil {
				detectedPrefix = m[1]
				fmt.Fprintf(os.Stderr, "Detected table prefix: %s\n", detectedPrefix)
			}
		}

		// Skip SET directives
		if strings.HasPrefix(trimmed, "SET @OLD_") || strings.HasPrefix(trimmed, "SET NAMES") || strings.HasPrefix(trimmed, "SET TIME_ZONE") || strings.HasPrefix(trimmed, "SET @") {
			continue
		}

		// Skip LOCK/UNLOCK TABLES
		if strings.HasPrefix(trimmed, "LOCK TABLES") || strings.HasPrefix(trimmed, "UNLOCK TABLES") {
			continue
		}

		// Handle continuation lines of skipped multi-line INSERT statements
		if inSkippedInsert {
			if strings.HasSuffix(trimmed, ";") {
				inSkippedInsert = false
			}
			continue
		}

		// Handle multi-value INSERT — convert each value row to individual INSERT
		if inMultiValueInsert {
			isLastRow := strings.HasSuffix(trimmed, ";")
			if isLastRow {
				inMultiValueInsert = false
			}

			// Skip oversized rows
			if len(trimmed) > maxValueLineLen {
				continue
			}

			// For options table, skip transient rows
			if strings.Contains(multiValueInsertPrefix, "options`") {
				skip := false
				for _, pat := range optionsSkipPatterns {
					if strings.Contains(trimmed, pat) {
						skip = true
						break
					}
				}
				if skip {
					continue
				}
			}

			// Extract the value tuple — strip trailing comma or semicolon
			row := strings.TrimRight(trimmed, ",;")
			// Write as individual INSERT statement
			writeLine(multiValueInsertPrefix + row + ";")
			continue
		}

		// Detect multi-value INSERT INTO — rewrite as single-row INSERTs
		if strings.Contains(trimmed, "INSERT INTO") && !strings.HasSuffix(trimmed, ";") {
			// This is a multi-value INSERT (VALUES on next lines)
			// Extract "INSERT INTO `table` VALUES " prefix

			// Skip users/usermeta tables
			if detectedPrefix != "" {
				usersTable := "`" + detectedPrefix + "users`"
				usermetaTable := "`" + detectedPrefix + "usermeta`"
				if strings.Contains(line, usersTable) || strings.Contains(line, usermetaTable) {
					inSkippedInsert = true
					continue
				}
			}

			// Build the INSERT prefix for rewriting individual rows
			prefix := trimmed
			if detectedPrefix != "" && detectedPrefix != "wp_" {
				prefix = strings.ReplaceAll(prefix, detectedPrefix, "wp_")
			}
			// Ensure it ends with a space for clean concatenation
			if !strings.HasSuffix(prefix, " ") {
				prefix += " "
			}
			multiValueInsertPrefix = prefix
			inMultiValueInsert = true
			continue
		}

		// Skip single-line INSERT INTO users/usermeta
		if detectedPrefix != "" && strings.Contains(trimmed, "INSERT INTO") {
			usersTable := "`" + detectedPrefix + "users`"
			usermetaTable := "`" + detectedPrefix + "usermeta`"
			if strings.Contains(line, usersTable) || strings.Contains(line, usermetaTable) {
				continue
			}
		}

		// Strip ENGINE=... and everything after it on CREATE TABLE closing lines
		if strings.HasPrefix(trimmed, ")") && strings.Contains(line, "ENGINE=") {
			line = engineRe.ReplaceAllString(line, ");")
		}

		// Rewrite table prefix to wp_ (prefix includes trailing underscore)
		if detectedPrefix != "" && detectedPrefix != "wp_" {
			line = strings.ReplaceAll(line, detectedPrefix, "wp_")
		}

		writeLine(line)
	}

	// Append synthetic admin user/usermeta at the end.
	// DELETE + INSERT ensures ID 1 is our admin regardless of existing data.
	syntheticSQL := []string{
		"DELETE FROM `wp_users` WHERE ID = 1;",
		"INSERT INTO `wp_users` VALUES (1,'admin','$P$BPMnfKMfChGsSTiECcPMwHAVmczNDu.','admin','admin@example.com','','2024-01-01 00:00:00','',0,'admin');",
		"DELETE FROM `wp_usermeta` WHERE user_id = 1;",
		"INSERT INTO `wp_usermeta` (umeta_id, user_id, meta_key, meta_value) VALUES (1,1,'wp_capabilities','a:1:{s:13:\"administrator\";b:1;}');",
		"INSERT INTO `wp_usermeta` (umeta_id, user_id, meta_key, meta_value) VALUES (2,1,'wp_user_level','10');",
	}

	for _, s := range syntheticSQL {
		writeLine(s)
	}
}

// ---------------------------------------------------------------------------
// quicksave migrate-v2
// ---------------------------------------------------------------------------

var quicksaveMigrateV2Cmd = &cobra.Command{
	Use:   "migrate-v2 <site>",
	Short: "Migrates quicksave restic repo from v1 to v2 with compression",
	Long: `Orchestrates a full v1 to v2 migration for the quicksave repo:
  1. Checks if repo is already v2 (skips unless --force)
  2. Upgrades repo to v2 (restic migrate upgrade_repo_v2)
  3. Repacks all uncompressed data (restic prune --repack-uncompressed)
  4. Clears local restic cache for this repo
  5. Verifies final repo state

Uses system path_tmp for TMPDIR (large repos need 100s of GBs).`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveMigrateV2Native)
	},
}

// quicksaveMigrateV2Native implements `captaincore quicksave migrate-v2 <site>` natively in Go.
func quicksaveMigrateV2Native(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Println("Error: Environment not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/quicksave-repo", rcloneBackup, siteDir, envName)
	siteLabel := fmt.Sprintf("%s-%s", site.Site, envName)

	rcloneArgs := []string{
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
		"-o", "rclone.timeout=600s",
	}

	// Build restic env with TMPDIR from config
	resticEnv := os.Environ()
	if system.PathTmp != "" {
		resticEnv = append(resticEnv, "TMPDIR="+system.PathTmp)
		fmt.Printf("Using TMPDIR=%s\n", system.PathTmp)
	}

	// -----------------------------------------------------------
	// Step 1: Pre-flight — check current repo version & get repo ID
	// -----------------------------------------------------------
	fmt.Printf("\n[1/5] Checking repo version for %s\n", siteLabel)

	catArgs := append([]string{"cat", "config", "--repo", resticRepo, "--password-file=" + resticKey, "--json"}, rcloneArgs...)
	catCmd := exec.Command("restic", catArgs...)
	catCmd.Env = resticEnv
	catOutput, err := catCmd.Output()
	if err != nil {
		fmt.Printf("Quicksave repo not found for %s. Skipping.\n", siteLabel)
		return
	}

	var repoConfig struct {
		Version int    `json:"version"`
		ID      string `json:"id"`
	}
	if json.Unmarshal(catOutput, &repoConfig) != nil {
		fmt.Println("Error: Failed to parse repo config.")
		return
	}

	// Get repo size before migration
	rclonePath := fmt.Sprintf("%s/%s/%s/quicksave-repo", rcloneBackup, siteDir, envName)
	type rcloneSize struct {
		Count int64  `json:"count"`
		Bytes uint64 `json:"bytes"`
	}
	var repoSizeBefore rcloneSize
	rcloneSizeCmd := exec.Command("rclone", "size", "--json", rclonePath)
	if rcloneSizeOutput, err := rcloneSizeCmd.Output(); err == nil {
		json.Unmarshal(rcloneSizeOutput, &repoSizeBefore)
	}
	fmt.Printf("Repo version: %d, size: %s (%d objects)\n", repoConfig.Version, formatBytes(strconv.FormatUint(repoSizeBefore.Bytes, 10)), repoSizeBefore.Count)

	if repoConfig.Version >= 2 && !flagForce {
		fmt.Printf("Repo is already version 2. Nothing to do. (use --force to re-run)\n")
		return
	}

	if repoConfig.Version >= 2 && flagForce {
		fmt.Printf("--force specified. Continuing.\n")
	} else {
		fmt.Printf("Proceeding with migration.\n")
	}

	// -----------------------------------------------------------
	// Step 2: Upgrade repo to v2
	// -----------------------------------------------------------
	fmt.Printf("\n[2/5] Upgrading repo to v2 for %s\n", siteLabel)

	upgradeArgs := append([]string{
		"migrate", "upgrade_repo_v2",
		"--repo", resticRepo,
		"--password-file=" + resticKey,
	}, rcloneArgs...)

	upgradeCmd := exec.Command("restic", upgradeArgs...)
	upgradeCmd.Env = resticEnv
	upgradeCmd.Stdout = os.Stdout
	upgradeCmd.Stderr = os.Stderr
	if err := upgradeCmd.Run(); err != nil {
		if repoConfig.Version >= 2 {
			fmt.Println("Migration already applied, continuing.")
		} else {
			fmt.Printf("Error: Upgrade failed for %s. Aborting.\n", siteLabel)
			return
		}
	}

	// -----------------------------------------------------------
	// Step 3: Repack uncompressed data
	// -----------------------------------------------------------
	skipRepack, _ := cmd.Flags().GetBool("skip-repack")
	if skipRepack {
		fmt.Printf("\n[3/5] Skipping repack (--skip-repack)\n")
	} else {
		fmt.Printf("\n[3/5] Repacking uncompressed data for %s (this may take a while)\n", siteLabel)

		pruneArgs := append([]string{
			"prune", "--repack-uncompressed",
			"--repo", resticRepo,
			"--password-file=" + resticKey,
		}, rcloneArgs...)

		pruneCmd := exec.Command("restic", pruneArgs...)
		pruneCmd.Env = resticEnv
		pruneCmd.Stdout = os.Stdout
		pruneCmd.Stderr = os.Stderr
		if err := pruneCmd.Run(); err != nil {
			fmt.Printf("Error: Repack failed for %s.\n", siteLabel)
			return
		}
	}

	// -----------------------------------------------------------
	// Step 4: Clear local restic cache for this quicksave repo
	// -----------------------------------------------------------
	skipCacheCleanup, _ := cmd.Flags().GetBool("skip-cache-cleanup")
	if skipCacheCleanup {
		fmt.Printf("\n[4/5] Skipping cache cleanup (--skip-cache-cleanup)\n")
	} else {
		fmt.Printf("\n[4/5] Clearing local restic cache for %s\n", siteLabel)

		if repoConfig.ID != "" {
			home, _ := os.UserHomeDir()
			cachePath := filepath.Join(home, ".cache", "restic", repoConfig.ID)
			if info, err := os.Stat(cachePath); err == nil && info.IsDir() {
				cacheSize, _ := dirSize(cachePath)
				fmt.Printf("Local cache: %s\n", formatBytes(strconv.FormatInt(cacheSize, 10)))
				if err := os.RemoveAll(cachePath); err != nil {
					fmt.Printf("Warning: Failed to remove cache at %s: %v\n", cachePath, err)
				} else {
					fmt.Println("Cache cleared.")
				}
			} else {
				fmt.Println("No local cache found.")
			}
		} else {
			fmt.Println("Warning: Could not determine repo ID for cache cleanup.")
		}
	}

	// -----------------------------------------------------------
	// Step 5: Verify final state
	// -----------------------------------------------------------
	fmt.Printf("\n[5/5] Verifying repo for %s\n", siteLabel)

	verifyCatCmd := exec.Command("restic", catArgs...)
	verifyCatCmd.Env = resticEnv
	verifyOutput, err := verifyCatCmd.Output()
	if err != nil {
		fmt.Printf("Warning: Could not verify repo config after migration.\n")
	} else {
		var verifyConfig struct {
			Version int `json:"version"`
		}
		if json.Unmarshal(verifyOutput, &verifyConfig) == nil {
			fmt.Printf("Repo version: %d\n", verifyConfig.Version)
		}
	}

	// Get repo size after migration
	var repoSizeAfter rcloneSize
	rcloneSizeAfterCmd := exec.Command("rclone", "size", "--json", rclonePath)
	if rcloneSizeOutput, err := rcloneSizeAfterCmd.Output(); err == nil {
		json.Unmarshal(rcloneSizeOutput, &repoSizeAfter)
	}
	fmt.Printf("Repo size: %s -> %s (%d objects)\n",
		formatBytes(strconv.FormatUint(repoSizeBefore.Bytes, 10)),
		formatBytes(strconv.FormatUint(repoSizeAfter.Bytes, 10)),
		repoSizeAfter.Count)
	if repoSizeBefore.Bytes > 0 && repoSizeAfter.Bytes < repoSizeBefore.Bytes {
		saved := repoSizeBefore.Bytes - repoSizeAfter.Bytes
		pct := float64(saved) / float64(repoSizeBefore.Bytes) * 100
		fmt.Printf("Saved: %s (%.1f%%)\n", formatBytes(strconv.FormatUint(saved, 10)), pct)
	}

	fmt.Printf("\nMigration complete for %s.\n", siteLabel)
}

// ---------------------------------------------------------------------------
// quicksave cache-purge
// ---------------------------------------------------------------------------

var quicksaveCachePurgeCmd = &cobra.Command{
	Use:   "cache-purge <site>",
	Short: "Delete local restic cache for this site's quicksave repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveCachePurgeNative)
	},
}

// quicksaveCachePurgeNative implements `captaincore quicksave cache-purge <site>`.
func quicksaveCachePurgeNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Println("Error: Environment not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/quicksave-repo", rcloneBackup, siteDir, envName)
	siteLabel := fmt.Sprintf("%s-%s", site.Site, envName)

	rcloneArgs := []string{
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
		"-o", "rclone.timeout=600s",
	}

	// Get repo ID from config
	catArgs := append([]string{"cat", "config", "--repo", resticRepo, "--password-file=" + resticKey, "--json"}, rcloneArgs...)
	catCmd := exec.Command("restic", catArgs...)
	catOutput, err := catCmd.Output()
	if err != nil {
		fmt.Printf("Quicksave repo not found for %s. Skipping.\n", siteLabel)
		return
	}

	var repoConfig struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(catOutput, &repoConfig) != nil || repoConfig.ID == "" {
		fmt.Println("Error: Failed to parse repo config.")
		return
	}

	home, _ := os.UserHomeDir()
	cachePath := filepath.Join(home, ".cache", "restic", repoConfig.ID)
	info, err := os.Stat(cachePath)
	if err != nil || !info.IsDir() {
		fmt.Printf("No local cache found for %s.\n", siteLabel)
		return
	}

	cacheSize, _ := dirSize(cachePath)
	fmt.Printf("%s\t%s\n", formatBytes(strconv.FormatInt(cacheSize, 10)), siteLabel)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		return
	}

	if err := os.RemoveAll(cachePath); err != nil {
		fmt.Printf("Error: Failed to remove cache at %s: %v\n", cachePath, err)
		return
	}
	fmt.Printf("Cache cleared for %s.\n", siteLabel)
}

// ---------------------------------------------------------------------------
// quicksave cache-check
// ---------------------------------------------------------------------------

var quicksaveCacheCheckCmd = &cobra.Command{
	Use:   "cache-check <site>",
	Short: "Report local cache size for this site's quicksave repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveCacheCheckNative)
	},
}

// quicksaveCacheCheckNative implements `captaincore quicksave cache-check <site>`.
func quicksaveCacheCheckNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Println("Error: Environment not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/quicksave-repo", rcloneBackup, siteDir, envName)
	siteLabel := fmt.Sprintf("%s-%s", site.Site, envName)

	rcloneArgs := []string{
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
		"-o", "rclone.timeout=600s",
	}

	// Get repo ID from config
	catArgs := append([]string{"cat", "config", "--repo", resticRepo, "--password-file=" + resticKey, "--json"}, rcloneArgs...)
	catCmd := exec.Command("restic", catArgs...)
	catOutput, err := catCmd.Output()
	if err != nil {
		fmt.Printf("Quicksave repo not found for %s. Skipping.\n", siteLabel)
		return
	}

	var repoConfig struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(catOutput, &repoConfig) != nil || repoConfig.ID == "" {
		fmt.Println("Error: Failed to parse repo config.")
		return
	}

	home, _ := os.UserHomeDir()
	cachePath := filepath.Join(home, ".cache", "restic", repoConfig.ID)
	info, err := os.Stat(cachePath)
	if err != nil || !info.IsDir() {
		fmt.Printf("No local cache found for %s.\n", siteLabel)
		return
	}

	cacheSize, _ := dirSize(cachePath)

	formatFlag, _ := cmd.Flags().GetString("format")
	if formatFlag == "json" {
		jsonOut, _ := json.Marshal(map[string]interface{}{
			"site":       siteLabel,
			"repo_id":    repoConfig.ID,
			"cache_path": cachePath,
			"cache_bytes": cacheSize,
			"cache_size": formatBytes(strconv.FormatInt(cacheSize, 10)),
		})
		fmt.Println(string(jsonOut))
	} else {
		fmt.Printf("%s\t%s\n", formatBytes(strconv.FormatInt(cacheSize, 10)), siteLabel)
	}
}

// ---------------------------------------------------------------------------
// quicksave unlock
// ---------------------------------------------------------------------------

var quicksaveUnlockCmd = &cobra.Command{
	Use:   "unlock <site>",
	Short: "Removes stale locks from a quicksave restic repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveUnlockNative)
	},
}

// quicksaveUnlockNative implements `captaincore quicksave unlock <site>`.
func quicksaveUnlockNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Println("Error: Environment not found.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/quicksave-repo", rcloneBackup, siteDir, envName)

	fmt.Printf("Unlocking quicksave repo for %s-%s\n", site.Site, envName)

	resticArgs := []string{
		"unlock",
		"--repo", resticRepo,
		"--password-file=" + resticKey,
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
		"-o", "rclone.timeout=600s",
	}

	resticCmd := exec.Command("restic", resticArgs...)
	resticCmd.Stdout = os.Stdout
	resticCmd.Stderr = os.Stderr
	resticCmd.Run()
}

func init() {
	rootCmd.AddCommand(quicksaveCmd)
	quicksaveCmd.AddCommand(quicksaveAddCmd)
	quicksaveCmd.AddCommand(quicksaveBackupCmd)
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
	quicksaveCmd.AddCommand(quicksaveMalwareScanCmd)
	quicksaveCmd.AddCommand(quicksaveArchiveCmd)
	quicksaveCmd.AddCommand(quicksaveDatabaseCmd)
	quicksaveCmd.AddCommand(quicksaveMigrateV2Cmd)
	quicksaveCmd.AddCommand(quicksaveCachePurgeCmd)
	quicksaveCmd.AddCommand(quicksaveCacheCheckCmd)
	quicksaveCmd.AddCommand(quicksaveUnlockCmd)
	quicksaveMigrateV2Cmd.Flags().BoolVar(&flagForce, "force", false, "Run even if repo is already v2")
	quicksaveMigrateV2Cmd.Flags().Bool("skip-repack", false, "Skip the repack step (upgrade only)")
	quicksaveMigrateV2Cmd.Flags().Bool("skip-cache-cleanup", false, "Skip local cache cleanup")
	quicksaveCachePurgeCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Report size only, no deletion")
	quicksaveCacheCheckCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Output format (json)")
	quicksaveArchiveCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin slug")
	quicksaveArchiveCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme slug")
	quicksaveMalwareScanCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Output format (json)")
	quicksaveMalwareScanCmd.Flags().BoolVar(&flagFull, "full", false, "Run Wordfence CLI scan on entire quicksave directory")
	quicksaveMalwareScanCmd.Flags().BoolVar(&flagLabel, "label", false, "Print colored site name headers in bulk mode")
	quicksaveFileDiffCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme slug")
	quicksaveFileDiffCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin slug")
	quicksaveLatestCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveListCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveRollbackCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagVersion, "version", "", "Rollback to 'this' or 'previous' version (default \"this\")")
	quicksaveRollbackCmd.Flags().StringVar(&flagFile, "file", "", "File to rollback")
	quicksaveRollbackCmd.Flags().BoolVar(&flagAll, "all", false, "All themes and plugins")
	quicksaveAddCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force even if no changes")
	quicksaveFileDiffCmd.Flags().BoolVar(&flagHtml, "html", false, "Returns HTML format")
	quicksaveBackupCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 10, "Number of sites to run at same time")
	quicksaveBackupCmd.Flags().StringVarP(&flagSkipIfRecent, "skip-if-recent", "", "", "Skip if restic snapshot exists within timeframe (e.g. 24h, 7d)")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force a new Quicksave")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
	quicksaveGenerateCmd.Flags().StringVarP(&flagSkipIfRecent, "skip-if-recent", "", "", "Skip if quicksave generated within timeframe (e.g. 24h)")
	quicksaveGenerateCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview which environments would be processed without executing")
	quicksaveGenerateCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 10, "Number of sites to run at same time")
}
