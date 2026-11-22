package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/scan"
	"github.com/spf13/cobra"
)

var flagSecuritySweepJSON bool

var securitySweepCmd = &cobra.Command{
	Use:   "security-sweep <site|@target>",
	Short: "Fast behavioral & persistence security sweep (~2-4s per site)",
	Long: `Runs a lightweight security pulse over one site or a fleet target (@production, @all):
- Session & privilege snapshot (new admins, injected caps, role escalations, new app passwords)
- High-risk captaincore_security_log events (file editor saves, WPCode, app passwords, admin_email)
- PHP eval()'d code errors tailed from server error logs
- Unexpected PHP files at web root / wp-content root + rule scan
- Redacted wp-config.php rule scan`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|@target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		targetCount := 0
		for _, arg := range args {
			if !strings.HasPrefix(arg, "--") {
				targetCount++
			}
		}
		if targetCount > 1 || (len(args) > 0 && (strings.HasPrefix(args[0], "@production") || strings.HasPrefix(args[0], "@staging") || strings.HasPrefix(args[0], "@all"))) {
			if flagParallel <= 0 {
				flagParallel = 20
			}
			resolveCommand(cmd, args)
			return
		}
		resolveNativeOrWP(cmd, args, securitySweepNative)
	},
}

func init() {
	rootCmd.AddCommand(securitySweepCmd)
	securitySweepCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 20, "Number of sites to sweep concurrently")
	securitySweepCmd.Flags().BoolVar(&flagSecuritySweepJSON, "json", false, "Output JSON summary")
}

// wpConfigFindings scans the redacted wp-config.php payload from security-sweep
// with the native malware rule set. The raw file's sha256 rides along so a
// benign verdict sticks until wp-config.php actually changes.
func wpConfigFindings(raw string, minSeverity string) []scan.LegacyFinding {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var doc struct {
		Path   string `json:"path"`
		Sha256 string `json:"sha256"`
		Size   int64  `json:"size"`
		B64    string `json:"b64"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil || doc.B64 == "" {
		return nil
	}
	body, err := base64.StdEncoding.DecodeString(doc.B64)
	if err != nil || len(body) == 0 {
		return nil
	}
	rs, err := scan.LoadDefaultRuleSet()
	if err != nil {
		return nil
	}
	dir, err := os.MkdirTemp("", "cc-wpconfig-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "wp-config.php")
	if os.WriteFile(p, body, 0o600) != nil {
		return nil
	}
	min := scan.SeverityRank(minSeverity)
	seen := map[string]bool{}
	var out []scan.LegacyFinding
	for _, f := range scan.New(rs, scan.Options{Workers: 1}).ScanDir(dir).Findings {
		if scan.SeverityRank(f.Severity) < min {
			continue
		}
		l := f.Legacy()
		l.Filename = "wp-config.php"
		if doc.Sha256 != "" {
			l.ContentHash = doc.Sha256
		}
		l.SignatureDescription = "wp-config.php: " + l.SignatureDescription
		key := l.Filename + "|" + l.SignatureID
		if !seen[key] {
			seen[key] = true
			out = append(out, l)
		}
	}
	return out
}

func securitySweepNative(cmd *cobra.Command, args []string) {
	started := time.Now()
	sa := parseSiteArgument(args[0])

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("security-sweep %s: error (site not found)\n", args[0])
		return
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Printf("security-sweep %s: error (environment not found)\n", args[0])
		return
	}
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Printf("security-sweep %s: error (config not found)\n", args[0])
		return
	}
	client := newAPIClient(system, captain)

	sshCmd := exec.Command("captaincore", "ssh", args[0], "--script=security-sweep", "--captain-id="+captainID)
	sshOutput, _ := sshCmd.Output()
	if len(sshOutput) == 0 {
		fmt.Printf("security-sweep %s: ssh failed (%.1fs)\n", args[0], time.Since(started).Seconds())
		return
	}
	if strings.TrimSpace(string(sshOutput)) == "WordPress not found" {
		fmt.Printf("security-sweep %s: wordpress not found (%.1fs)\n", args[0], time.Since(started).Seconds())
		return
	}

	data := parseSiteData(string(sshOutput))
	counts := map[string]int{}
	totalFindings := 0

	// 1. Session snapshot → SessionAnomalyDetector + AdminWatches
	if signal := strings.TrimSpace(data["session_signal"]); signal != "" && signal != "{}" {
		client.Post("session-snapshot", map[string]interface{}{
			"site_id":     site.SiteID,
			"environment": env.Environment,
			"data":        signal,
		})
	}

	// 2. Unexpected root/content PHP provenance
	if v := strings.TrimSpace(data["unexpected_root_php"]); v != "" && v != "[]" {
		if findings := provenanceFindings(v); len(findings) > 0 {
			postMalwareAlert(site, env, system, captain, findings, "provenance")
			counts["provenance"] = len(findings)
			totalFindings += len(findings)
		}
	}

	// 3. Unexpected root/content PHP rule scan + redacted wp-config.php rule scan
	var provScan []scan.LegacyFinding
	if v := strings.TrimSpace(data["unexpected_root_php_content"]); v != "" && v != "[]" {
		provScan = append(provScan, rootPHPContentFindings(v, nativeAlertSeverity)...)
	}
	if v := strings.TrimSpace(data["wp_config_check"]); v != "" && v != "null" {
		provScan = append(provScan, wpConfigFindings(v, nativeAlertSeverity)...)
	}
	if len(provScan) > 0 {
		postMalwareAlert(site, env, system, captain, provScan, "provenance-scan")
		counts["provenance-scan"] = len(provScan)
		totalFindings += len(provScan)
	}

	// 4. High-risk captaincore_security_log behavioral events
	if v := strings.TrimSpace(data["security_log_events"]); v != "" && v != "[]" {
		if findings := securityLogEventFindings(v); len(findings) > 0 {
			postMalwareAlert(site, env, system, captain, findings, "security-log")
			counts["security-log"] = len(findings)
			totalFindings += len(findings)
		}
	}

	// 5. PHP eval()'d code error-log tell
	if v := strings.TrimSpace(data["runtime_eval_errors"]); v != "" && v != "[]" {
		if findings := runtimeEvalFindings(v); len(findings) > 0 {
			postMalwareAlert(site, env, system, captain, findings, "runtime-log")
			counts["runtime-log"] = len(findings)
			totalFindings += len(findings)
		}
	}

	// 6. Risky role/registration config check (production only, mirroring sync-data)
	if strings.EqualFold(env.Environment, "Production") {
		defRole := strings.TrimSpace(data["default_role"])
		reg := strings.TrimSpace(data["registration"])
		var cfgFindings []scan.LegacyFinding
		if defRole == "administrator" {
			sev := "high"
			if reg == "1" {
				sev = "critical"
			}
			cfgFindings = append(cfgFindings, scan.LegacyFinding{
				Filename:             "db:option/default_role",
				SignatureID:          "config-default-role-administrator",
				SignatureName:        "Default role is Administrator",
				SignatureDescription: "New users are created as administrators.",
				MatchedText:          fmt.Sprintf("default_role=%s, users_can_register=%s", defRole, reg),
				Severity:             sev,
				Family:               "config",
				ContentHash:          subjectHash(defRole, reg),
			})
		} else if reg == "1" && defRole != "" && defRole != "subscriber" {
			cfgFindings = append(cfgFindings, scan.LegacyFinding{
				Filename:             "db:option/users_can_register",
				SignatureID:          "config-open-registration-elevated",
				SignatureName:        "Open registration with an elevated role",
				SignatureDescription: fmt.Sprintf("Anyone can register, and new accounts get the %s role.", defRole),
				MatchedText:          fmt.Sprintf("default_role=%s, users_can_register=%s", defRole, reg),
				Severity:             "high",
				Family:               "config",
				ContentHash:          subjectHash(defRole, reg),
			})
		}
		if len(cfgFindings) > 0 {
			postMalwareAlert(site, env, system, captain, cfgFindings, "config")
			counts["config"] = len(cfgFindings)
			totalFindings += len(cfgFindings)
		}
	}

	elapsed := time.Since(started).Seconds()
	if flagSecuritySweepJSON {
		out, _ := json.Marshal(map[string]any{
			"site":        args[0],
			"environment": env.Environment,
			"elapsed_sec": elapsed,
			"findings":    totalFindings,
			"by_source":   counts,
		})
		fmt.Println(string(out))
		return
	}
	if totalFindings == 0 {
		fmt.Printf("security-sweep %s: clean (%.1fs)\n", args[0], elapsed)
		return
	}
	var parts []string
	for k, n := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", k, n))
	}
	fmt.Printf("security-sweep %s: %d finding(s) [%s] (%.1fs)\n", args[0], totalFindings, strings.Join(parts, ", "), elapsed)
}
