package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/CaptainCore/captaincore/scan"
	"github.com/spf13/cobra"
)

// The nightly quicksave hook scans only the files that changed in the latest
// commit, so anything already present when a site was onboarded was never
// looked at (a self-hiding backdoor rode through a migration unscanned,
// 2026-09-18). Two additions close that:
//
//   - a full-tree native scan when a quicksave has no parent commit, when the
//     tree has not had one for fullScanInterval, or when the rule set changed
//     since the last one (stamped in <env>/.malware-full-scan)
//   - a hidden-plugin check comparing the plugin directories on disk with the
//     inventory WordPress reported to the Manager
//
// Both alert through the same malware-alert path as the nightly hooks.

const fullScanInterval = 7 * 24 * time.Hour

type fullScanStamp struct {
	Time      string `json:"time"`
	RulesHash string `json:"rules_hash"`
	Files     int    `json:"files"`
	Findings  int    `json:"findings"`
}

func fullScanStampPath(sitePath string) string {
	return filepath.Join(filepath.Dir(sitePath), ".malware-full-scan")
}

// rulesHash fingerprints the rule files so a rule change triggers a rescan.
func rulesHash() string {
	h := sha256.New()
	for _, p := range scan.DefaultRulePaths() {
		if b, err := os.ReadFile(p); err == nil {
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// fullScanWorkers caps the scanner inside the nightly hook: quicksave
// generate already runs sixteen sites in parallel on the fleet server.
const fullScanWorkers = 2

// fullScanDue reports whether a full-tree scan should run now and why.
// A first quicksave always scans. A backlog (no stamp, or a rule change)
// is spread across the week by site id so a deploy does not trigger every
// site's full scan on the same night; the weekly rescan then stays spread.
func fullScanDue(sitePath string, firstCommit bool, now time.Time, siteID uint) (bool, string) {
	if firstCommit {
		return true, "first quicksave"
	}
	spread := func(reason string) (bool, string) {
		if int(siteID%7) == int(now.Weekday()) {
			return true, reason
		}
		return false, ""
	}
	b, err := os.ReadFile(fullScanStampPath(sitePath))
	if err != nil {
		return spread("never scanned")
	}
	var st fullScanStamp
	if json.Unmarshal(b, &st) != nil {
		return true, "unreadable stamp"
	}
	if st.RulesHash != rulesHash() {
		return spread("rules changed")
	}
	t, err := time.Parse(time.RFC3339, st.Time)
	if err != nil || now.Sub(t) >= fullScanInterval {
		return true, "weekly"
	}
	return false, ""
}

func writeFullScanStamp(sitePath string, files, findings int) {
	b, _ := json.Marshal(fullScanStamp{Time: time.Now().UTC().Format(time.RFC3339), RulesHash: rulesHash(), Files: files, Findings: findings})
	os.WriteFile(fullScanStampPath(sitePath), append(b, '\n'), 0o644)
}

// quicksaveFullScan runs the native scanner over the whole quicksave tree and
// alerts on findings at nativeAlertSeverity and above. Wordfence is not run
// here: the tree is the whole site and the native engine does it in seconds.
func quicksaveFullScan(sitePath, reason string, site *models.Site, env *models.Environment, system *config.SystemConfig, captain *config.CaptainConfig) {
	rs, err := scan.LoadDefaultRuleSet()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Full scan: %v\n", err)
		return
	}
	s := scan.New(rs, scan.Options{Workers: fullScanWorkers})
	res := s.ScanDir(sitePath)
	min := scan.SeverityRank(nativeAlertSeverity)
	var alert []scan.LegacyFinding
	for _, f := range res.Findings {
		if scan.SeverityRank(f.Severity) < min {
			continue
		}
		l := f.Legacy()
		l.Filename = f.Path
		alert = append(alert, l)
	}
	writeFullScanStamp(sitePath, res.Scanned, len(alert))
	if len(alert) == 0 {
		fmt.Printf("  Full scan (%s): %d file(s) checked, clean\n", reason, res.Scanned)
		return
	}
	fmt.Printf("Full scan (%s): %d finding(s) on %s-%s\n", reason, len(alert), site.Site, strings.ToLower(env.Environment))
	for _, f := range alert {
		fmt.Printf("  %s — %s\n", f.Filename, f.SignatureName)
	}
	postMalwareAlert(site, env, system, captain, alert, "native-full")
}

func postMalwareAlert(site *models.Site, env *models.Environment, system *config.SystemConfig, captain *config.CaptainConfig, findings []scan.LegacyFinding, source string) {
	client := newAPIClient(system, captain)
	client.Post("malware-alert", map[string]interface{}{
		"site_id": site.SiteID,
		"data": map[string]interface{}{
			"site_name":   site.Name,
			"environment": env.Environment,
			"home_url":    env.HomeURL,
			"findings":    findings,
			"source":      source,
		},
	})
}

// hiddenPlugin is a plugins/ directory WordPress did not report.
type hiddenPlugin struct {
	Dir       string `json:"dir"`
	Title     string `json:"title,omitempty"` // Plugin Name: from the header, when present
	HasHeader bool   `json:"has_header"`      // a top-level PHP file carries a "Plugin Name:" header
	PHPFiles  int    `json:"php_files"`
	Empty     bool   `json:"empty"`
	AgeDays   int    `json:"age_days"` // days since the directory first appeared in quicksave history (-1 unknown)
}

// hiddenPluginMinAge is how long an unlisted plugin directory must have been
// in quicksave history before it counts as hidden: the nightly sync has had
// several chances to report it by then.
const hiddenPluginMinAge = 3

// hiddenPluginSiteCap: more unlisted plugins than this on one environment
// means the inventory itself is broken or stale, not that they are hidden.
const hiddenPluginSiteCap = 5

// inventory holds the plugin names and titles the Manager synced for an environment.
type inventory struct {
	names  map[string]bool
	titles map[string]bool
}

func normalizeTitle(t string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(t) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// inventoryNames returns the plugin slugs and titles the Manager holds for an environment.
func inventoryNames(env *models.Environment) (inventory, bool) {
	inv := inventory{names: map[string]bool{}, titles: map[string]bool{}}
	if strings.TrimSpace(env.Plugins) == "" {
		return inv, false
	}
	var list []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(env.Plugins), &list); err != nil {
		return inv, false
	}
	for _, p := range list {
		inv.names[strings.ToLower(p.Name)] = true
		if t := normalizeTitle(p.Title); t != "" {
			inv.titles[t] = true
		}
	}
	return inv, len(list) > 0
}

var premiumSuffixes = []string{"-pro", "-premium", "-lite", "-free", "-paid", "-plus"}

// listed reports whether a directory is accounted for by the inventory: by
// name, by name without a Freemius-style premium suffix, or by header title
// (Freemius premium builds live in <slug>-pro but report the free slug).
func (inv inventory) listed(dir, title string) bool {
	d := strings.ToLower(dir)
	if inv.names[d] {
		return true
	}
	for _, suf := range premiumSuffixes {
		if strings.HasSuffix(d, suf) && inv.names[strings.TrimSuffix(d, suf)] {
			return true
		}
		if inv.names[d+suf] {
			return true
		}
	}
	if t := normalizeTitle(title); t != "" {
		if inv.titles[t] {
			return true
		}
		for _, suf := range []string{"premium", "pro", "lite", "free"} {
			if inv.titles[strings.TrimSuffix(t, suf)] || inv.titles[t+suf] {
				return true
			}
		}
	}
	return false
}

// pluginHeaderTitle returns the Plugin Name: from a top-level PHP file, if any.
func pluginHeaderTitle(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || scan.ExtOf(e.Name()) != ".php" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		head := string(b[:min(len(b), 8192)])
		if i := strings.Index(head, "Plugin Name:"); i >= 0 {
			rest := head[i+len("Plugin Name:"):]
			if nl := strings.IndexAny(rest, "\r\n"); nl >= 0 {
				rest = rest[:nl]
			}
			return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest), "*/")), true
		}
	}
	return "", false
}

// firstSeenDays returns how many days ago rel first appeared in the quicksave
// repository, or -1 when history is unavailable.
func firstSeenDays(sitePath, rel string) int {
	out, err := exec.Command("git", "-C", sitePath, "log", "--diff-filter=A", "--format=%ct", "--", rel).Output()
	if err != nil {
		return -1
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return -1
	}
	ts, err := strconv.ParseInt(lines[len(lines)-1], 10, 64)
	if err != nil {
		return -1
	}
	return int(time.Since(time.Unix(ts, 0)).Hours() / 24)
}

// hiddenPlugins compares plugins/ on disk with the inventory. Returned
// entries are directories the inventory does not account for; callers decide
// what to alert on using HasHeader, Empty and AgeDays.
func hiddenPlugins(sitePath string, inv inventory) ([]hiddenPlugin, error) {
	dir := filepath.Join(sitePath, "plugins")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []hiddenPlugin
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		title, hasHeader := pluginHeaderTitle(filepath.Join(dir, e.Name()))
		if inv.listed(e.Name(), title) {
			continue
		}
		hp := hiddenPlugin{Dir: e.Name(), Title: title, HasHeader: hasHeader, AgeDays: -1}
		filepath.WalkDir(filepath.Join(dir, e.Name()), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && scan.ExtOf(p) == ".php" {
				hp.PHPFiles++
			}
			return nil
		})
		if hp.PHPFiles == 0 {
			sub, _ := os.ReadDir(filepath.Join(dir, e.Name()))
			hp.Empty = len(sub) == 0
		}
		if hp.HasHeader || hp.Empty {
			hp.AgeDays = firstSeenDays(sitePath, "plugins/"+e.Name())
		}
		out = append(out, hp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dir < out[j].Dir })
	return out, nil
}

// randomDirName reports whether a directory name looks machine-generated
// (letters and digits mixed, no separators), the way decoy directories
// dropped next to a backdoor are named.
var randomDirName = regexp.MustCompile(`^[a-z0-9]{6,16}$`)

func looksRandom(name string) bool {
	n := strings.ToLower(name)
	if !randomDirName.MatchString(n) {
		return false
	}
	digits, letters := 0, 0
	for _, r := range n {
		if r >= '0' && r <= '9' {
			digits++
		} else {
			letters++
		}
	}
	return digits >= 2 && letters >= 1
}

// hiddenPluginVerdict classifies one unlisted directory.
//
//	hidden      a plugin with a header, in history long enough for the sync to have seen it
//	new         a plugin with a header that appeared recently; wait for the next sync
//	decoy       an empty directory with a machine-looking name
//	leftover    an empty directory with an ordinary slug, PHP without a plugin header, or only non-PHP files
func hiddenPluginVerdict(h hiddenPlugin) string {
	switch {
	case h.HasHeader && (h.AgeDays < 0 || h.AgeDays >= hiddenPluginMinAge):
		return "hidden"
	case h.HasHeader:
		return "new"
	case h.Empty && looksRandom(h.Dir):
		return "decoy"
	}
	return "leftover"
}

// inventoryStale reports whether an environment has so many unlisted plugins
// with headers that its synced inventory cannot be trusted.
func inventoryStale(found []hiddenPlugin) (int, bool) {
	headers := 0
	for _, h := range found {
		if h.HasHeader {
			headers++
		}
	}
	return headers, headers > hiddenPluginSiteCap
}

// inventoryMatchesTree reports whether the synced inventory describes the
// plugins/ directory in the quicksave at all. A site with a custom
// WP_PLUGIN_DIR (Bedrock-style app/plugins) reports plugins that never
// appear under wp-content/plugins, so nothing there can be judged hidden.
func inventoryMatchesTree(sitePath string, inv inventory) bool {
	if len(inv.names) < 3 {
		return true // too small to judge; let the per-directory verdicts decide
	}
	entries, err := os.ReadDir(filepath.Join(sitePath, "plugins"))
	if err != nil {
		return true
	}
	matched := 0
	for _, e := range entries {
		if e.IsDir() && inv.listed(e.Name(), "") {
			matched++
		}
	}
	// A quicksave mirrors every plugin WordPress reports, so a healthy tree
	// accounts for nearly all of the inventory. Well under half means the
	// inventory describes some other directory (a Bedrock app/plugins with a
	// stray shared plugin or two left under wp-content/plugins).
	return matched*2 >= len(inv.names)
}

// quicksaveHiddenPluginsCheck runs inside quicksave add. Hidden plugins
// alert; new, decoy and leftover directories are printed only. When an
// environment has more unlisted plugins than hiddenPluginSiteCap the
// inventory is treated as unreliable and nothing alerts.
func quicksaveHiddenPluginsCheck(sitePath string, site *models.Site, env *models.Environment, system *config.SystemConfig, captain *config.CaptainConfig) {
	inv, ok := inventoryNames(env)
	if !ok {
		return // no inventory synced yet; nothing to compare against
	}
	if !inventoryMatchesTree(sitePath, inv) {
		fmt.Printf("  Hidden plugin check skipped: the synced plugin list does not correspond to plugins/ (custom WP_PLUGIN_DIR?)\n")
		return
	}
	found, err := hiddenPlugins(sitePath, inv)
	if err != nil || len(found) == 0 {
		return
	}
	if headers, stale := inventoryStale(found); stale {
		fmt.Printf("  Hidden plugin check skipped: %d plugin directories are missing from the synced inventory, so the inventory looks stale\n", headers)
		return
	}
	var alert []scan.LegacyFinding
	for _, h := range found {
		switch hiddenPluginVerdict(h) {
		case "hidden":
			fmt.Printf("  Hidden plugin: plugins/%s (%q) has been in quicksave %d day(s) but WordPress does not report it\n", h.Dir, h.Title, h.AgeDays)
			alert = append(alert, scan.LegacyFinding{
				Filename:             filepath.Join(sitePath, "plugins", h.Dir),
				SignatureID:          "hidden-plugin",
				SignatureName:        "Plugin hidden from WordPress",
				SignatureDescription: fmt.Sprintf("plugins/%s (%s, %d PHP file(s)) has been present for %d day(s) but is missing from the plugin list WordPress reported; self-hiding backdoors filter themselves out of that list", h.Dir, h.Title, h.PHPFiles, h.AgeDays),
			})
		case "decoy":
			fmt.Printf("  Decoy plugin directory: plugins/%s is empty\n", h.Dir)
		}
	}
	if len(alert) > 0 {
		postMalwareAlert(site, env, system, captain, alert, "hidden-plugin")
	}
}

var quicksaveHiddenPluginsCmd = &cobra.Command{
	Use:   "hidden-plugins <site|@target>",
	Short: "List plugin directories in a quicksave that WordPress did not report",
	Long: `Compares the plugins/ directories in each quicksave with the plugin inventory
synced from the site. A directory with a plugin header that WordPress does not
list is a plugin hiding itself (the SMILODON backdoor does this); an empty or
header-less directory is a decoy or a leftover. Nothing is changed or alerted;
use it to sweep the fleet.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> or @target argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, quicksaveHiddenPluginsNative)
	},
}

func quicksaveHiddenPluginsNative(cmd *cobra.Command, args []string) {
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}
	type row struct {
		Site        string         `json:"site"`
		Environment string         `json:"environment"`
		Hidden      []hiddenPlugin `json:"hidden"`
		Note        string         `json:"note,omitempty"` // custom-plugin-dir: inventory does not describe plugins/
	}
	var rows []row
	visit := func(site *models.Site, env *models.Environment) {
		sitePath := filepath.Join(system.Path, fmt.Sprintf("%s_%d", site.Site, site.SiteID), strings.ToLower(env.Environment), "quicksave")
		if _, err := os.Stat(filepath.Join(sitePath, "plugins")); err != nil {
			return
		}
		inv, ok := inventoryNames(env)
		if !ok {
			return
		}
		if !inventoryMatchesTree(sitePath, inv) {
			rows = append(rows, row{site.Site, env.Environment, nil, "custom-plugin-dir"})
			return
		}
		found, err := hiddenPlugins(sitePath, inv)
		if err != nil || len(found) == 0 {
			return
		}
		rows = append(rows, row{site.Site, env.Environment, found, ""})
	}
	if strings.HasPrefix(args[0], "@") {
		sites, err := models.GetAllActiveSites()
		if err != nil {
			fmt.Printf("Error fetching sites: %v\n", err)
			return
		}
		environment, _ := models.ParseTargetString(args[0])
		for i := range sites {
			envs, err := models.FindEnvironmentsBySiteID(sites[i].SiteID)
			if err != nil {
				continue
			}
			for j := range envs {
				if environment != "" && environment != "all" && !strings.EqualFold(envs[j].Environment, environment) {
					continue
				}
				visit(&sites[i], &envs[j])
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
		visit(site, env)
	}
	if flagFormat == "json" {
		out, _ := json.MarshalIndent(rows, "", "    ")
		fmt.Println(string(out))
		return
	}
	counts := map[string]int{}
	for _, r := range rows {
		label := fmt.Sprintf("%s-%s", r.Site, strings.ToLower(r.Environment))
		if r.Note == "custom-plugin-dir" {
			counts["custom"]++
			if flagAll {
				fmt.Printf("\033[90m-\033[0m %-34s synced plugin list does not correspond to plugins/ (custom WP_PLUGIN_DIR?)\n", label)
			}
			continue
		}
		if headers, stale := inventoryStale(r.Hidden); stale {
			counts["stale"]++
			fmt.Printf("\033[35m~\033[0m %-34s inventory stale: %d plugin directories with headers are unlisted; re-sync before trusting this environment\n", label, headers)
			continue
		}
		for _, h := range r.Hidden {
			v := hiddenPluginVerdict(h)
			counts[v]++
			switch v {
			case "hidden":
				fmt.Printf("\033[31m✗\033[0m %-34s plugins/%-36s hidden plugin %q, %d PHP files, in quicksave %d days\n", label, h.Dir, h.Title, h.PHPFiles, h.AgeDays)
			case "new":
				fmt.Printf("\033[33m?\033[0m %-34s plugins/%-36s new plugin %q, %d days old, not yet in inventory\n", label, h.Dir, h.Title, h.AgeDays)
			case "decoy":
				fmt.Printf("\033[33m!\033[0m %-34s plugins/%-36s empty directory\n", label, h.Dir)
			default:
				if flagAll {
					fmt.Printf("\033[90m-\033[0m %-34s plugins/%-36s leftover, %d PHP files, no plugin header\n", label, h.Dir, h.PHPFiles)
				}
			}
		}
	}
	fmt.Printf("%d hidden, %d new, %d decoy, %d leftover, %d stale inventor(ies), %d custom plugin dir(s) across %d environment(s).\n", counts["hidden"], counts["new"], counts["decoy"], counts["leftover"], counts["stale"], counts["custom"], len(rows))
}

func init() {
	quicksaveCmd.AddCommand(quicksaveHiddenPluginsCmd)
	quicksaveHiddenPluginsCmd.Flags().StringVar(&flagFormat, "format", "", "Output format (json)")
	quicksaveHiddenPluginsCmd.Flags().BoolVar(&flagAll, "all", false, "Also list leftover directories (PHP without a plugin header)")
}
