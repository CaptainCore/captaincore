package cmd

import (
	"encoding/json"
	"fmt"
	"html"
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

	summarizeCore := cfg.Command == "ssh" && bulkScriptName(cfg.Flags) == "update-core"
	started := time.Now()
	fmt.Printf("Running '%s' on %d sites (parallel: %d)...\n", cfg.Command, len(sites), parallel)
	if summarizeCore {
		fmt.Println("Core update mode: one line per site, email summary of failures at the end.")
	}

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
	var resultsMu sync.Mutex
	results := make([]bulkSiteResult, 0, len(sites))

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

			var output string
			if summarizeCore {
				raw, _ := cmd.CombinedOutput()
				output = string(raw)
			} else if cfg.Label {
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

			if summarizeCore {
				res := parseUpdateCoreOutput(s, exitCode, output)
				resultsMu.Lock()
				results = append(results, res)
				resultsMu.Unlock()
				outputMu.Lock()
				fmt.Print(formatCoreUpdateLine(res))
				if res.Result == "fail" && strings.TrimSpace(output) != "" {
					fmt.Print(output)
					if !strings.HasSuffix(output, "\n") {
						fmt.Print("\n")
					}
				}
				outputMu.Unlock()
			}
		}(site)
	}

	wg.Wait()

	if summarizeCore {
		sort.Slice(results, func(i, j int) bool { return results[i].Site < results[j].Site })
		printCoreUpdateSummary(results, time.Since(started), len(sites), parallel)
		emailCoreUpdateSummary(cfg, results, time.Since(started), parallel)
		for _, res := range results {
			if res.Result == "fail" {
				return fmt.Errorf("core update finished with failures")
			}
		}
	}
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

type bulkSiteResult struct {
	Site     string
	URL      string
	Result   string
	Action   string
	Stage    string
	From     string
	To       string
	Reason   string
	ExitCode int
	Excerpt  string
}

func bulkScriptName(flags []string) string {
	for _, f := range flags {
		if strings.HasPrefix(f, "--script=") {
			return filepath.Base(strings.TrimPrefix(f, "--script="))
		}
	}
	return ""
}

func parseResultFields(line string) map[string]string {
	out := map[string]string{}
	rest := strings.TrimSpace(line)
	for rest != "" {
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			break
		}
		key := rest[:eq]
		rest = rest[eq+1:]
		if key == "reason" {
			out[key] = rest
			break
		}
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			out[key] = rest
			break
		}
		out[key] = rest[:sp]
		rest = strings.TrimSpace(rest[sp+1:])
	}
	return out
}

func failureExcerpt(output string) string {
	var hits []string
	for _, line := range strings.Split(output, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.Contains(trim, "Fatal error") || strings.Contains(trim, "TypeError") || strings.Contains(trim, "Parse error") || strings.HasPrefix(trim, "Error:") {
			hits = append(hits, trim)
		}
	}
	if len(hits) > 6 {
		hits = hits[:6]
	}
	if len(hits) > 0 {
		joined := strings.Join(hits, " | ")
		if len(joined) > 500 {
			return joined[:500] + "…"
		}
		return joined
	}
	var last []string
	for _, line := range strings.Split(output, "\n") {
		trim := strings.TrimSpace(line)
		if trim != "" {
			last = append(last, trim)
		}
	}
	if len(last) > 4 {
		last = last[len(last)-4:]
	}
	joined := strings.Join(last, " | ")
	if len(joined) > 500 {
		return joined[:500] + "…"
	}
	return joined
}

func parseUpdateCoreOutput(site string, exitCode int, output string) bulkSiteResult {
	res := bulkSiteResult{Site: site, ExitCode: exitCode, Result: "ok"}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "site=") {
			res.URL = strings.TrimPrefix(line, "site=")
		}
		if strings.HasPrefix(line, "result=") {
			fields := parseResultFields(line)
			if v := fields["result"]; v != "" {
				res.Result = v
			}
			res.Action = fields["action"]
			res.Stage = fields["stage"]
			res.From = fields["from"]
			res.To = fields["to"]
			if v := fields["url"]; v != "" {
				res.URL = v
			}
			res.Reason = fields["reason"]
		}
	}
	if exitCode != 0 && res.Result != "fail" {
		res.Result = "fail"
		if res.Stage == "" {
			res.Stage = "ssh"
		}
		if res.Reason == "" {
			res.Reason = fmt.Sprintf("exit %d", exitCode)
		}
	}
	if res.Result == "fail" {
		res.Excerpt = failureExcerpt(output)
		if res.Reason == "" {
			res.Reason = res.Excerpt
		}
	}
	return res
}

func formatCoreUpdateLine(res bulkSiteResult) string {
	tag := "ok"
	if res.Result == "fail" {
		tag = "FAIL"
	} else if res.Action == "skip" {
		tag = "skip"
	}
	detail := res.Action
	if res.From != "" || res.To != "" {
		if res.From != "" && res.To != "" && res.From != res.To {
			detail = strings.TrimSpace(detail + " " + res.From + "→" + res.To)
		} else if res.To != "" {
			detail = strings.TrimSpace(detail + " " + res.To)
		}
	}
	if res.Result == "fail" {
		if res.Stage != "" {
			detail = res.Stage
		}
		if res.Reason != "" {
			detail = strings.TrimSpace(detail + "  " + res.Reason)
		}
	} else if res.Action == "skip" && res.Reason != "" {
		detail = res.Reason
		if res.To != "" {
			detail += " " + res.To
		}
	}
	line := fmt.Sprintf("%-4s %-42s %s\n", tag, res.Site, strings.TrimSpace(detail))
	if res.Result == "fail" {
		return "\033[31m" + line + "\033[0m"
	}
	return line
}

func countCoreUpdateResults(results []bulkSiteResult) (updated, skipped, failed, probed int) {
	for _, res := range results {
		switch {
		case res.Result == "fail":
			failed++
		case res.Action == "apply":
			updated++
		case res.Action == "skip":
			skipped++
			if res.Reason == "probe-only" {
				probed++
			}
		}
	}
	return
}

func printCoreUpdateSummary(results []bulkSiteResult, elapsed time.Duration, total, parallel int) {
	updated, skipped, failed, _ := countCoreUpdateResults(results)
	fmt.Printf("\nCore update finished in %s (%d sites, parallel %d)\n", elapsed.Round(time.Second), total, parallel)
	fmt.Printf("  updated: %d\n", updated)
	fmt.Printf("  skipped: %d\n", skipped)
	fmt.Printf("  failed:  %d\n", failed)
	if failed == 0 {
		return
	}
	fmt.Printf("\nFailures:\n")
	for _, res := range results {
		if res.Result != "fail" {
			continue
		}
		why := res.Reason
		if why == "" {
			why = res.Excerpt
		}
		fmt.Printf("  %-42s %-8s %s %s\n", res.Site, res.Stage, res.URL, why)
	}
}

func emailCoreUpdateSummary(cfg BulkConfig, results []bulkSiteResult, elapsed time.Duration, parallel int) {
	prevCaptain := captainID
	if cfg.CaptainID != "" {
		captainID = cfg.CaptainID
	}
	defer func() { captainID = prevCaptain }()

	_, system, captain, err := loadCaptainConfig()
	if err != nil || captain == nil {
		fmt.Fprintf(os.Stderr, "Core update email skipped: could not load config.\n")
		return
	}
	adminEmail := getVarString(captain, "captaincore_admin_email")
	if adminEmail == "" {
		fmt.Fprintf(os.Stderr, "Core update email skipped: captaincore_admin_email is not set.\n")
		return
	}

	updated, skipped, failed, _ := countCoreUpdateResults(results)
	subject := fmt.Sprintf("Core update: %d updated, %d skipped, %d failed", updated, skipped, failed)
	if failed == 0 {
		subject = fmt.Sprintf("Core update: %d updated, %d skipped, 0 failed", updated, skipped)
	}

	var b strings.Builder
	b.WriteString("<div style=\"text-align:left\">")
	b.WriteString(fmt.Sprintf("<p><strong>WordPress core update finished</strong> in %s.</p>", html.EscapeString(elapsed.Round(time.Second).String())))
	b.WriteString("<ul>")
	b.WriteString(fmt.Sprintf("<li>Sites: %d (parallel %d)</li>", len(results), parallel))
	b.WriteString(fmt.Sprintf("<li>Updated: %d</li>", updated))
	b.WriteString(fmt.Sprintf("<li>Skipped: %d</li>", skipped))
	b.WriteString(fmt.Sprintf("<li>Failed: %d</li>", failed))
	if len(cfg.Flags) > 0 {
		b.WriteString(fmt.Sprintf("<li>Flags: %s</li>", html.EscapeString(strings.Join(cfg.Flags, " "))))
	}
	b.WriteString("</ul>")

	if failed > 0 {
		const cell = "padding:4px 6px;vertical-align:top;font-size:12px;line-height:1.35"
		const head = cell + ";font-size:11px;font-weight:600;white-space:nowrap"
		b.WriteString("<p><strong>Failures</strong></p>")
		b.WriteString("<table cellpadding=\"0\" cellspacing=\"0\" border=\"1\" style=\"border-collapse:collapse;text-align:left;font-size:12px;line-height:1.35;width:100%\">")
		b.WriteString("<tr>")
		b.WriteString("<th style=\"" + head + "\">Site</th>")
		b.WriteString("<th style=\"" + head + "\">URL</th>")
		b.WriteString("<th style=\"" + head + "\">Stage</th>")
		b.WriteString("<th style=\"" + head + "\">Reason</th>")
		b.WriteString("</tr>")
		const maxRows = 150
		n := 0
		for _, res := range results {
			if res.Result != "fail" {
				continue
			}
			n++
			if n > maxRows {
				continue
			}
			why := res.Reason
			if res.Excerpt != "" && res.Excerpt != res.Reason {
				why = res.Reason + " — " + res.Excerpt
			}
			b.WriteString("<tr>")
			b.WriteString("<td style=\"" + cell + "\">" + html.EscapeString(res.Site) + "</td>")
			b.WriteString("<td style=\"" + cell + "\">" + html.EscapeString(res.URL) + "</td>")
			b.WriteString("<td style=\"" + cell + "\">" + html.EscapeString(res.Stage) + "</td>")
			b.WriteString("<td style=\"" + cell + ";word-break:break-word\">" + html.EscapeString(why) + "</td>")
			b.WriteString("</tr>")
		}
		b.WriteString("</table>")
		if failed > maxRows {
			b.WriteString(fmt.Sprintf("<p>Showing the first %d of %d failures.</p>", maxRows, failed))
		}
	}
	b.WriteString("</div>")

	contentJSON, _ := json.Marshal(b.String())
	client := newAPIClient(system, captain)
	if _, err := client.Post("monitor-notify", map[string]interface{}{
		"data": map[string]interface{}{
			"email":   adminEmail,
			"subject": subject,
			"content": json.RawMessage(contentJSON),
		},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Core update email failed: %v\n", err)
		return
	}
	fmt.Printf("Summary emailed to %s\n", adminEmail)
}
