package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

// MonitorRecord represents an entry in monitor.json.
type MonitorRecord struct {
	URL         string `json:"url"`
	Name        string `json:"name"`
	HTTPCode    string `json:"http_code"`
	HTMLValid   string `json:"html_valid"`
	Error       string `json:"error,omitempty"`
	CheckCount  int    `json:"check_count"`
	NotifyCount int    `json:"notify_count"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// MonitorEmailItem holds data for a single row in the notification email.
type MonitorEmailItem struct {
	Name      string
	URL       string
	HTTPCode  string
	TimeAgo   string
	Detail    string
	CreatedAt int64
}

// monitorRunChecks runs health checks in parallel using a bounded worker pool.
// Results are returned in the same order as the input URLs.
// The optional onResult callback is invoked for each result as it arrives.
// The timeout parameter controls per-request timeout (0 uses the default 15s).
// The transport parameter selects the HTTP transport (nil uses sharedTransport).
func monitorRunChecks(urls []string, parallelism int, timeout time.Duration, transport *http.Transport, onResult func(MonitorCheckResult)) []MonitorCheckResult {
	type indexedResult struct {
		index  int
		result MonitorCheckResult
	}

	results := make([]MonitorCheckResult, len(urls))
	ch := make(chan indexedResult, len(urls))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i, entry := range urls {
		wg.Add(1)
		go func(idx int, e string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			parts := strings.SplitN(e, ",", 2)
			url := parts[0]
			name := ""
			if len(parts) > 1 {
				name = parts[1]
			}
			ch <- indexedResult{index: idx, result: monitorCheckSingle(url, name, timeout, transport)}
		}(i, entry)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for ir := range ch {
		results[ir.index] = ir.result
		if onResult != nil {
			onResult(ir.result)
		}
	}
	return results
}

// monitorNative is the native Go implementation of the monitor command.
func monitorNative(cmd *cobra.Command, args []string) {
	// Load config
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	// Initialize monitor stats DB
	if err := models.InitMonitorDB(); err != nil {
		fmt.Println("Error initializing monitor DB:", err)
		os.Exit(1)
	}

	parallelism := monitorParallel
	if parallelism == 0 {
		parallelism = 10
	}
	retryMax := monitorRetry
	if retryMax == 0 {
		retryMax = 3
	}

	// Determine paths
	pathTmp := "/tmp"
	if system != nil && system.PathTmp != "" {
		pathTmp = system.PathTmp
	}
	dataPath := ""
	if system != nil && system.Path != "" {
		dataPath = system.Path
	}
	logsPath := ""
	if system != nil && system.Logs != "" {
		logsPath = system.Logs
	}

	adminEmail := getVarString(captain, "captaincore_admin_email")
	lockFile := filepath.Join(pathTmp, "captaincore-monitor.lock")

	// ---- Lock file management ----
	if data, err := os.ReadFile(lockFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			// Check if process is alive
			if err := syscall.Kill(pid, 0); err == nil {
				fmt.Printf("Skipping monitor run: Previous monitor job (PID %d) is currently running.\n", pid)
				models.LogMonitorRunSkipped(parallelism)
				return
			}
			// Stale lock file
			fmt.Printf("Found stale lock file. Previous job (PID %d) died without cleanup. Proceeding.\n", pid)
			if adminEmail != "" {
				alertBody := fmt.Sprintf("A stale lock file was detected for PID %d.<br /><br />This indicates the previous monitor job crashed or was killed abruptly.<br /><br />The lock file has been cleared and the monitor job is restarting.", pid)
				client := newAPIClient(system, captain)
				contentJSON, _ := json.Marshal(alertBody)
				client.Post("monitor-notify", map[string]interface{}{
					"data": map[string]interface{}{
						"email":   adminEmail,
						"subject": "Monitor Alert: Stale Lock File Detected (Previous Crash)",
						"content": json.RawMessage(contentJSON),
					},
				})
			}
		}
	}

	// Write lock file
	if err := os.MkdirAll(filepath.Dir(lockFile), 0755); err == nil {
		os.WriteFile(lockFile, []byte(strconv.Itoa(os.Getpid())), 0644)
	}

	// Cleanup function
	cleanup := func() {
		os.Remove(lockFile)
	}
	defer cleanup()

	// Signal handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cleanup()
		os.Exit(0)
	}()

	// Start logging
	runID := models.LogMonitorRunStart("monitor", parallelism)

	// ---- Build URL list ----
	var urlsToCheck []string
	page := flagPage

	isTarget := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "@") {
			isTarget = true
			break
		}
	}

	if isTarget {
		// Use FetchSitesMatching for target groups
		for _, arg := range args {
			if !strings.HasPrefix(arg, "@") {
				continue
			}
			environment, minorTargets := models.ParseTargetString(arg)
			fetchArgs := models.FetchSiteMatchingArgs{
				Environment: environment,
				Targets:     minorTargets,
			}
			results, err := models.FetchSitesMatching(fetchArgs)
			if err != nil {
				fmt.Println("Error fetching sites:", err)
				continue
			}
			for _, r := range results {
				if r.HomeURL == "" {
					continue
				}
				siteName := r.Site
				if r.Environment != "" && !strings.EqualFold(r.Environment, "production") {
					siteName = r.Site + "-" + strings.ToLower(r.Environment)
				}
				url := r.HomeURL + page
				urlsToCheck = append(urlsToCheck, url+","+siteName)
			}
		}
	} else {
		// Individual sites
		for _, arg := range args {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			sa := parseSiteArgument(arg)
			site, err := sa.LookupSite()
			if err != nil || site == nil {
				continue
			}
			env, err := sa.LookupEnvironment(site.SiteID)
			if err != nil || env == nil || env.HomeURL == "" {
				continue
			}
			url := env.HomeURL + page
			urlsToCheck = append(urlsToCheck, url+","+arg)
		}
	}

	if len(urlsToCheck) == 0 {
		fmt.Println("\x1b[31mError:\x1b[39m Nothing to check")
		models.LogMonitorRunEnd(runID, 0, 0, 0, 0, false)
		return
	}

	// Store original list for generate step
	originalURLs := make([]string, len(urlsToCheck))
	copy(originalURLs, urlsToCheck)

	siteCount := len(urlsToCheck)

	// Generate log file path
	now := time.Now()
	auth := fmt.Sprintf("%07x", rand.Intn(0x10000000))
	logFileName := fmt.Sprintf("%s_%s_%s.txt", now.Format("2006-01-02"), now.Format("15-04"), auth)
	logFile := filepath.Join(logsPath, logFileName)
	if logsPath != "" {
		os.MkdirAll(logsPath, 0755)
	}

	monitorFile := filepath.Join(dataPath, "monitor.json")

	fmt.Println("logging to", logFile)

	// Open log file for streaming results as they arrive
	var logWriter *os.File
	if logFile != "" {
		logWriter, _ = os.Create(logFile)
	}
	defer func() {
		if logWriter != nil {
			logWriter.Close()
		}
	}()

	// streamResult writes a single check result to stdout and the log file.
	streamResult := func(r MonitorCheckResult) {
		line, _ := json.Marshal(r)
		fmt.Println(string(line))
		if logWriter != nil {
			logWriter.WriteString(string(line) + "\n")
		}
	}

	// ---- Retry loop ----
	var allResults []MonitorCheckResult
	retryAttempts := 0
	for attempt := 1; attempt <= retryMax; attempt++ {
		if attempt > 1 {
			time.Sleep(10 * time.Second)
		}

		// Use 15s timeout for early attempts, 60s for the final attempt
		checkTimeout := 15 * time.Second
		if attempt == retryMax {
			checkTimeout = 60 * time.Second
		}

		// First attempt uses system DNS; retries use 1.1.1.1 to rule out local DNS issues
		transport := sharedTransport
		if attempt > 1 {
			transport = retryTransport
		}

		results := monitorRunChecks(urlsToCheck, parallelism, checkTimeout, transport, streamResult)

		// On first attempt, store full results
		if attempt == 1 {
			allResults = make([]MonitorCheckResult, len(results))
			copy(allResults, results)
		} else {
			// Merge retry results back into allResults
			retryMap := make(map[string]MonitorCheckResult)
			for _, r := range results {
				retryMap[r.URL] = r
			}
			for i, r := range allResults {
				if updated, ok := retryMap[r.URL]; ok {
					allResults[i] = updated
				}
			}
		}

		// Count errors
		errorCount := 0
		var failedURLs []string
		for _, r := range results {
			if r.HTMLValid == "false" || (r.HTTPCode != "200" && r.HTTPCode != "301") {
				errorCount++
				failedURLs = append(failedURLs, r.URL+","+r.Name)
			}
		}

		if errorCount == 0 {
			break
		}

		retryAttempts = attempt
		if attempt < retryMax {
			fmt.Printf("Attempt #%d found %d errors. Checking those URLs again.\n", attempt, errorCount)
			urlsToCheck = failedURLs
		} else {
			fmt.Printf("Attempt #%d found %d errors.\n", attempt, errorCount)
		}
	}

	// ---- State management (replaces monitor.php generate) ----
	emailContent, restoredCount := monitorGenerate(allResults, monitorFile, originalURLs, captain, system)

	// Count final errors
	finalErrorCount := 0
	for _, r := range allResults {
		if r.HTMLValid == "false" || (r.HTTPCode != "200" && r.HTTPCode != "301") {
			finalErrorCount++
		}
	}

	notificationSent := emailContent != ""

	if notificationSent {
		fmt.Println("Sending monitor alert email")

		// Build a descriptive subject line
		var subjectParts []string
		if finalErrorCount == 1 {
			subjectParts = append(subjectParts, "1 error")
		} else if finalErrorCount > 1 {
			subjectParts = append(subjectParts, fmt.Sprintf("%d errors", finalErrorCount))
		}
		if restoredCount == 1 {
			subjectParts = append(subjectParts, "1 site restored")
		} else if restoredCount > 1 {
			subjectParts = append(subjectParts, fmt.Sprintf("%d sites restored", restoredCount))
		}
		subject := "Monitor: " + strings.Join(subjectParts, ", ")

		contentJSON, _ := json.Marshal(emailContent)
		client := newAPIClient(system, captain)
		resp, err := client.Post("monitor-notify", map[string]interface{}{
			"data": map[string]interface{}{
				"email":   adminEmail,
				"subject": subject,
				"content": json.RawMessage(contentJSON),
			},
		})
		if err != nil {
			fmt.Printf("Error sending monitor email: %v\n", err)
		} else {
			fmt.Printf("Monitor email API response: %s\n", string(resp))
		}
	}

	// Finalize
	models.LogMonitorRunEnd(runID, siteCount, finalErrorCount, retryAttempts, restoredCount, notificationSent)
}

// monitorGenerate processes results, updates monitor.json, and returns HTML email content.
func monitorGenerate(logErrors []MonitorCheckResult, monitorFile string, originalURLs []string, captain *config.CaptainConfig, system *config.SystemConfig) (string, int) {
	timeNow := time.Now().Unix()

	// Build set of checked site names from originalURLs
	checkedNames := make(map[string]bool)
	for _, entry := range originalURLs {
		parts := strings.SplitN(entry, ",", 2)
		if len(parts) > 1 {
			checkedNames[parts[1]] = true
		}
	}

	// Filter log errors (same logic as PHP process_log)
	var filteredErrors []MonitorCheckResult
	for _, r := range logErrors {
		if r.HTMLValid == "false" || (r.HTTPCode != "200" && r.HTTPCode != "301") {
			filteredErrors = append(filteredErrors, r)
		}
	}

	// Load existing monitor.json
	var monitorRecords []MonitorRecord
	if data, err := os.ReadFile(monitorFile); err == nil {
		json.Unmarshal(data, &monitorRecords)
	}

	// Build set of error URLs for fast lookup
	errorURLs := make(map[string]bool)
	for _, e := range filteredErrors {
		errorURLs[e.URL] = true
	}

	// Store new errors in monitor records
	for _, logError := range filteredErrors {
		found := false
		for i := range monitorRecords {
			if monitorRecords[i].URL == logError.URL {
				monitorRecords[i].CheckCount++
				monitorRecords[i].UpdatedAt = timeNow
				monitorRecords[i].HTTPCode = logError.HTTPCode
				monitorRecords[i].HTMLValid = logError.HTMLValid
				monitorRecords[i].Error = logError.Error
				found = true
				break
			}
		}
		if !found {
			monitorRecords = append(monitorRecords, MonitorRecord{
				URL:         logError.URL,
				Name:        logError.Name,
				HTTPCode:    logError.HTTPCode,
				HTMLValid:   logError.HTMLValid,
				Error:       logError.Error,
				CheckCount:  1,
				NotifyCount: 0,
				CreatedAt:   timeNow,
				UpdatedAt:   timeNow,
			})
		}
	}

	// Notification time thresholds (matching PHP: 1 hour, 4 hours, 24 hours)
	notifyThresholds := []time.Duration{1 * time.Hour, 4 * time.Hour, 24 * time.Hour}

	var emailErrors []MonitorEmailItem
	var knownErrors []MonitorEmailItem
	var restored []MonitorEmailItem
	var warnings []MonitorEmailItem

	// Process monitor records
	var keepRecords []MonitorRecord
	for _, record := range monitorRecords {
		// If existing record not in original check and has been notified, remove it
		if !checkedNames[record.Name] && record.NotifyCount != 0 {
			continue
		}

		// Check if site is now online (not in error list)
		if !errorURLs[record.URL] {
			restored = append(restored, MonitorEmailItem{
				Name:      record.Name,
				URL:       record.URL,
				HTTPCode:  record.HTTPCode,
				TimeAgo:   "offline since " + time.Unix(record.CreatedAt, 0).Format("January 2, 2006, 3:04 pm"),
				CreatedAt: record.CreatedAt,
			})
			continue
		}

		// Check if notifications count is exceeded (beyond 24hrs)
		if record.NotifyCount >= len(notifyThresholds) {
			knownErrors = append(knownErrors, MonitorEmailItem{
				Name:      record.Name,
				URL:       record.URL,
				HTTPCode:  record.HTTPCode,
				TimeAgo:   timeElapsedString(record.CreatedAt),
				CreatedAt: record.CreatedAt,
			})
			keepRecords = append(keepRecords, record)
			continue
		}

		// WordPress 5.2 bug: 301 on first detection triggers Kinsta cache purge
		if record.NotifyCount == 0 && record.HTTPCode == "301" {
			exec.Command("captaincore", "ssh", record.Name, "--command=wp kinsta cache purge --all", "--captain-id="+captainID).Run()
		}

		// Calculate notification time check
		var notifyTimeCheck int64
		if record.NotifyCount == 0 {
			notifyTimeCheck = record.CreatedAt
		} else {
			threshold := notifyThresholds[record.NotifyCount-1]
			notifyTimeCheck = time.Now().Add(-threshold).Unix()
		}

		// Check if "notify at" time is ready
		if record.CreatedAt > notifyTimeCheck {
			knownErrors = append(knownErrors, MonitorEmailItem{
				Name:      record.Name,
				URL:       record.URL,
				HTTPCode:  record.HTTPCode,
				TimeAgo:   timeElapsedString(record.CreatedAt),
				CreatedAt: record.CreatedAt,
			})
			keepRecords = append(keepRecords, record)
			continue
		}

		// HTML invalid
		if record.HTMLValid == "false" {
			detail := "HTML is invalid"
			if record.Error != "" {
				detail = record.Error
			}
			record.NotifyCount++
			emailErrors = append(emailErrors, MonitorEmailItem{
				Name:      record.Name,
				URL:       record.URL,
				HTTPCode:  record.HTTPCode,
				TimeAgo:   timeElapsedString(record.CreatedAt),
				Detail:    detail,
				CreatedAt: record.CreatedAt,
			})
			keepRecords = append(keepRecords, record)
			continue
		}

		// 301 redirect warning
		if record.HTTPCode == "301" {
			warnings = append(warnings, MonitorEmailItem{
				Name:      record.Name,
				URL:       record.URL,
				HTTPCode:  record.HTTPCode,
				CreatedAt: record.CreatedAt,
			})
			keepRecords = append(keepRecords, record)
			continue
		}

		// General error
		record.NotifyCount++
		emailErrors = append(emailErrors, MonitorEmailItem{
			Name:      record.Name,
			URL:       record.URL,
			HTTPCode:  record.HTTPCode,
			TimeAgo:   timeElapsedString(record.CreatedAt),
			Detail:    record.Error,
			CreatedAt: record.CreatedAt,
		})
		keepRecords = append(keepRecords, record)
	}

	// Sort errors newest first (by original CreatedAt timestamp)
	sort.SliceStable(emailErrors, func(i, j int) bool {
		return emailErrors[i].CreatedAt > emailErrors[j].CreatedAt
	})
	sort.SliceStable(knownErrors, func(i, j int) bool {
		return knownErrors[i].CreatedAt > knownErrors[j].CreatedAt
	})

	// Save updated monitor.json atomically
	monitorJSON, _ := json.MarshalIndent(keepRecords, "", "    ")
	tmpFile := monitorFile + ".tmp"
	if err := os.WriteFile(tmpFile, monitorJSON, 0644); err == nil {
		os.Rename(tmpFile, monitorFile)
	}

	restoredCount := len(restored)

	// Build HTML email if there are errors or restored sites
	if len(emailErrors) == 0 && restoredCount == 0 {
		return "", restoredCount
	}

	var html strings.Builder
	html.WriteString("<div style='text-align: left;'>")
	html.WriteString(buildEmailSection("Errors", emailErrors, "#FED7D7", "#9B2C2C", "http_code"))
	html.WriteString(buildEmailSection("Restored", restored, "#C6F6D5", "#22543D", "label"))
	html.WriteString(buildEmailSection("Warnings", warnings, "#FEFCBF", "#975A16", "http_code"))
	html.WriteString(buildEmailSection("Ongoing Errors", knownErrors, "#E2E8F0", "#4A5568", "http_code"))
	html.WriteString("</div>")

	return html.String(), restoredCount
}

// buildEmailSiteRow generates a single table row for the notification email.
func buildEmailSiteRow(item MonitorEmailItem, badgeBG, badgeColor, badgeText string) string {
	detailHTML := ""
	if item.Detail != "" {
		detailHTML = fmt.Sprintf("<div style='font-size: 12px; color: #e53e3e; margin-top: 2px;'>%s</div>", item.Detail)
	}
	timeHTML := ""
	if item.TimeAgo != "" {
		timeHTML = fmt.Sprintf("<div style='font-size: 12px; color: #a0aec0; margin-top: 4px;'>%s</div>", item.TimeAgo)
	}
	return fmt.Sprintf(`
			<tr>
				<td style='padding: 12px 15px; border-bottom: 1px solid #edf2f7; vertical-align: top;'>
					<div style='font-weight: 600; color: #2d3748;'>%s</div>
					<div style='font-size: 13px; margin-top: 2px;'><a href='%s' style='color: #718096; text-decoration: none;'>%s</a></div>
					%s
					%s
				</td>
				<td style='padding: 12px 15px; border-bottom: 1px solid #edf2f7; text-align: right; vertical-align: top; white-space: nowrap;'>
					<span style='display: inline-block; background-color: %s; color: %s; font-size: 11px; font-weight: 700; padding: 3px 8px; border-radius: 9999px;'>%s</span>
				</td>
			</tr>`, item.Name, item.URL, item.URL, detailHTML, timeHTML, badgeBG, badgeColor, badgeText)
}

// buildEmailSection generates a section of the notification email.
func buildEmailSection(title string, items []MonitorEmailItem, badgeBG, badgeColor, badgeField string) string {
	if len(items) == 0 {
		return ""
	}
	var rows strings.Builder
	for _, item := range items {
		badgeText := item.HTTPCode
		if badgeField == "label" {
			badgeText = title
		}
		rows.WriteString(buildEmailSiteRow(item, badgeBG, badgeColor, badgeText))
	}
	return fmt.Sprintf(`
			<div style='margin-bottom: 25px;'>
				<h3 style='margin: 0 0 10px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: #a0aec0;'>%s</h3>
				<div style='background-color: #ffffff; border: 1px solid #e2e8f0; border-radius: 6px; overflow: hidden;'>
					<table width='100%%' cellpadding='0' cellspacing='0'>
						%s
					</table>
				</div>
			</div>`, title, rows.String())
}

// timeElapsedString returns a human-readable "X units ago" string using the largest unit only.
func timeElapsedString(timestamp int64) string {
	diff := time.Since(time.Unix(timestamp, 0))
	if diff < 0 {
		return "just now"
	}

	totalSeconds := int64(diff.Seconds())
	totalMinutes := totalSeconds / 60
	totalHours := totalMinutes / 60
	totalDays := totalHours / 24

	years := totalDays / 365
	if years > 0 {
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}

	months := totalDays / 30
	if months > 0 {
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}

	weeks := totalDays / 7
	if weeks > 0 {
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	}

	days := totalDays % 7
	if totalDays > 0 && days == 0 {
		days = totalDays
	}
	if days > 0 {
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}

	hours := totalHours
	if hours > 0 {
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	minutes := totalMinutes
	if minutes > 0 {
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}

	if totalSeconds > 0 {
		if totalSeconds == 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", totalSeconds)
	}

	return "just now"
}

var monitorParallel, monitorRetry int
var monitorStatsLimit int

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor commands for up-time checks and stats",
}

var monitorRunCmd = &cobra.Command{
	Use:   "run <site|target>",
	Short: "Runs up-time check on one or more sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, monitorNative)
	},
}

var monitorStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "List recent monitor runs from the stats database",
	Run: func(cmd *cobra.Command, args []string) {
		if err := models.InitMonitorDB(); err != nil {
			fmt.Println("Error initializing monitor DB:", err)
			os.Exit(1)
		}

		var runs []models.MonitorRun
		models.MonitorDB.Order("id desc").Limit(monitorStatsLimit).Find(&runs)

		if len(runs) == 0 {
			fmt.Println("No monitor runs found.")
			return
		}

		fmt.Printf("%-6s  %-16s  %-7s  %-10s  %-28s  %-12s  %-8s  %-9s  %-8s  %-10s  %-20s\n",
			"ID", "Status", "Sites", "Parallel", "Duration", "System Load", "Errors", "Retries", "Notify", "Restored", "Started")
		fmt.Println("------  ----------------  -------  ----------  ----------------------------  ------------  --------  ---------  --------  ----------  --------------------")

		for _, r := range runs {
			started := time.Unix(r.StartTime, 0).Format(time.DateTime)
			duration := ""
			if r.Duration > 0 {
				duration = secondsToTimeString(r.Duration)
			}
			notify := "no"
			if r.NotificationSent {
				notify = "yes"
			}
			fmt.Printf("%-6d  %-16s  %-7d  %-10d  %-28s  %-12.2f  %-8d  %-9d  %-8s  %-10d  %-20s\n",
				r.ID, r.Status, r.SiteCount, r.Parallelism, duration, r.SystemLoad, r.ErrorCount, r.RetryAttempts, notify, r.RestoredCount, started)
		}
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
	monitorCmd.AddCommand(monitorRunCmd)
	monitorCmd.AddCommand(monitorStatsCmd)
	monitorRunCmd.Flags().IntVarP(&monitorParallel, "parallel", "p", 10, "Number of monitor checks to run at same time")
	monitorRunCmd.Flags().IntVarP(&monitorRetry, "retry", "r", 3, "Number of retries for failures")
	monitorRunCmd.Flags().StringVarP(&flagPage, "page", "", "", "Check a specific page, example: --page=/wp-admin/")
	monitorStatsCmd.Flags().IntVarP(&monitorStatsLimit, "limit", "l", 20, "Number of monitor runs to show")
}
