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
	"sync"
	"sync/atomic"
	"time"

	"github.com/CaptainCore/captaincore/apiclient"
	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/scan"
	"github.com/spf13/cobra"
)

var flagSecuritySweepJSON bool

// Process-wide compiled default scanner. Compiling the ~6,000 malware rules takes
// ~10s on the fleet server; sharing one immutable *scan.Scanner across db-scan,
// root PHP content scan, wp-config scan, and in-process bulk sweeps pays that
// cost at most once per process (and zero times when every file hits cache).
var (
	sharedScannerOnce sync.Once
	sharedScanner     *scan.Scanner
	sharedScannerErr  error
)

func getDefaultScanner() (*scan.Scanner, error) {
	sharedScannerOnce.Do(func() {
		rs, err := scan.LoadDefaultRuleSet()
		if err != nil {
			sharedScannerErr = err
			return
		}
		sharedScanner = scan.New(rs, scan.Options{Workers: 2})
	})
	return sharedScanner, sharedScannerErr
}

// cleanHashCache persists sha256 hashes of files (wp-config.php, root PHP) that
// scanned completely clean under the current rulesHash(). Any change to the file
// or to the rule set invalidates the entry automatically.
type cleanHashCache struct {
	mu        sync.RWMutex
	path      string
	rulesHash string
	hashes    map[string]bool
	dirty     bool
}

type cleanHashCacheDisk struct {
	RulesHash string          `json:"rules_hash"`
	Hashes    map[string]bool `json:"hashes"`
}

func defaultCleanHashCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".captaincore", "data", "clean-scan-hashes.json")
}

func loadCleanHashCache(path, curRulesHash string) *cleanHashCache {
	c := &cleanHashCache{
		path:      path,
		rulesHash: curRulesHash,
		hashes:    map[string]bool{},
	}
	if path == "" {
		return c
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var disk cleanHashCacheDisk
	if json.Unmarshal(b, &disk) == nil && disk.RulesHash == curRulesHash && disk.Hashes != nil {
		c.hashes = disk.Hashes
	}
	return c
}

func (c *cleanHashCache) Has(sha string) bool {
	if c == nil || sha == "" {
		return false
	}
	c.mu.RLock()
	ok := c.hashes[strings.ToLower(sha)]
	c.mu.RUnlock()
	return ok
}

func (c *cleanHashCache) Add(sha string) {
	if c == nil || sha == "" {
		return
	}
	sha = strings.ToLower(sha)
	c.mu.Lock()
	if !c.hashes[sha] {
		c.hashes[sha] = true
		c.dirty = true
	}
	c.mu.Unlock()
}

func (c *cleanHashCache) Save() error {
	if c == nil || c.path == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(cleanHashCacheDisk{
		RulesHash: c.rulesHash,
		Hashes:    c.hashes,
	})
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		return err
	}
	c.dirty = false
	return nil
}

var securitySweepCmd = &cobra.Command{
	Use:   "security-sweep <site|@target>",
	Short: "Fast behavioral & persistence security sweep (~2s per site)",
	Long: `Runs a lightweight security pulse over one site or a fleet target (@production, @all):
- Session & privilege snapshot (new admins, injected caps, role escalations, new app passwords)
- High-risk captaincore_security_log events (file editor saves, WPCode, app passwords, admin_email)
- PHP eval()'d code errors tailed from server error logs
- Unexpected PHP files at web root / wp-content root + rule scan
- Redacted wp-config.php rule scan (cached by rulesHash + sha256)

Bulk @target runs execute in-process with a shared compiled scanner so 3,000 sites
finish in minutes without recompiling malware rules per site.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|@target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, securitySweepNative)
	},
}

func init() {
	rootCmd.AddCommand(securitySweepCmd)
	securitySweepCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 20, "Number of sites to sweep concurrently")
	securitySweepCmd.Flags().BoolVar(&flagSecuritySweepJSON, "json", false, "Output JSON summary")
}

// wpConfigFindings scans the redacted wp-config.php payload with the native malware
// rule set. Uses nil cache; callers that want cross-run caching use wpConfigFindingsWithCache.
func wpConfigFindings(raw string, minSeverity string) []scan.LegacyFinding {
	return wpConfigFindingsWithCache(raw, minSeverity, nil)
}

func wpConfigFindingsWithCache(raw string, minSeverity string, cache *cleanHashCache) []scan.LegacyFinding {
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
	if doc.Sha256 != "" && cache.Has(doc.Sha256) {
		return nil
	}
	body, err := base64.StdEncoding.DecodeString(doc.B64)
	if err != nil || len(body) == 0 {
		return nil
	}
	s, err := getDefaultScanner()
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
	res := s.ScanDir(dir)
	if len(res.Findings) == 0 && doc.Sha256 != "" {
		cache.Add(doc.Sha256)
		return nil
	}
	min := scan.SeverityRank(minSeverity)
	seen := map[string]bool{}
	var out []scan.LegacyFinding
	for _, f := range res.Findings {
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

type sweepResult struct {
	Site     string
	Status   string // "clean", "findings", "error"
	Findings int
	Counts   map[string]int
	Elapsed  float64
	ErrMsg   string
}

func sweepOneSite(target string, system *config.SystemConfig, captain *config.CaptainConfig, client *apiclient.Client, cache *cleanHashCache) sweepResult {
	started := time.Now()
	sa := parseSiteArgument(target)
	res := sweepResult{Site: target, Counts: map[string]int{}}

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		res.Status = "error"
		res.ErrMsg = "site not found"
		res.Elapsed = time.Since(started).Seconds()
		return res
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		res.Status = "error"
		res.ErrMsg = "environment not found"
		res.Elapsed = time.Since(started).Seconds()
		return res
	}

	sshCmd := exec.Command("captaincore", "ssh", target, "--script=security-sweep", "--captain-id="+captainID)
	sshOutput, _ := sshCmd.Output()
	if len(sshOutput) == 0 {
		res.Status = "error"
		res.ErrMsg = "ssh failed"
		res.Elapsed = time.Since(started).Seconds()
		return res
	}
	if strings.TrimSpace(string(sshOutput)) == "WordPress not found" {
		res.Status = "error"
		res.ErrMsg = "wordpress not found"
		res.Elapsed = time.Since(started).Seconds()
		return res
	}

	data := parseSiteData(string(sshOutput))

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
			res.Counts["provenance"] = len(findings)
			res.Findings += len(findings)
		}
	}

	// 3. Unexpected root/content PHP rule scan + redacted wp-config.php rule scan (cache-backed)
	var provScan []scan.LegacyFinding
	if v := strings.TrimSpace(data["unexpected_root_php_content"]); v != "" && v != "[]" {
		provScan = append(provScan, rootPHPContentFindingsWithCache(v, nativeAlertSeverity, cache)...)
	}
	if v := strings.TrimSpace(data["wp_config_check"]); v != "" && v != "null" {
		provScan = append(provScan, wpConfigFindingsWithCache(v, nativeAlertSeverity, cache)...)
	}
	if len(provScan) > 0 {
		postMalwareAlert(site, env, system, captain, provScan, "provenance-scan")
		res.Counts["provenance-scan"] = len(provScan)
		res.Findings += len(provScan)
	}

	// 4. High-risk captaincore_security_log behavioral events
	if v := strings.TrimSpace(data["security_log_events"]); v != "" && v != "[]" {
		if findings := securityLogEventFindings(v); len(findings) > 0 {
			postMalwareAlert(site, env, system, captain, findings, "security-log")
			res.Counts["security-log"] = len(findings)
			res.Findings += len(findings)
		}
	}

	// 5. PHP eval()'d code error-log tell
	if v := strings.TrimSpace(data["runtime_eval_errors"]); v != "" && v != "[]" {
		if findings := runtimeEvalFindings(v); len(findings) > 0 {
			postMalwareAlert(site, env, system, captain, findings, "runtime-log")
			res.Counts["runtime-log"] = len(findings)
			res.Findings += len(findings)
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
			res.Counts["config"] = len(cfgFindings)
			res.Findings += len(cfgFindings)
		}
	}

	res.Elapsed = time.Since(started).Seconds()
	if res.Findings > 0 {
		res.Status = "findings"
	} else {
		res.Status = "clean"
	}
	return res
}

func formatSweepLine(prefix string, r sweepResult) string {
	switch r.Status {
	case "error":
		return fmt.Sprintf("%ssecurity-sweep %s: %s (%.1fs)", prefix, r.Site, r.ErrMsg, r.Elapsed)
	case "findings":
		var parts []string
		for k, n := range r.Counts {
			parts = append(parts, fmt.Sprintf("%s=%d", k, n))
		}
		return fmt.Sprintf("%ssecurity-sweep %s: %d finding(s) [%s] (%.1fs)", prefix, r.Site, r.Findings, strings.Join(parts, ", "), r.Elapsed)
	default:
		return fmt.Sprintf("%ssecurity-sweep %s: clean (%.1fs)", prefix, r.Site, r.Elapsed)
	}
}

func securitySweepNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("security-sweep: error (config not found)")
		os.Exit(1)
	}
	client := newAPIClient(system, captain)
	cache := loadCleanHashCache(defaultCleanHashCachePath(), rulesHash())
	defer cache.Save()

	// Collect non-flag targets
	var rawTargets []string
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			rawTargets = append(rawTargets, a)
		}
	}

	isBulk := len(rawTargets) > 1 || (len(rawTargets) == 1 && strings.HasPrefix(rawTargets[0], "@"))
	if !isBulk {
		r := sweepOneSite(rawTargets[0], system, captain, client, cache)
		if flagSecuritySweepJSON {
			out, _ := json.Marshal(map[string]any{
				"site":        r.Site,
				"status":      r.Status,
				"elapsed_sec": r.Elapsed,
				"findings":    r.Findings,
				"by_source":   r.Counts,
				"error":       r.ErrMsg,
			})
			fmt.Println(string(out))
			return
		}
		fmt.Println(formatSweepLine("", r))
		return
	}

	// In-process bulk sweep: shares one compiled *scan.Scanner and one cleanHashCache
	// across all worker goroutines.
	sites, err := resolveTargets(rawTargets, captainID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving targets: %v\n", err)
		os.Exit(1)
	}
	if len(sites) == 0 {
		fmt.Fprintln(os.Stderr, "No sites matched the target.")
		os.Exit(1)
	}

	parallel := flagParallel
	if parallel <= 0 {
		parallel = 20
	}
	started := time.Now()
	if !flagSecuritySweepJSON {
		fmt.Printf("Running security-sweep on %d sites (parallel: %d)...\n", len(sites), parallel)
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var outMu sync.Mutex
	var doneCount, cleanCount, findingsCount, errCount int64
	var allResults []sweepResult

	for _, s := range sites {
		sem <- struct{}{}
		wg.Add(1)
		go func(siteTarget string) {
			defer wg.Done()
			defer func() { <-sem }()

			r := sweepOneSite(siteTarget, system, captain, client, cache)
			n := atomic.AddInt64(&doneCount, 1)
			switch r.Status {
			case "clean":
				atomic.AddInt64(&cleanCount, 1)
			case "findings":
				atomic.AddInt64(&findingsCount, 1)
			default:
				atomic.AddInt64(&errCount, 1)
			}

			outMu.Lock()
			if flagSecuritySweepJSON {
				allResults = append(allResults, r)
			} else {
				fmt.Println(formatSweepLine(fmt.Sprintf("[%d/%d] ", n, len(sites)), r))
			}
			outMu.Unlock()

			// Periodically persist cache so long sweeps keep progress if interrupted.
			if n%100 == 0 {
				cache.Save()
			}
		}(s)
	}
	wg.Wait()
	cache.Save()

	elapsed := time.Since(started)
	if flagSecuritySweepJSON {
		out, _ := json.Marshal(map[string]any{
			"total":       len(sites),
			"clean":       cleanCount,
			"findings":    findingsCount,
			"errors":      errCount,
			"elapsed_sec": elapsed.Seconds(),
			"results":     allResults,
		})
		fmt.Println(string(out))
		return
	}
	fmt.Printf("Sweep complete: %d site(s) (%d clean, %d with findings, %d errors) in %s\n",
		len(sites), cleanCount, findingsCount, errCount, elapsed.Round(time.Second))
}
