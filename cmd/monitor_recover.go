package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// Auto-recovery for sites that keep failing the uptime monitor.
//
// `monitor run` queues a site once it has failed two runs in a row, and again
// each hour while it stays down, up to three attempts per outage (the counter
// resets when the site recovers, because its monitor.json record is dropped).
// Each attempt runs detached as `monitor recover <site>`, so a slow recovery
// never holds the 5-minute monitor lock. The recover command probes the
// PHP-FPM pool over SSH (lib/remote-scripts/php-worker-check) and posts the
// result to the Manager, which decides whether to restart PHP through the
// provider. After a restart it re-checks the URL and reports the outcome; the
// Manager records every attempt and emails it.
const (
	recoveryMinChecks      = 2
	recoveryMaxAttempts    = 3
	recoveryInterval       = time.Hour
	recoveryMaxOutageAge   = 12 * time.Hour // older outages are not PHP lockups worth chasing
	recoveryMaxSitesPerRun = 5
	// This many outages starting within recoveryMassWindow points at the
	// monitor or the network, not the sites. Counted from new outages only:
	// the fleet always carries some long-standing failures (parked domains,
	// 403s), and those must not hold recovery back for everyone else.
	recoveryMassOutage = 25
	recoveryMassWindow = 30 * time.Minute
)

var monitorNoRecovery bool

// launchRecover starts one detached recovery; tests swap it out.
var launchRecover = launchMonitorRecover

var monitorRecoverAttempt, monitorRecoverFailedChecks, monitorRecoverWait int
var monitorRecoverURL, monitorRecoverHTTPCode, monitorRecoverError string
var monitorRecoverDryRun bool

// monitorQueueRecoveries picks failing sites that are due an attempt, records
// the attempt in monitor.json and launches `monitor recover` for each in the
// background.
func monitorQueueRecoveries(monitorFile string, logsPath string, autoRecovery string) {
	if monitorNoRecovery || strings.EqualFold(strings.TrimSpace(autoRecovery), "off") {
		return
	}

	var records []MonitorRecord
	data, err := os.ReadFile(monitorFile)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return
	}

	now := time.Now().Unix()
	fresh := 0
	for _, r := range records {
		if now-r.CreatedAt <= int64(recoveryMassWindow.Seconds()) {
			fresh++
		}
	}
	if fresh > recoveryMassOutage {
		fmt.Printf("Auto-recovery skipped: %d sites went down in the last %s, which points at the monitor or the network, not the sites.\n", fresh, recoveryMassWindow)
		return
	}

	launched := 0
	for i := range records {
		r := &records[i]
		if r.CheckCount < recoveryMinChecks || r.HTTPCode == "301" {
			continue
		}
		if r.RecoveryAttempts >= recoveryMaxAttempts {
			continue
		}
		if r.RecoveryAttempts == 0 && now-r.CreatedAt > int64(recoveryMaxOutageAge.Seconds()) {
			continue
		}
		if r.RecoveryAttempts > 0 && now-r.LastRecoveryAt < int64(recoveryInterval.Seconds()) {
			continue
		}
		if launched >= recoveryMaxSitesPerRun {
			break
		}
		r.RecoveryAttempts++
		r.LastRecoveryAt = now
		if err := launchRecover(*r, logsPath); err != nil {
			fmt.Printf("Auto-recovery for %s could not start: %v\n", r.Name, err)
			continue
		}
		fmt.Printf("Auto-recovery attempt %d of %d started for %s\n", r.RecoveryAttempts, recoveryMaxAttempts, r.Name)
		launched++
	}

	if launched == 0 {
		return
	}
	out, _ := json.MarshalIndent(records, "", "    ")
	tmp := monitorFile + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err == nil {
		os.Rename(tmp, monitorFile)
	}
}

// launchMonitorRecover starts `captaincore monitor recover` in its own session
// so it outlives this monitor run.
func launchMonitorRecover(r MonitorRecord, logsPath string) error {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "captaincore"
	}
	args := []string{
		"monitor", "recover", r.Name,
		"--attempt=" + strconv.Itoa(r.RecoveryAttempts),
		"--failed-checks=" + strconv.Itoa(r.CheckCount),
		"--url=" + r.URL,
		"--http-code=" + r.HTTPCode,
		"--error=" + r.Error,
		"--captain-id=" + captainID,
	}
	c := exec.Command(exe, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if logsPath != "" {
		name := fmt.Sprintf("monitor-recover_%s_%s.txt", time.Now().Format("2006-01-02_15-04"), safeFileName(r.Name))
		if f, err := os.OpenFile(filepath.Join(logsPath, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			c.Stdout = f
			c.Stderr = f
			defer f.Close()
		}
	}
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func safeFileName(s string) string {
	return unsafeFileChars.ReplaceAllString(s, "_")
}

func monitorRecoverNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}
	name := args[0]

	// One recovery per site at a time.
	pathTmp := "/tmp"
	if system != nil && system.PathTmp != "" {
		pathTmp = system.PathTmp
	}
	lockFile := filepath.Join(pathTmp, "captaincore-recover-"+safeFileName(name)+".lock")
	if raw, err := os.ReadFile(lockFile); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && syscall.Kill(pid, 0) == nil {
			fmt.Printf("Recovery for %s already running (PID %d).\n", name, pid)
			return
		}
	}
	os.WriteFile(lockFile, []byte(strconv.Itoa(os.Getpid())), 0644)
	defer os.Remove(lockFile)

	sa := parseSiteArgument(name)
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: site %s not found.\n", name)
		os.Exit(1)
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Printf("Error: environment for %s not found.\n", name)
		os.Exit(1)
	}
	url := monitorRecoverURL
	if url == "" {
		url = env.HomeURL
	}
	attempt := monitorRecoverAttempt
	if attempt < 1 {
		attempt = 1
	}

	// 1. Probe the PHP-FPM pool. Pure shell on the far side; give it a hard
	// deadline because an overloaded container can stall SSH too.
	fmt.Printf("[%s] Probing PHP on %s (attempt %d)\n", time.Now().Format(time.DateTime), name, attempt)
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "captaincore"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	output, probeErr := exec.CommandContext(ctx, exe, "ssh", name, "--script=php-worker-check", "--captain-id="+captainID).CombinedOutput()
	cancel()
	probe := parseSiteData(string(output))
	sshOK := probeErr == nil && probe["probe"] == "ok"
	if !sshOK {
		detail := strings.TrimSpace(string(output))
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			detail = "SSH probe timed out after 75s"
		} else if detail == "" && probeErr != nil {
			detail = probeErr.Error()
		}
		if len(detail) > 300 {
			detail = detail[len(detail)-300:]
		}
		probe["ssh_error"] = detail
	}
	fmt.Printf("Probe: ssh_ok=%v busy_workers=%s max_workers=%s load_1=%s\n", sshOK, probe["busy_workers"], probe["max_workers"], probe["load_1"])

	// 2. Report it. The Manager decides whether a restart is justified.
	client := newAPIClient(system, captain)
	adminEmail := getVarString(captain, "captaincore_admin_email")
	resp, err := client.Post("monitor-recovery-start", map[string]interface{}{
		"site_id":     site.SiteID,
		"environment": env.Environment,
		"data": map[string]interface{}{
			"url":           url,
			"http_code":     monitorRecoverHTTPCode,
			"error":         monitorRecoverError,
			"failed_checks": monitorRecoverFailedChecks,
			"attempt":       attempt,
			"ssh_ok":        sshOK,
			"probe":         probe,
			"dry_run":       monitorRecoverDryRun,
			"email":         adminEmail,
		},
	})
	if err != nil {
		fmt.Println("Error posting monitor-recovery-start:", err)
		os.Exit(1)
	}
	var start struct {
		MonitorRecoveryID json.Number `json:"monitor_recovery_id"`
		Action            string      `json:"action"`
		Reason            string      `json:"reason"`
	}
	if err := json.Unmarshal(resp, &start); err != nil {
		fmt.Println("Unexpected monitor-recovery-start response:", string(resp))
		os.Exit(1)
	}
	fmt.Printf("Manager: %s. %s\n", start.Action, start.Reason)
	if start.Action != "restarted" {
		return
	}

	// 3. Give PHP a moment to come back, then check the site the same way the
	// monitor does, a few times before calling it still down.
	wait := time.Duration(monitorRecoverWait) * time.Second
	fmt.Printf("Waiting %s before re-checking %s\n", wait, url)
	time.Sleep(wait)
	var after MonitorCheckResult
	restored := false
	for i := 1; i <= 3; i++ {
		after = monitorCheckSingle(url, name, 60*time.Second, sharedTransport)
		if after.HTMLValid != "false" && (after.HTTPCode == "200" || after.HTTPCode == "301") {
			restored = true
			break
		}
		fmt.Printf("Re-check %d: %s %s\n", i, after.HTTPCode, after.Error)
		if i < 3 {
			time.Sleep(20 * time.Second)
		}
	}
	fmt.Printf("Restored: %v (%s)\n", restored, after.HTTPCode)

	resp, err = client.Post("monitor-recovery-finish", map[string]interface{}{
		"site_id":     site.SiteID,
		"environment": env.Environment,
		"data": map[string]interface{}{
			"monitor_recovery_id": start.MonitorRecoveryID,
			"restored":            restored,
			"after_http_code":     after.HTTPCode,
			"after_error":         after.Error,
			"email":               adminEmail,
		},
	})
	if err != nil {
		fmt.Println("Error posting monitor-recovery-finish:", err)
		os.Exit(1)
	}
	fmt.Println("Manager:", strings.TrimSpace(string(resp)))
}

var monitorRecoverCmd = &cobra.Command{
	Use:   "recover <site>",
	Short: "Probes a failing site's PHP pool and asks the Manager to restart PHP when it is saturated",
	Long: `Probes a failing site's PHP-FPM pool over SSH and reports it to the Manager,
which restarts PHP through the provider (Kinsta) when the failure is a timeout
or gateway error and the pool is saturated or SSH cannot get in. After a restart
it re-checks the site and reports whether it came back; the Manager records and
emails every attempt. 'monitor run' launches this on its own for sites that fail
two runs in a row; run it by hand with --dry-run to see the probe and decision
without restarting anything.`,
	Example: "captaincore monitor recover mysite --dry-run",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, monitorRecoverNative)
	},
}

func init() {
	monitorCmd.AddCommand(monitorRecoverCmd)
	monitorRecoverCmd.Flags().IntVar(&monitorRecoverAttempt, "attempt", 1, "Attempt number for this outage (1-3)")
	monitorRecoverCmd.Flags().IntVar(&monitorRecoverFailedChecks, "failed-checks", 0, "Monitor runs failed so far")
	monitorRecoverCmd.Flags().StringVar(&monitorRecoverURL, "url", "", "URL to re-check (defaults to the environment's home URL)")
	monitorRecoverCmd.Flags().StringVar(&monitorRecoverHTTPCode, "http-code", "000", "HTTP code the monitor saw")
	monitorRecoverCmd.Flags().StringVar(&monitorRecoverError, "error", "", "Error the monitor saw")
	monitorRecoverCmd.Flags().IntVar(&monitorRecoverWait, "wait", 45, "Seconds to wait after a restart before re-checking")
	monitorRecoverCmd.Flags().BoolVar(&monitorRecoverDryRun, "dry-run", false, "Probe and report, but never restart")
}
