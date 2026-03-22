package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/apiclient"
	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
)

// SiteArg holds parsed components from a "site-environment@provider" argument.
type SiteArg struct {
	SiteName    string
	Environment string // defaults to "production"
	Provider    string
}

// parseSiteArgument parses a "site-environment@provider" string into its components.
func parseSiteArgument(arg string) SiteArg {
	sa := SiteArg{SiteName: arg, Environment: "production"}

	// Parse site-environment format
	if strings.Contains(sa.SiteName, "-") {
		parts := strings.SplitN(sa.SiteName, "-", 2)
		sa.SiteName = parts[0]
		sa.Environment = parts[1]
	}

	// Parse @provider from site name
	if strings.Contains(sa.SiteName, "@") {
		parts := strings.SplitN(sa.SiteName, "@", 2)
		sa.SiteName = parts[0]
		sa.Provider = parts[1]
	}

	// Parse @provider from environment
	if strings.Contains(sa.Environment, "@") {
		parts := strings.SplitN(sa.Environment, "@", 2)
		sa.Environment = parts[0]
		sa.Provider = parts[1]
	}

	return sa
}

// LookupSite finds the site in the database matching this SiteArg.
func (sa SiteArg) LookupSite() (*models.Site, error) {
	if sa.Provider != "" {
		return models.GetSiteByNameAndProvider(sa.SiteName, sa.Provider)
	}
	if id, err := strconv.ParseUint(sa.SiteName, 10, 64); err == nil {
		return models.GetSiteByID(uint(id))
	}
	return models.GetSiteByName(sa.SiteName)
}

// LookupEnvironment finds the matching environment for a site.
func (sa SiteArg) LookupEnvironment(siteID uint) (*models.Environment, error) {
	environments, err := models.FindEnvironmentsBySiteID(siteID)
	if err != nil {
		return nil, err
	}
	for i, e := range environments {
		if strings.EqualFold(e.Environment, sa.Environment) {
			return &environments[i], nil
		}
	}
	return nil, nil
}

// loadCaptainConfig loads config.json and returns system + captain configs for the current captain ID.
func loadCaptainConfig() (config.FullConfig, *config.SystemConfig, *config.CaptainConfig, error) {
	configs, err := config.LoadConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	system := configs.GetSystem()
	captain := configs.GetCaptain(captainID)

	// Adjust path for fleet mode
	if system != nil && system.CaptainCoreFleet == "true" {
		system.Path = system.Path + "/" + captainID
	}

	return configs, system, captain, nil
}

// newAPIClient creates an API client from the config.
func newAPIClient(system *config.SystemConfig, captain *config.CaptainConfig) *apiclient.Client {
	apiURL := ""
	token := ""
	skipSSL := false

	if captain != nil {
		if v, ok := captain.Vars["captaincore_api"]; ok {
			json.Unmarshal(v, &apiURL)
		}
		if v, ok := captain.Keys["token"]; ok {
			token = v
		}
	}
	if system != nil && system.CaptainCoreDev != "" && system.CaptainCoreDev != "false" {
		skipSSL = true
	}

	return apiclient.NewClient(apiURL, token, skipSSL)
}

// getRcloneBackup returns the rclone backup remote path, adjusted for fleet mode.
func getRcloneBackup(captain *config.CaptainConfig, system *config.SystemConfig) string {
	backup := ""
	if captain != nil {
		backup = captain.Remotes["rclone_backup"]
	}
	if backup == "" && system != nil {
		backup = system.RcloneBackup
	}
	if system != nil && system.CaptainCoreFleet == "true" {
		backup = backup + "/" + captainID
	}
	return backup
}

// getResticKeyPath returns the path to the restic password key file.
func getResticKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".captaincore", "data", "restic.key")
}

// getBackupLockPath returns the path to the backup lock file for a site/environment.
func getBackupLockPath(siteName string, siteID uint, envName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".captaincore", "data", fmt.Sprintf("%s_%d", siteName, siteID), envName, "backup.lock")
}

// acquireBackupLock attempts to acquire the backup lock for a site/environment.
// Returns true if the lock was acquired, false if another process holds it.
func acquireBackupLock(lockPath string) bool {
	// Check if lock file exists and process is still running
	if data, err := os.ReadFile(lockPath); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pidStr != "" {
			// Check if process is still alive
			checkCmd := fmt.Sprintf("/proc/%s", pidStr)
			if _, err := os.Stat(checkCmd); err == nil {
				return false
			}
			// Also try kill -0 approach for macOS compatibility
			if pid, err := strconv.Atoi(pidStr); err == nil {
				process, err := os.FindProcess(pid)
				if err == nil {
					// On Unix, FindProcess always succeeds; use Signal(0) to check
					if err := process.Signal(os.Signal(nil)); err == nil {
						return false
					}
				}
			}
			// Stale lock, remove it
		}
	}

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(lockPath), 0755)

	// Write our PID
	return os.WriteFile(lockPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644) == nil
}

// releaseBackupLock removes the backup lock file.
func releaseBackupLock(lockPath string) {
	os.Remove(lockPath)
}

// updateEnvironmentDetails merges updates into environment details JSON, saves to DB, and posts to API.
func updateEnvironmentDetails(envID uint, siteID uint, updates map[string]interface{}, system *config.SystemConfig, captain *config.CaptainConfig) error {
	env, err := models.GetEnvironmentByID(envID)
	if err != nil {
		return err
	}

	var details map[string]interface{}
	if env.Details != "" {
		json.Unmarshal([]byte(env.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}

	for k, v := range updates {
		if v == nil {
			delete(details, k)
		} else {
			details[k] = v
		}
	}

	detailsJSON, _ := json.Marshal(details)
	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	models.DB.Model(&models.Environment{}).Where("environment_id = ?", envID).Updates(map[string]interface{}{
		"details":    string(detailsJSON),
		"updated_at": timeNow,
	})

	client := newAPIClient(system, captain)
	envUpdate := map[string]interface{}{
		"environment_id": envID,
		"details":        string(detailsJSON),
		"updated_at":     timeNow,
	}
	client.Post("update-environment", map[string]interface{}{
		"site_id": siteID,
		"data":    envUpdate,
	})

	return nil
}

// updateSiteDetails merges updates into site details JSON, saves to DB, and posts to API.
// Pass nil values to delete keys from the details object.
func updateSiteDetails(siteID uint, updates map[string]interface{}, system *config.SystemConfig, captain *config.CaptainConfig) error {
	site, err := models.GetSiteByID(siteID)
	if err != nil {
		return err
	}

	var details map[string]interface{}
	if site.Details != "" {
		json.Unmarshal([]byte(site.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}

	for k, v := range updates {
		if v == nil {
			delete(details, k)
		} else {
			details[k] = v
		}
	}

	detailsJSON, _ := json.Marshal(details)
	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	models.DB.Model(&models.Site{}).Where("site_id = ?", siteID).Updates(map[string]interface{}{
		"details":    string(detailsJSON),
		"updated_at": timeNow,
	})

	client := newAPIClient(system, captain)
	siteUpdate := map[string]interface{}{
		"site_id": siteID,
		"details": string(detailsJSON),
	}
	client.Post("update-site", map[string]interface{}{
		"site_id": siteID,
		"data":    siteUpdate,
	})

	return nil
}

// getVarString extracts a string value from a captain's vars map.
func getVarString(captain *config.CaptainConfig, key string) string {
	if captain == nil {
		return ""
	}
	v, ok := captain.Vars[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(v))
}

// b2AuthorizeDownload performs the two-step Backblaze B2 authorization flow
// and returns a download authorization token.
func b2AuthorizeDownload(accountID, accountKey, bucketID, fileNamePrefix string) (string, error) {
	// Step 1: Authorize account
	credentials := base64.StdEncoding.EncodeToString([]byte(accountID + ":" + accountKey))
	req, err := http.NewRequest("GET", "https://api.backblazeb2.com/b2api/v1/b2_authorize_account", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+credentials)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var authResp struct {
		AuthorizationToken string `json:"authorizationToken"`
	}
	if err := json.Unmarshal(body, &authResp); err != nil {
		return "", err
	}

	// Step 2: Get download authorization
	postData, _ := json.Marshal(map[string]interface{}{
		"bucketId":               bucketID,
		"validDurationInSeconds": 604800,
		"fileNamePrefix":         fileNamePrefix,
	})

	req2, err := http.NewRequest("POST", "https://api001.backblazeb2.com/b2api/v1/b2_get_download_authorization", bytes.NewReader(postData))
	if err != nil {
		return "", err
	}
	req2.Header.Set("Authorization", authResp.AuthorizationToken)

	resp2, err := client.Do(req2)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", err
	}

	var dlResp struct {
		AuthorizationToken string `json:"authorizationToken"`
	}
	if err := json.Unmarshal(body2, &dlResp); err != nil {
		return "", err
	}

	return dlResp.AuthorizationToken, nil
}

// parseThresholdDuration converts a human-friendly threshold string into a time.Duration.
// Supports "24h" (hours), "7d" (days), "30m" (minutes).
func parseThresholdDuration(threshold string) (time.Duration, error) {
	threshold = strings.TrimSpace(strings.ToLower(threshold))
	if len(threshold) < 2 {
		return 0, fmt.Errorf("invalid threshold: %s", threshold)
	}
	unit := threshold[len(threshold)-1]
	numStr := threshold[:len(threshold)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid threshold number: %s", threshold)
	}
	switch unit {
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'm':
		return time.Duration(num) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unsupported threshold unit: %c", unit)
	}
}

// checkLastRun replicates lib/local-scripts/check-last-run.php in Go.
// Returns true if the site should be skipped (was run recently within threshold).
func checkLastRun(listFilePath string, threshold string) bool {
	dur, err := parseThresholdDuration(threshold)
	if err != nil {
		return false
	}

	info, err := os.Stat(listFilePath)
	if err != nil {
		return false
	}
	fileMtime := info.ModTime()

	// Parse JSON content for timestamps
	data, err := os.ReadFile(listFilePath)
	if err != nil {
		return false
	}

	var lastEntryTime time.Time

	var items []map[string]interface{}
	if json.Unmarshal(data, &items) == nil && len(items) > 0 {
		for _, item := range items {
			if createdAt, ok := item["created_at"]; ok {
				var ts int64
				switch v := createdAt.(type) {
				case float64:
					ts = int64(v)
				case string:
					ts, _ = strconv.ParseInt(v, 10, 64)
				}
				if ts > 0 {
					t := time.Unix(ts, 0)
					if t.After(lastEntryTime) {
						lastEntryTime = t
					}
				}
			} else if timeStr, ok := item["time"].(string); ok && timeStr != "" {
				t, parseErr := time.Parse(time.RFC3339Nano, timeStr)
				if parseErr != nil {
					t, parseErr = time.Parse("2006-01-02T15:04:05Z07:00", timeStr)
				}
				if parseErr == nil && t.After(lastEntryTime) {
					lastEntryTime = t
				}
			}
		}
	}

	// Determine most recent activity
	lastActivity := fileMtime
	if lastEntryTime.After(lastActivity) {
		lastActivity = lastEntryTime
	}

	cutoff := time.Now().Add(-dur)
	return lastActivity.After(cutoff)
}

// dryRunGenerate previews which environments would be processed by generate commands.
// listSubdir is "quicksaves" or "backups".
func dryRunGenerate(target string, listSubdir string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

	// Check if target looks like a bulk target (@all, @production, @staging) or single site
	isBulk := strings.HasPrefix(target, "@")

	if isBulk {
		environment, minorTargets := models.ParseTargetString(target)
		queryArgs := models.FetchSiteMatchingArgs{
			Environment: environment,
			Targets:     minorTargets,
		}
		results, err := models.FetchSitesMatching(queryArgs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		_, system, _, err := loadCaptainConfig()
		if err != nil || system == nil {
			fmt.Println("Error: Configuration file not found.")
			return
		}

		var toDo []string
		skipped := 0

		for _, r := range results {
			envLower := strings.ToLower(r.Environment)
			siteEnv := fmt.Sprintf("%s-%s", r.Site, envLower)
			siteDir := fmt.Sprintf("%s_%d", r.Site, r.SiteID)
			listPath := filepath.Join(system.Path, siteDir, envLower, listSubdir, "list.json")

			if flagSkipIfRecent != "" && checkLastRun(listPath, flagSkipIfRecent) {
				skipped++
			} else {
				toDo = append(toDo, siteEnv)
			}
		}

		if flagSkipIfRecent != "" {
			fmt.Println("\nEnvironments to process:")
			for _, env := range toDo {
				fmt.Printf("  %s\n", env)
			}
			fmt.Printf("\nenvironments skipped: %d\n", skipped)
			fmt.Printf("environments to do: %d\n", len(toDo))
		} else {
			fmt.Printf("\nenvironments to do: %d\n", len(toDo))
			fmt.Println("(no --skip-if-recent specified, all environments would be processed)")
		}
	} else {
		// Single site target
		sa := parseSiteArgument(target)
		site, err := sa.LookupSite()
		if err != nil || site == nil {
			fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
			return
		}

		env, err := sa.LookupEnvironment(site.SiteID)
		if err != nil || env == nil {
			fmt.Printf("Error: Environment not found for '%s'.\n", target)
			return
		}

		_, system, _, err := loadCaptainConfig()
		if err != nil || system == nil {
			fmt.Println("Error: Configuration file not found.")
			return
		}

		envLower := strings.ToLower(env.Environment)
		siteEnv := fmt.Sprintf("%s-%s", site.Site, envLower)
		siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
		listPath := filepath.Join(system.Path, siteDir, envLower, listSubdir, "list.json")

		if flagSkipIfRecent != "" && checkLastRun(listPath, flagSkipIfRecent) {
			fmt.Printf("\n%s would be skipped (recent activity within %s)\n", siteEnv, flagSkipIfRecent)
			fmt.Printf("\nenvironments skipped: 1\n")
			fmt.Printf("environments to do: 0\n")
		} else {
			fmt.Println("\nEnvironments to process:")
			fmt.Printf("  %s\n", siteEnv)
			if flagSkipIfRecent != "" {
				fmt.Printf("\nenvironments skipped: 0\n")
			}
			fmt.Printf("environments to do: 1\n")
		}
	}
}

// getB2SnapshotsPath returns the B2 snapshots bucket path and the folder prefix
// used for download authorization. Adjusts for fleet mode.
func getB2SnapshotsPath(captain *config.CaptainConfig, system *config.SystemConfig) (b2Snapshots, b2Folder string) {
	if captain != nil {
		b2Snapshots = captain.Remotes["b2_snapshots"]
	}
	// Derive from rclone_snapshot (e.g. "Remote:Bucket/Path" → "Bucket/Path") when b2_snapshots is not set
	if b2Snapshots == "" && system != nil && system.RcloneSnapshot != "" {
		if idx := strings.Index(system.RcloneSnapshot, ":"); idx >= 0 {
			b2Snapshots = system.RcloneSnapshot[idx+1:]
		}
	}
	if system != nil && system.CaptainCoreFleet == "true" {
		b2Snapshots = b2Snapshots + "/" + captainID
	}
	idx := strings.Index(b2Snapshots, "/")
	if idx >= 0 {
		b2Folder = b2Snapshots[idx+1:]
	}
	return
}

// secondsToTimeString converts seconds into a human-readable duration string.
func secondsToTimeString(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	if seconds < 3600 {
		minutes := seconds / 60
		secs := seconds % 60
		return fmt.Sprintf("%d minutes and %d seconds", minutes, secs)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%d hours, %d minutes and %d seconds", hours, minutes, secs)
}

// dirSize walks a directory tree and returns the total size of all files in bytes.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// formatDateTimeHuman formats a time as "January 2nd 2006 3:04 pm" (matching PHP's 'F jS Y g:i a').
func formatDateTimeHuman(t time.Time) string {
	day := t.Day()
	suffix := "th"
	if day%10 == 1 && day != 11 {
		suffix = "st"
	} else if day%10 == 2 && day != 12 {
		suffix = "nd"
	} else if day%10 == 3 && day != 13 {
		suffix = "rd"
	}
	return fmt.Sprintf("%s %d%s %s", t.Format("January"), day, suffix, t.Format("2006 3:04 pm"))
}
