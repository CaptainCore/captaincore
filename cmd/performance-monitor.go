package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var performanceMonitorCmd = &cobra.Command{
	Use:   "performance-monitor",
	Short: "Manage background performance monitoring on sites",
}

var performanceMonitorActivateCmd = &cobra.Command{
	Use:   "activate <site>",
	Short: "Deploys performance monitor scripts to a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, performanceMonitorActivateNative)
	},
}

var performanceMonitorDeactivateCmd = &cobra.Command{
	Use:   "deactivate <site>",
	Short: "Removes performance monitor scripts from a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, performanceMonitorDeactivateNative)
	},
}

var performanceMonitorFetchCmd = &cobra.Command{
	Use:   "fetch <site>",
	Short: "Fetches and parses performance monitor data from a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, performanceMonitorFetchNative)
	},
}

func performanceMonitorActivateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
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

	if env.DatabaseUsername == "" || env.DatabasePassword == "" {
		fmt.Println("Error: Database credentials not found for this environment.")
		return
	}

	if env.HomeURL == "" {
		fmt.Println("Error: Home URL not found for this environment.")
		return
	}

	// Derive private directory from home directory
	privateDir := derivePrivateDir(env.HomeDirectory)

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)
	fmt.Printf("Activating performance monitor on %s...\n", siteEnvArg)

	sshCmd := exec.Command("captaincore", "ssh", siteEnvArg,
		"--script=performance-monitor-deploy",
		"--",
		"--db_user="+env.DatabaseUsername,
		"--db_pass="+env.DatabasePassword,
		"--home_url="+env.HomeURL,
		"--private_dir="+privateDir,
		"--username="+env.Username)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Run()

	// Update environment details
	updates := map[string]interface{}{
		"performance_monitor_enabled": true,
	}
	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, updates, system, captain)

	fmt.Println("Performance monitor activated.")
}

func performanceMonitorDeactivateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
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

	privateDir := derivePrivateDir(env.HomeDirectory)

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)
	fmt.Printf("Deactivating performance monitor on %s...\n", siteEnvArg)

	sshCmd := exec.Command("captaincore", "ssh", siteEnvArg,
		"--script=performance-monitor-remove",
		"--",
		"--private_dir="+privateDir)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Run()

	// Update environment details
	updates := map[string]interface{}{
		"performance_monitor_enabled": false,
	}
	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, updates, system, captain)

	fmt.Println("Performance monitor deactivated.")
}

func performanceMonitorFetchNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Println("Error: Environment not found.")
		return
	}

	privateDir := derivePrivateDir(env.HomeDirectory)

	// ~120 samples per hour (every 30 seconds). Scale tail lines to requested hours.
	hours, _ := cmd.Flags().GetInt("hours")
	tailLines := 0 // 0 = fetch all
	if hours > 0 {
		tailLines = hours * 120 * 2 // 2x buffer to ensure we capture enough
	}

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)
	tailCmd := "cat"
	if tailLines > 0 {
		tailCmd = fmt.Sprintf("tail -%d", tailLines)
	}
	sshCmd := exec.Command("captaincore", "ssh", siteEnvArg,
		fmt.Sprintf("--command=%s %s/php-monitor.log", tailCmd, privateDir))
	output, err := sshCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching monitor data: %v\n", err)
		return
	}

	logContent := strings.TrimSpace(string(output))

	format, _ := cmd.Flags().GetString("format")
	if format == "raw" {
		rawData := parseMonitorLogRaw(logContent, hours)
		// Use MarshalIndent so each line stays under 64KB for the dispatch server's bufio.Scanner
		jsonOut, _ := json.MarshalIndent(rawData, "", " ")
		fmt.Println(string(jsonOut))
		return
	}

	data := parseMonitorLog(logContent)

	// Trim to requested hours if we got more data than requested
	if hours > 0 {
		maxSamples := hours * 120
		data = trimMonitorData(logContent, maxSamples)
	}

	jsonOut, _ := json.Marshal(data)
	fmt.Println(string(jsonOut))
}

// derivePrivateDir computes the private directory path from the home directory.
// For Kinsta sites, the home directory is like /www/site_123/public — we replace public with private.
// For other providers, we use ~/private as a fallback.
func derivePrivateDir(homeDir string) string {
	homeDir = strings.TrimSuffix(homeDir, "/")
	if strings.HasSuffix(homeDir, "/public") {
		return strings.TrimSuffix(homeDir, "public") + "private"
	}
	return "~/private"
}

// monitorSample represents a single parsed log entry.
type monitorSample struct {
	Hour          int
	Minute        int
	Second        int
	DBConns       int
	Load          float64
	HTTPCode      int
	ResponseTime  float64
	ActiveWorkers int
	MaxWorkers    int
}

// monitorBucket represents an aggregated time bucket for charting.
type monitorBucket struct {
	Label       string  `json:"label"`
	DBAvg       float64 `json:"db_avg"`
	DBMax       int     `json:"db_max"`
	LoadAvg     float64 `json:"load_avg"`
	LoadMax     float64 `json:"load_max"`
	ResponseAvg float64 `json:"response_avg"`
	ResponseMax float64 `json:"response_max"`
	HTTPCode    int     `json:"http_code"`
}

// monitorData is the JSON output structure.
type monitorData struct {
	Labels       []string  `json:"labels"`
	DBAvg        []float64 `json:"db_avg"`
	DBMax        []int     `json:"db_max"`
	LoadAvg      []float64 `json:"load_avg"`
	LoadMax      []float64 `json:"load_max"`
	ResponseAvg  []float64 `json:"response_avg"`
	ResponseMax  []float64 `json:"response_max"`
	WorkersAvg   []float64 `json:"workers_avg"`
	WorkersMax   []int     `json:"workers_max"`
	TotalSamples int       `json:"total_samples"`
	TotalDays    int       `json:"total_days"`
	BucketLabel  string    `json:"bucket_label"`
	TotalTimeout int       `json:"total_timeouts"`
	PeakDB       int       `json:"peak_db"`
	PeakLoad     float64   `json:"peak_load"`
	AvgResponse  float64   `json:"avg_response"`
	PeakWorkers  int       `json:"peak_workers"`
	MaxWorkers   int       `json:"max_workers"`
}

// Matches both old format (without workers) and new format (with workers)
var monitorLineRegex = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2}) \| (\d+) \| ([\d.]+) \| (\d+) ([\d.]+)s(?:\s*\|\s*(\d+)/(\d+))?$`)

func parseMonitorLog(logContent string) monitorData {
	lines := strings.Split(logContent, "\n")

	var samples []monitorSample
	for _, line := range lines {
		line = strings.TrimSpace(line)
		m := monitorLineRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		hh, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		ss, _ := strconv.Atoi(m[3])
		db, _ := strconv.Atoi(m[4])
		load, _ := strconv.ParseFloat(m[5], 64)
		code, _ := strconv.Atoi(m[6])
		resp, _ := strconv.ParseFloat(m[7], 64)

		activeWorkers := 0
		maxWorkers := 0
		if m[8] != "" {
			activeWorkers, _ = strconv.Atoi(m[8])
			maxWorkers, _ = strconv.Atoi(m[9])
		}

		samples = append(samples, monitorSample{
			Hour: hh, Minute: mm, Second: ss,
			DBConns: db, Load: load,
			HTTPCode: code, ResponseTime: resp,
			ActiveWorkers: activeWorkers, MaxWorkers: maxWorkers,
		})
	}

	if len(samples) == 0 {
		return monitorData{}
	}

	// Count midnight rollovers to determine total days
	crossings := 0
	prevHH := samples[len(samples)-1].Hour
	for i := len(samples) - 2; i >= 0; i-- {
		hh := samples[i].Hour
		if hh > prevHH && prevHH <= 1 {
			crossings++
		}
		prevHH = hh
	}
	totalDays := crossings + 1

	// Assign dates by walking forward detecting midnight rollovers
	type datedSample struct {
		dayIndex int // relative day (0-based)
		estHour  int
		estMin   int
		monitorSample
	}

	var dated []datedSample
	curDay := 0
	prevHH = samples[0].Hour
	utcNow := time.Now().UTC()
	_ = utcNow

	for _, s := range samples {
		if s.Hour < prevHH && prevHH >= 22 {
			curDay++
		}
		prevHH = s.Hour
		// Convert UTC to EST (UTC-5)
		estH := (s.Hour - 5 + 24) % 24
		dated = append(dated, datedSample{
			dayIndex:      curDay,
			estHour:       estH,
			estMin:        s.Minute,
			monitorSample: s,
		})
	}

	// Determine bucket size based on duration
	var bucketMinutes int
	var bucketLabel string
	switch {
	case totalDays <= 1:
		bucketMinutes = 5
		bucketLabel = "5-min"
	case totalDays <= 5:
		bucketMinutes = 15
		bucketLabel = "15-min"
	case totalDays <= 14:
		bucketMinutes = 60
		bucketLabel = "1-hour"
	default:
		bucketMinutes = 240
		bucketLabel = "4-hour"
	}

	// Group into buckets
	type bucketKey struct {
		day    int
		period int // period index within the day
	}

	type bucketAccum struct {
		key     bucketKey
		label   string
		db      []int
		load    []float64
		resp    []float64
		codes   []int
		workers []int
	}

	var buckets []bucketAccum
	bucketMap := make(map[bucketKey]int) // key -> index in buckets slice

	for _, ds := range dated {
		var period int
		var label string

		h := ds.estHour
		m := ds.estMin

		switch {
		case bucketMinutes >= 240:
			period = h / 4
			if h == 0 && ds.dayIndex > 0 {
				label = fmt.Sprintf("%d/%d", int(utcNow.Month())-crossings+ds.dayIndex, utcNow.Day()-crossings+ds.dayIndex)
			} else {
				h12 := h % 12
				if h12 == 0 {
					h12 = 12
				}
				ampm := "a"
				if h >= 12 {
					ampm = "p"
				}
				label = fmt.Sprintf("%d%s", h12, ampm)
			}
		case bucketMinutes >= 60:
			period = h
			if h == 0 && ds.dayIndex > 0 {
				startDate := utcNow.AddDate(0, 0, -(crossings - ds.dayIndex))
				label = fmt.Sprintf("%d/%d", int(startDate.Month()), startDate.Day())
			} else {
				h12 := h % 12
				if h12 == 0 {
					h12 = 12
				}
				ampm := "a"
				if h >= 12 {
					ampm = "p"
				}
				label = fmt.Sprintf("%d%s", h12, ampm)
			}
		default:
			bucketM := (m / bucketMinutes) * bucketMinutes
			period = h*60 + bucketM
			h12 := h % 12
			if h12 == 0 {
				h12 = 12
			}
			ampm := "a"
			if h >= 12 {
				ampm = "p"
			}
			label = fmt.Sprintf("%d:%02d%s", h12, bucketM, ampm)
		}

		bk := bucketKey{day: ds.dayIndex, period: period}
		idx, exists := bucketMap[bk]
		if !exists {
			idx = len(buckets)
			bucketMap[bk] = idx
			buckets = append(buckets, bucketAccum{key: bk, label: label})
		}
		buckets[idx].db = append(buckets[idx].db, ds.DBConns)
		buckets[idx].load = append(buckets[idx].load, ds.Load)
		buckets[idx].resp = append(buckets[idx].resp, ds.ResponseTime)
		buckets[idx].codes = append(buckets[idx].codes, ds.HTTPCode)
		buckets[idx].workers = append(buckets[idx].workers, ds.ActiveWorkers)
	}

	// Build output arrays
	result := monitorData{
		TotalSamples: len(samples),
		TotalDays:    totalDays,
		BucketLabel:  bucketLabel,
	}

	var allResp []float64
	for _, b := range buckets {
		result.Labels = append(result.Labels, b.label)

		// DB averages and max
		dbSum := 0
		dbMax := 0
		for _, v := range b.db {
			dbSum += v
			if v > dbMax {
				dbMax = v
			}
		}
		dbAvg := round(float64(dbSum)/float64(len(b.db)), 1)
		result.DBAvg = append(result.DBAvg, dbAvg)
		result.DBMax = append(result.DBMax, dbMax)
		if dbMax > result.PeakDB {
			result.PeakDB = dbMax
		}

		// Load averages and max
		loadSum := 0.0
		loadMax := 0.0
		for _, v := range b.load {
			loadSum += v
			if v > loadMax {
				loadMax = v
			}
		}
		loadAvg := round(loadSum/float64(len(b.load)), 2)
		result.LoadAvg = append(result.LoadAvg, loadAvg)
		result.LoadMax = append(result.LoadMax, round(loadMax, 2))
		if loadMax > result.PeakLoad {
			result.PeakLoad = loadMax
		}

		// Response time averages and max
		respSum := 0.0
		respMax := 0.0
		for _, v := range b.resp {
			respSum += v
			if v > respMax {
				respMax = v
			}
		}
		respAvg := round(respSum/float64(len(b.resp)), 3)
		result.ResponseAvg = append(result.ResponseAvg, respAvg)
		result.ResponseMax = append(result.ResponseMax, round(respMax, 3))
		allResp = append(allResp, b.resp...)

		// Count timeouts
		for _, c := range b.codes {
			if c == 0 {
				result.TotalTimeout++
			}
		}

		// Workers averages and max
		wSum := 0
		wMax := 0
		for _, v := range b.workers {
			wSum += v
			if v > wMax {
				wMax = v
			}
		}
		wAvg := round(float64(wSum)/float64(len(b.workers)), 1)
		result.WorkersAvg = append(result.WorkersAvg, wAvg)
		result.WorkersMax = append(result.WorkersMax, wMax)
		if wMax > result.PeakWorkers {
			result.PeakWorkers = wMax
		}
	}

	// Calculate overall average response
	if len(allResp) > 0 {
		sum := 0.0
		for _, r := range allResp {
			sum += r
		}
		result.AvgResponse = round(sum/float64(len(allResp)), 3)
	}

	// Detect max workers from last sample (consistent across all samples)
	if len(samples) > 0 {
		result.MaxWorkers = samples[len(samples)-1].MaxWorkers
	}

	result.PeakLoad = round(result.PeakLoad, 2)

	return result
}

// trimMonitorData re-parses the log keeping only the last maxSamples entries.
func trimMonitorData(logContent string, maxSamples int) monitorData {
	lines := strings.Split(logContent, "\n")

	// Collect all valid data lines
	var dataLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if monitorLineRegex.MatchString(line) {
			dataLines = append(dataLines, line)
		}
	}

	// Keep only the last maxSamples lines
	if len(dataLines) > maxSamples {
		dataLines = dataLines[len(dataLines)-maxSamples:]
	}

	return parseMonitorLog(strings.Join(dataLines, "\n"))
}

// rawSample is a single data point with an ISO timestamp for Chart.js time axis.
type rawSample struct {
	Time       string  `json:"time"`
	DB         int     `json:"db"`
	Load       float64 `json:"load"`
	Code       int     `json:"code"`
	Resp       float64 `json:"resp"`
	Workers    int     `json:"workers"`
	MaxWorkers int     `json:"max_workers"`
}

type rawMonitorData struct {
	Samples    []rawSample `json:"samples"`
	MaxWorkers int         `json:"max_workers"`
}

// parseMonitorLogRaw parses the log and returns individual samples with reconstructed ISO timestamps.
func parseMonitorLogRaw(logContent string, hours int) rawMonitorData {
	lines := strings.Split(logContent, "\n")

	var samples []monitorSample
	for _, line := range lines {
		line = strings.TrimSpace(line)
		m := monitorLineRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		hh, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		ss, _ := strconv.Atoi(m[3])
		db, _ := strconv.Atoi(m[4])
		load, _ := strconv.ParseFloat(m[5], 64)
		code, _ := strconv.Atoi(m[6])
		resp, _ := strconv.ParseFloat(m[7], 64)

		activeWorkers := 0
		maxWorkers := 0
		if m[8] != "" {
			activeWorkers, _ = strconv.Atoi(m[8])
			maxWorkers, _ = strconv.Atoi(m[9])
		}

		samples = append(samples, monitorSample{
			Hour: hh, Minute: mm, Second: ss,
			DBConns: db, Load: load,
			HTTPCode: code, ResponseTime: resp,
			ActiveWorkers: activeWorkers, MaxWorkers: maxWorkers,
		})
	}

	if len(samples) == 0 {
		return rawMonitorData{}
	}

	// Count midnight rollovers to determine start date
	crossings := 0
	prevHH := samples[len(samples)-1].Hour
	for i := len(samples) - 2; i >= 0; i-- {
		hh := samples[i].Hour
		if hh > prevHH && prevHH <= 1 {
			crossings++
		}
		prevHH = hh
	}

	// Anchor to current UTC date and walk backward
	utcNow := time.Now().UTC()
	startDate := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, time.UTC)
	startDate = startDate.AddDate(0, 0, -crossings)

	// Walk forward, assigning full timestamps
	curDate := startDate
	prevHH = samples[0].Hour
	var result []rawSample
	globalMaxWorkers := 0

	for _, s := range samples {
		if s.Hour < prevHH && prevHH >= 22 {
			curDate = curDate.AddDate(0, 0, 1)
		}
		prevHH = s.Hour

		ts := time.Date(curDate.Year(), curDate.Month(), curDate.Day(), s.Hour, s.Minute, s.Second, 0, time.UTC)

		result = append(result, rawSample{
			Time:       ts.Format(time.RFC3339),
			DB:         s.DBConns,
			Load:       s.Load,
			Code:       s.HTTPCode,
			Resp:       s.ResponseTime,
			Workers:    s.ActiveWorkers,
			MaxWorkers: s.MaxWorkers,
		})

		if s.MaxWorkers > globalMaxWorkers {
			globalMaxWorkers = s.MaxWorkers
		}
	}

	// Trim to requested hours
	if hours > 0 {
		maxSamples := hours * 120
		if len(result) > maxSamples {
			result = result[len(result)-maxSamples:]
		}
	}

	// Downsample to ~1000 points using min/max pairs to preserve peaks.
	// For every N samples, emit two points: the min and max, keeping visual fidelity.
	const targetPoints = 500
	if len(result) > targetPoints {
		groupSize := len(result) / (targetPoints / 2)
		if groupSize < 2 {
			groupSize = 2
		}
		var downsampled []rawSample
		for i := 0; i < len(result); i += groupSize {
			end := i + groupSize
			if end > len(result) {
				end = len(result)
			}
			group := result[i:end]
			// Find the sample with the lowest load and the sample with the highest load
			minIdx, maxIdx := 0, 0
			for j := range group {
				if group[j].Load < group[minIdx].Load {
					minIdx = j
				}
				if group[j].Load > group[maxIdx].Load {
					maxIdx = j
				}
			}
			// Emit in chronological order
			if minIdx <= maxIdx {
				downsampled = append(downsampled, group[minIdx])
				if minIdx != maxIdx {
					downsampled = append(downsampled, group[maxIdx])
				}
			} else {
				downsampled = append(downsampled, group[maxIdx])
				if minIdx != maxIdx {
					downsampled = append(downsampled, group[minIdx])
				}
			}
		}
		result = downsampled
	}

	return rawMonitorData{
		Samples:    result,
		MaxWorkers: globalMaxWorkers,
	}
}

func round(val float64, precision int) float64 {
	p := math.Pow(10, float64(precision))
	return math.Round(val*p) / p
}

func init() {
	performanceMonitorFetchCmd.Flags().Int("hours", 0, "Number of hours of data to fetch (0 = all available)")
	performanceMonitorFetchCmd.Flags().String("format", "", "Output format: 'raw' for individual timestamped samples")
	performanceMonitorCmd.AddCommand(performanceMonitorActivateCmd)
	performanceMonitorCmd.AddCommand(performanceMonitorDeactivateCmd)
	performanceMonitorCmd.AddCommand(performanceMonitorFetchCmd)
	rootCmd.AddCommand(performanceMonitorCmd)
}
