package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CaptainCore/captaincore/models"
)

// BulkConfig holds the configuration for a bulk execution run.
type BulkConfig struct {
	Command   string   // e.g., "ssh", "backup/generate"
	Targets   []string // site names or @target groups
	Flags     []string // pass-through flags as discrete strings
	CaptainID string
	Parallel  int
	Label     bool
	Debug     bool
}

// bulkRunning is an in-process guard to prevent re-entry.
var bulkRunning int32

// runBulk executes a CaptainCore command across multiple sites in parallel.
func runBulk(cfg BulkConfig) error {
	// Cross-process recursion guard
	if os.Getenv("CC_BULK_RUNNING") == "true" {
		fmt.Fprintf(os.Stderr, "\033[31mError:\033[39m Recursive bulk execution detected. Aborting.\n")
		return fmt.Errorf("recursive bulk execution detected")
	}

	// In-process recursion guard
	if !atomic.CompareAndSwapInt32(&bulkRunning, 0, 1) {
		fmt.Fprintf(os.Stderr, "\033[31mError:\033[39m Recursive bulk execution detected. Aborting.\n")
		return fmt.Errorf("recursive bulk execution detected")
	}
	defer atomic.StoreInt32(&bulkRunning, 0)

	// Resolve @targets to site lists
	sites, err := resolveTargets(cfg.Targets, cfg.CaptainID)
	if err != nil {
		return fmt.Errorf("resolving targets: %w", err)
	}
	if len(sites) == 0 {
		fmt.Fprintf(os.Stderr, "\033[31mError:\033[39m No sites matched the target.\n")
		return fmt.Errorf("no sites matched")
	}

	parallel := cfg.Parallel
	if parallel <= 0 {
		parallel = 10
	}

	fmt.Printf("Running '%s' on %d sites (parallel: %d)...\n", cfg.Command, len(sites), parallel)

	// Set up progress tracking
	home, _ := os.UserHomeDir()
	progressDir := filepath.Join(home, ".captaincore", "data", "progress")
	os.MkdirAll(progressDir, 0755)

	pid := os.Getpid()
	metaPath := filepath.Join(progressDir, fmt.Sprintf("%d.json", pid))
	logPath := filepath.Join(progressDir, fmt.Sprintf("%d.log", pid))

	meta := progressMeta{
		Command:   cfg.Command,
		Total:     len(sites),
		PID:       pid,
		StartedAt: time.Now().Unix(),
		CaptainID: cfg.CaptainID,
		Parallel:  parallel,
		Target:    strings.Join(sites, " "),
		Args:      strings.Join(cfg.Flags, " "),
	}
	metaJSON, _ := json.Marshal(meta)
	os.WriteFile(metaPath, metaJSON, 0644)
	os.WriteFile(logPath, nil, 0644)

	defer func() {
		os.Remove(metaPath)
		os.Remove(logPath)
	}()

	// Build the base command parts
	// e.g., "backup/generate" → ["backup", "generate"]
	cmdParts := strings.Split(cfg.Command, "/")

	// Find the captaincore binary
	binPath, err := os.Executable()
	if err != nil {
		binPath = "captaincore"
	}

	// Run sites in parallel
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var outputMu sync.Mutex
	var logMu sync.Mutex

	for _, site := range sites {
		sem <- struct{}{}
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Build args: <command parts> <flags> --captain-id=<id> <site>
			args := make([]string, 0, len(cmdParts)+len(cfg.Flags)+2)
			args = append(args, cmdParts...)
			args = append(args, cfg.Flags...)
			args = append(args, "--captain-id="+cfg.CaptainID)
			args = append(args, s)

			cmd := exec.Command(binPath, args...)
			cmd.Env = append(os.Environ(), "CC_BULK_RUNNING=true")

			if cfg.Label {
				runLabeledSite(cmd, s, &outputMu)
			} else {
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Run()
			}

			exitCode := 0
			if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
				exitCode = cmd.ProcessState.ExitCode()
			}

			logMu.Lock()
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				fmt.Fprintf(f, "%s %d %d\n", s, exitCode, time.Now().Unix())
				f.Close()
			}
			logMu.Unlock()
		}(site)
	}

	wg.Wait()
	return nil
}

// runLabeledSite captures output from a site command and prints it with a
// colored site header. Strips SSH MOTD/banners by extracting content between
// output markers.
func runLabeledSite(cmd *exec.Cmd, site string, mu *sync.Mutex) {
	raw, _ := cmd.CombinedOutput()
	output := string(raw)

	// Extract content between markers (strips SSH MOTD/banner)
	const markerStart = "____CC_OUTPUT_START____"
	const markerEnd = "____CC_OUTPUT_END____"

	startIdx := strings.Index(output, markerStart)
	endIdx := strings.Index(output, markerEnd)
	if startIdx >= 0 && endIdx > startIdx {
		output = output[startIdx+len(markerStart) : endIdx]
	}

	// Strip empty lines
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	output = strings.Join(lines, "\n")

	if output == "" {
		return
	}

	mu.Lock()
	fmt.Printf("\033[32;1m== %s ==\033[0m\n%s\n\n", site, output)
	mu.Unlock()
}

// resolveTargets converts target arguments into a concrete list of site names.
// Handles @all, @production, @staging via database lookup, or returns site
// names as-is for explicit site lists.
func resolveTargets(targets []string, captainID string) ([]string, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets specified")
	}

	// Check if the first target is a group target
	first := targets[0]
	if strings.HasPrefix(first, "@production") || strings.HasPrefix(first, "@staging") || strings.HasPrefix(first, "@all") {
		if !ensureDB() {
			return nil, fmt.Errorf("database not available")
		}

		// Set captain ID for DB queries
		os.Setenv("CAPTAIN_ID", captainID)

		environment, minorTargets := models.ParseTargetString(first)
		queryArgs := models.FetchSiteMatchingArgs{
			Environment: environment,
			Targets:     minorTargets,
		}

		results, err := models.FetchSitesMatching(queryArgs)
		if err != nil {
			return nil, fmt.Errorf("fetching sites: %w", err)
		}

		var sites []string
		for _, r := range results {
			envLower := strings.ToLower(r.Environment)
			sites = append(sites, fmt.Sprintf("%s-%s", r.Site, envLower))
		}

		sites = uniqueStrings(sites)
		sort.Strings(sites)
		return sites, nil
	}

	// Explicit site list — return as-is
	return targets, nil
}

// collectBulkFlags gathers the current global flag variables into a slice of
// CLI flag strings suitable for passing to child processes.
func collectBulkFlags() []string {
	var flags []string
	if flagCommand != "" {
		flags = append(flags, "--command="+flagCommand)
	}
	if flagRecipe != "" {
		flags = append(flags, "--recipe="+flagRecipe)
	}
	if flagScript != "" {
		flags = append(flags, "--script="+flagScript)
	}
	for _, passArg := range flagScriptPassthrough {
		flags = append(flags, passArg)
	}
	if flagSkipIfRecent != "" {
		flags = append(flags, "--skip-if-recent="+flagSkipIfRecent)
	}
	if flagSkipDB {
		flags = append(flags, "--skip-db")
	}
	if flagInit {
		flags = append(flags, "--init")
	}
	if flagField != "" {
		flags = append(flags, "--field="+flagField)
	}
	if flagSkipRemote {
		flags = append(flags, "--skip-remote")
	}
	if flagUpdateExtras {
		flags = append(flags, "--update-extras")
	}
	if flagDeleteAfterSnapshot {
		flags = append(flags, "--delete-after-snapshot")
	}
	if flagNotes != "" {
		flags = append(flags, "--notes="+flagNotes)
	}
	if flagVersion != "" {
		flags = append(flags, "--version="+flagVersion)
	}
	if flagAll {
		flags = append(flags, "--all")
	}
	if flagForce {
		flags = append(flags, "--force")
	}
	if flagHtml {
		flags = append(flags, "--html")
	}
	if flagTheme != "" {
		flags = append(flags, "--theme="+flagTheme)
	}
	if flagPlugin != "" {
		flags = append(flags, "--plugin="+flagPlugin)
	}
	if flagFile != "" {
		flags = append(flags, "--file="+flagFile)
	}
	if flagLimit != "" {
		flags = append(flags, "--limit="+flagLimit)
	}
	if flagName != "" {
		flags = append(flags, "--name="+flagName)
	}
	if flagLink != "" {
		flags = append(flags, "--link="+flagLink)
	}
	if flagSubject != "" {
		flags = append(flags, "--subject="+flagSubject)
	}
	if flagStatus != "" {
		flags = append(flags, "--status="+flagStatus)
	}
	if flagAction != "" {
		flags = append(flags, "--action="+flagAction)
	}
	if flagEmail != "" {
		flags = append(flags, "--email="+flagEmail)
	}
	if flagUserId != "" {
		flags = append(flags, "--user-id="+flagUserId)
	}
	if flagFilter != "" {
		flags = append(flags, "--filter="+flagFilter)
	}
	if flagRetry != 0 {
		flags = append(flags, fmt.Sprintf("--retry=%d", flagRetry))
	}
	if flagPublic {
		flags = append(flags, "--public")
	}
	if flagCode != "" {
		flags = append(flags, "--code="+flagCode)
	}
	if flagDebug {
		flags = append(flags, "--debug")
	}
	if flagLabel {
		flags = append(flags, "--label")
	}
	if flagSkipAlreadyGenerated {
		flags = append(flags, "--skip-already-generated")
	}
	if flagSkipScreenshot {
		flags = append(flags, "--skip-screenshot")
	}
	if flagDryRun {
		flags = append(flags, "--dry-run")
	}
	if flagCached {
		flags = append(flags, "--cached")
	}
	if flagGlobalOnly {
		flags = append(flags, "--global-only")
	}
	if flagRepackUncompressed {
		flags = append(flags, "--repack-uncompressed")
	}
	if flagBash {
		flags = append(flags, "--bash")
	}
	if flagSearchField != "" {
		flags = append(flags, "--search-field="+flagSearchField)
	}
	if flagFormat != "" {
		flags = append(flags, "--format="+flagFormat)
	}
	if flagPage != "" {
		flags = append(flags, "--page="+flagPage)
	}
	if flagFilterName != "" {
		flags = append(flags, "--filter-name="+flagFilterName)
	}
	if flagFilterVersion != "" {
		flags = append(flags, "--filter-version="+flagFilterVersion)
	}
	if flagFilterStatus != "" {
		flags = append(flags, "--filter-status="+flagFilterStatus)
	}
	return flags
}
