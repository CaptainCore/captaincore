package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

// captaincore db-scan: run lib/remote-scripts/db-scan on one site or a
// target and judge the export here, exactly as the nightly sync does for its
// incremental copy. --full reads every post and comment (the weekly sweep);
// --spread keeps only the sites whose id falls on today's weekday, so a daily
// cron covers the fleet once a week the way the full-tree scans do.

var (
	dbScanFull     bool
	dbScanSince    int
	dbScanSpread   bool
	dbScanParallel int
	dbScanQuiet    bool
)

var dbScanCmd = &cobra.Command{
	Use:   "db-scan <site|@target>",
	Short: "Scan a site's database for injections with the malware rules",
	Long: `Runs lib/remote-scripts/db-scan on the site (options, cron, recent posts and
comments, administrators, active plugins, triggers, events, routines, tables)
and judges the export with the malware rules and the fixed database checks.
Findings at the alert floor post a malware alert with source "db-scan-full"
(--full) or "db-scan".

  captaincore db-scan mysite-production
  captaincore db-scan mysite-production --full
  captaincore db-scan @production --full --spread --parallel=3   # the weekly sweep, from cron

--spread selects the sites whose id modulo 7 equals today's weekday, so a
daily cron line sweeps every site once a week.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, dbScanNative)
	},
}

type dbScanTarget struct {
	site *models.Site
	env  *models.Environment
}

func dbScanNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}
	var targets []dbScanTarget
	if strings.HasPrefix(args[0], "@") {
		sites, err := models.GetAllActiveSites()
		if err != nil {
			fmt.Printf("Error fetching sites: %v\n", err)
			return
		}
		environment, _ := models.ParseTargetString(args[0])
		today := int(time.Now().UTC().Weekday())
		for i := range sites {
			if dbScanSpread && int(sites[i].SiteID%7) != today {
				continue
			}
			envs, err := models.FindEnvironmentsBySiteID(sites[i].SiteID)
			if err != nil {
				continue
			}
			for j := range envs {
				if environment != "" && environment != "all" && !strings.EqualFold(envs[j].Environment, environment) {
					continue
				}
				if envs[j].Protocol != "sftp" || envs[j].Address == "" {
					continue
				}
				targets = append(targets, dbScanTarget{&sites[i], &envs[j]})
			}
		}
	} else {
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
		targets = append(targets, dbScanTarget{site, env})
	}
	if len(targets) == 0 {
		fmt.Println("No sites selected.")
		return
	}
	mode := "db-scan"
	if dbScanFull {
		mode = "db-scan-full"
	}
	if !dbScanQuiet {
		fmt.Printf("Database scan (%s) on %d environment(s), %d at a time\n", mode, len(targets), dbScanParallel)
	}
	jobs := make(chan dbScanTarget)
	var wg sync.WaitGroup
	var mu sync.Mutex
	scanned, withFindings, failed := 0, 0, 0
	for i := 0; i < dbScanParallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				line, findings, err := dbScanOne(t, system, captain, mode)
				mu.Lock()
				if err != nil {
					failed++
				} else {
					scanned++
					if findings > 0 {
						withFindings++
					}
				}
				if !dbScanQuiet || err != nil || findings > 0 {
					fmt.Println(line)
				}
				mu.Unlock()
			}
		}()
	}
	for _, t := range targets {
		jobs <- t
	}
	close(jobs)
	wg.Wait()
	if !dbScanQuiet || withFindings > 0 || failed > 0 {
		fmt.Printf("Done: %d scanned, %d with findings, %d failed\n", scanned, withFindings, failed)
	}
}

// dbScanOne runs the remote script for one environment and judges the export.
func dbScanOne(t dbScanTarget, system *config.SystemConfig, captain *config.CaptainConfig, mode string) (string, int, error) {
	label := fmt.Sprintf("%s-%s", t.site.Site, strings.ToLower(t.env.Environment))
	args := []string{"ssh", label, "--script=db-scan", "--captain-id=" + captainID}
	if dbScanFull {
		args = append(args, "--full")
	} else {
		args = append(args, fmt.Sprintf("--since=%d", dbScanSince))
	}
	out, err := exec.Command("captaincore", args...).Output()
	if err != nil {
		return fmt.Sprintf("\033[31m✗\033[0m %-34s ssh failed: %v", label, err), 0, err
	}
	// The script prints one JSON object; anything before it is ssh noise.
	raw := strings.TrimSpace(string(out))
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	if raw == "" || !json.Valid([]byte(raw)) {
		e := fmt.Errorf("no JSON from db-scan")
		return fmt.Sprintf("\033[31m✗\033[0m %-34s %v", label, e), 0, e
	}
	findings, summary, err := dbScanFindings(raw, knownUserLogins(t.env.Users), nativeAlertSeverity)
	if err != nil {
		return fmt.Sprintf("\033[31m✗\033[0m %-34s %v", label, err), 0, err
	}
	if len(findings) > 0 {
		postMalwareAlert(t.site, t.env, system, captain, findings, mode)
	}
	mark := "\033[32m✓\033[0m"
	if len(findings) > 0 {
		mark = "\033[31m✗\033[0m"
	}
	note := ""
	if summary.Truncated {
		note = " (export truncated at the size cap)"
	}
	names := []string{}
	for _, f := range findings {
		names = append(names, f.Filename+" "+f.SignatureID)
		if len(names) == 4 {
			names = append(names, "…")
			break
		}
	}
	line := fmt.Sprintf("%s %-34s %d options, %d/%d posts, %d comments exported; %d finding(s)%s", mark, label,
		summary.Stats["options_exported"], summary.Stats["posts_exported"], summary.Stats["posts_checked"], summary.Stats["comments_exported"], len(findings), note)
	if len(names) > 0 {
		line += "\n    " + strings.Join(names, "\n    ")
	}
	return line, len(findings), nil
}

func init() {
	dbScanCmd.Flags().BoolVar(&dbScanFull, "full", false, "Read every post and comment (the weekly sweep) instead of the last --since days")
	dbScanCmd.Flags().IntVar(&dbScanSince, "since", 2, "Days of posts and comments to read when not --full")
	dbScanCmd.Flags().BoolVar(&dbScanSpread, "spread", false, "With an @target: only sites whose id modulo 7 is today's weekday")
	dbScanCmd.Flags().IntVar(&dbScanParallel, "parallel", 3, "Environments scanned at once")
	dbScanCmd.Flags().BoolVar(&dbScanQuiet, "quiet", false, "Only print environments with findings or errors")
	rootCmd.AddCommand(dbScanCmd)
}
