package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var (
	flagDriftJSON        bool
	flagDriftPlugin      string
	flagDriftTheme       string
	flagDriftCore        bool
	flagDriftThemes      bool
	flagDriftTarget      string
	flagDriftProvider    string
	flagDriftEnvironment string
	flagDriftTop         int
	flagDriftSort        string
	flagDriftSteer       bool
	flagDriftHashes      bool
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Show version distribution across sites",
	Long: `Show how plugin, theme, or WordPress core versions are distributed
across environments. Use --target to identify sites that have drifted
from a specific version. Run with no flags to see the top drifting plugins.`,
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, driftNative)
	},
}

type driftSiteInfo struct {
	Site        string `json:"site"`
	SiteID      uint   `json:"site_id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	HomeURL     string `json:"home_url"`
	Environment string `json:"environment,omitempty"`
}

type driftEntry struct {
	Version string          `json:"version"`
	Count   int             `json:"count"`
	Sites   []driftSiteInfo `json:"sites,omitempty"`
}

type pluginDriftSummary struct {
	Slug      string `json:"slug"`
	Latest    string `json:"latest"`
	Total     int    `json:"total"`
	OnLatest  int    `json:"on_latest"`
	Drifted   int    `json:"drifted"`
	Versions  int    `json:"versions"`
}

type driftHashEntry struct {
	Hash  string          `json:"hash"`
	Count int             `json:"count"`
	Sites []driftSiteInfo `json:"sites,omitempty"`
}

type driftVersionHashEntry struct {
	Version string            `json:"version"`
	Count   int               `json:"count"`
	Hashes  []*driftHashEntry `json:"hashes"`
	Sites   []driftSiteInfo   `json:"sites,omitempty"`
}

func driftNative(cmd *cobra.Command, args []string) {
	// Determine mode
	hasPlugin := flagDriftPlugin != ""
	hasTheme := flagDriftTheme != ""
	hasCore := flagDriftCore
	singleMode := hasPlugin || hasTheme || hasCore

	// Validate: can't combine single-component flags
	modeCount := 0
	if hasPlugin {
		modeCount++
	}
	if hasTheme {
		modeCount++
	}
	if hasCore {
		modeCount++
	}
	if modeCount > 1 {
		fmt.Println("Error: Specify only one of --plugin, --theme, or --core")
		return
	}

	if flagDriftHashes {
		if !hasPlugin && !hasTheme {
			fmt.Println("Error: --hashes requires --plugin or --theme")
			return
		}
		if hasCore {
			fmt.Println("Error: --hashes is not supported with --core")
			return
		}
		if flagDriftSteer {
			fmt.Println("Error: --hashes cannot be combined with --steer")
			return
		}
		if flagDriftTarget != "" {
			fmt.Println("Error: --hashes is not yet supported with --target")
			return
		}
	}

	// Normalize environment
	env := normalizeEnv(flagDriftEnvironment)

	if singleMode {
		driftSingleComponent(env, hasPlugin, hasTheme, hasCore)
	} else {
		driftOverview(env)
	}
}

// normalizeEnv normalizes the environment flag to match DB values.
func normalizeEnv(env string) string {
	switch strings.ToLower(env) {
	case "production":
		return "Production"
	case "staging":
		return "Staging"
	case "all":
		return "all"
	default:
		return env
	}
}

// driftSingleComponent handles --plugin, --theme, or --core mode.
func driftSingleComponent(env string, hasPlugin, hasTheme, hasCore bool) {
	var results []models.SiteEnvironmentResult

	query := models.DB.Table("captaincore_sites").
		Select("captaincore_sites.site, captaincore_sites.site_id, captaincore_sites.name, captaincore_sites.provider, captaincore_environments.environment, captaincore_environments.home_url, captaincore_environments.core, captaincore_environments.plugins, captaincore_environments.themes").
		Joins("INNER JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active")

	if env != "all" {
		query = query.Where("captaincore_environments.environment = ?", env)
	}
	if flagDriftProvider != "" {
		query = query.Where("captaincore_sites.provider = ?", flagDriftProvider)
	}
	if hasPlugin {
		query = query.Where("captaincore_environments.plugins LIKE ?", `%"name":"`+flagDriftPlugin+`"%`)
	}
	if hasTheme {
		query = query.Where("captaincore_environments.themes LIKE ?", `%"name":"`+flagDriftTheme+`"%`)
	}

	query.Order("captaincore_sites.name ASC").Find(&results)

	// Hash-aware path
	if flagDriftHashes {
		vhMap := make(map[string]*driftVersionHashEntry)
		for _, r := range results {
			version, hash := extractVersionAndHash(r, flagDriftPlugin, flagDriftTheme)
			if version == "" {
				continue
			}
			vhEntry, ok := vhMap[version]
			if !ok {
				vhEntry = &driftVersionHashEntry{Version: version}
				vhMap[version] = vhEntry
			}
			vhEntry.Count++
			site := driftSiteInfo{
				Site:        r.Site,
				SiteID:      r.SiteID,
				Name:        r.Name,
				Provider:    r.Provider,
				HomeURL:     r.HomeURL,
				Environment: r.Environment,
			}
			vhEntry.Sites = append(vhEntry.Sites, site)

			// Find or create hash sub-entry
			found := false
			for _, he := range vhEntry.Hashes {
				if he.Hash == hash {
					he.Count++
					he.Sites = append(he.Sites, site)
					found = true
					break
				}
			}
			if !found {
				vhEntry.Hashes = append(vhEntry.Hashes, &driftHashEntry{
					Hash: hash, Count: 1, Sites: []driftSiteInfo{site},
				})
			}
		}

		if len(vhMap) == 0 {
			label, slug := componentLabel(hasPlugin, hasTheme, hasCore)
			fmt.Printf("No sites found with %s \"%s\" installed.\n", label, slug)
			suggestSlugs(results, slug, hasPlugin)
			return
		}

		sorted := sortedVersionsFromHashMap(vhMap)
		label, slug := componentLabel(hasPlugin, hasTheme, hasCore)
		outputDistributionWithHashes(sorted, vhMap, label, slug, env)
		return
	}

	// Aggregate by version
	versionMap := make(map[string]*driftEntry)
	for _, r := range results {
		version := extractVersion(r, flagDriftPlugin, flagDriftTheme, hasCore)
		if version == "" {
			continue
		}
		entry, ok := versionMap[version]
		if !ok {
			entry = &driftEntry{Version: version}
			versionMap[version] = entry
		}
		entry.Count++
		entry.Sites = append(entry.Sites, driftSiteInfo{
			Site:        r.Site,
			SiteID:      r.SiteID,
			Name:        r.Name,
			Provider:    r.Provider,
			HomeURL:     r.HomeURL,
			Environment: r.Environment,
		})
	}

	if len(versionMap) == 0 {
		label, slug := componentLabel(hasPlugin, hasTheme, hasCore)
		fmt.Printf("No sites found with %s \"%s\" installed.\n", label, slug)
		if hasPlugin || hasTheme {
			suggestSlugs(results, slug, hasPlugin)
		}
		return
	}

	// Sort versions descending
	sorted := sortedVersions(versionMap)

	// Determine component label for output
	label, slug := componentLabel(hasPlugin, hasTheme, hasCore)

	if flagDriftSteer {
		if hasCore {
			fmt.Println("Error: --steer is not supported with --core")
			return
		}
		driftSteer(sorted, versionMap, slug, hasPlugin, env)
		return
	}

	if flagDriftTarget != "" {
		outputTarget(sorted, versionMap, label, slug, env)
	} else {
		outputDistribution(sorted, versionMap, label, slug, env)
	}
}

// driftOverview handles the default overview mode — top drifting plugins/themes.
func driftOverview(env string) {
	var results []models.SiteEnvironmentResult

	query := models.DB.Table("captaincore_sites").
		Select("captaincore_sites.site, captaincore_sites.site_id, captaincore_sites.name, captaincore_sites.provider, captaincore_environments.environment, captaincore_environments.home_url, captaincore_environments.core, captaincore_environments.plugins, captaincore_environments.themes").
		Joins("INNER JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active")

	if env != "all" {
		query = query.Where("captaincore_environments.environment = ?", env)
	}
	if flagDriftProvider != "" {
		query = query.Where("captaincore_sites.provider = ?", flagDriftProvider)
	}

	query.Order("captaincore_sites.name ASC").Find(&results)

	// Build map: slug -> version -> count
	type versionCounts map[string]int
	slugVersions := make(map[string]versionCounts)
	componentType := "plugins"
	if flagDriftThemes {
		componentType = "themes"
	}

	for _, r := range results {
		var jsonData string
		if componentType == "plugins" {
			jsonData = r.Plugins
		} else {
			jsonData = r.Themes
		}
		if jsonData == "" {
			continue
		}
		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &items); err != nil {
			continue
		}
		for _, item := range items {
			name, _ := item["name"].(string)
			version, _ := item["version"].(string)
			if name == "" || version == "" {
				continue
			}
			if slugVersions[name] == nil {
				slugVersions[name] = make(versionCounts)
			}
			slugVersions[name][version]++
		}
	}

	// Build summaries for slugs with 2+ versions
	var summaries []pluginDriftSummary
	for slug, versions := range slugVersions {
		if len(versions) < 2 {
			continue
		}
		// Find latest version and total count
		var latest string
		total := 0
		for v, count := range versions {
			total += count
			if latest == "" || compareVersions(v, latest) > 0 {
				latest = v
			}
		}
		onLatest := versions[latest]
		summaries = append(summaries, pluginDriftSummary{
			Slug:     slug,
			Latest:   latest,
			Total:    total,
			OnLatest: onLatest,
			Drifted:  total - onLatest,
			Versions: len(versions),
		})
	}

	// Sort
	if flagDriftSort == "spread" {
		sort.Slice(summaries, func(i, j int) bool {
			if summaries[i].Versions != summaries[j].Versions {
				return summaries[i].Versions > summaries[j].Versions
			}
			return summaries[i].Drifted > summaries[j].Drifted
		})
	} else {
		sort.Slice(summaries, func(i, j int) bool {
			if summaries[i].Drifted != summaries[j].Drifted {
				return summaries[i].Drifted > summaries[j].Drifted
			}
			return summaries[i].Versions > summaries[j].Versions
		})
	}

	// Limit to top N
	totalWithDrift := len(summaries)
	if flagDriftTop > 0 && len(summaries) > flagDriftTop {
		summaries = summaries[:flagDriftTop]
	}

	if len(summaries) == 0 {
		typeLabel := "plugins"
		if flagDriftThemes {
			typeLabel = "themes"
		}
		fmt.Printf("No %s with version drift found.\n", typeLabel)
		return
	}

	// Output
	typeLabel := "plugins"
	if flagDriftThemes {
		typeLabel = "themes"
	}

	if flagDriftJSON {
		data := map[string]interface{}{
			"type":             typeLabel,
			"environment":      env,
			"total_sites":      len(results),
			"total_with_drift": totalWithDrift,
			"showing":          len(summaries),
			"sort":             flagDriftSort,
			"results":          summaries,
		}
		result, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(result))
		return
	}

	fmt.Printf("Top %d drifting %s (%s, %s sites):\n\n", len(summaries), typeLabel, env, formatNumber(len(results)))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PLUGIN\tLATEST\tON LATEST\tDRIFTED\tVERSIONS")
	for _, s := range summaries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", s.Slug, s.Latest, formatNumber(s.OnLatest), formatNumber(s.Drifted), s.Versions)
	}
	w.Flush()

	fmt.Printf("\n%d of %d %s with version drift\n", len(summaries), totalWithDrift, typeLabel)
}

// extractVersion returns the version string for a given result.
func extractVersion(r models.SiteEnvironmentResult, pluginSlug, themeSlug string, isCore bool) string {
	if isCore {
		if r.Core == "" {
			return "unknown"
		}
		return r.Core
	}

	var jsonData string
	var slug string
	if pluginSlug != "" {
		jsonData = r.Plugins
		slug = pluginSlug
	} else {
		jsonData = r.Themes
		slug = themeSlug
	}

	if jsonData == "" {
		return ""
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &items); err != nil {
		return ""
	}
	for _, item := range items {
		if name, _ := item["name"].(string); name == slug {
			version, _ := item["version"].(string)
			return version
		}
	}
	return ""
}

// extractVersionAndHash returns the version and hash for a given result.
func extractVersionAndHash(r models.SiteEnvironmentResult, pluginSlug, themeSlug string) (string, string) {
	var jsonData string
	var slug string
	if pluginSlug != "" {
		jsonData = r.Plugins
		slug = pluginSlug
	} else {
		jsonData = r.Themes
		slug = themeSlug
	}

	if jsonData == "" {
		return "", ""
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &items); err != nil {
		return "", ""
	}
	for _, item := range items {
		if name, _ := item["name"].(string); name == slug {
			version, _ := item["version"].(string)
			hash, _ := item["hash"].(string)
			return version, hash
		}
	}
	return "", ""
}

// suggestSlugs searches all environments for plugin/theme slugs that partially match the query.
func suggestSlugs(results []models.SiteEnvironmentResult, query string, isPlugin bool) {
	// Results may be empty due to the LIKE pre-filter, so do a fresh query
	var envs []models.SiteEnvironmentResult
	col := "captaincore_environments.plugins"
	if !isPlugin {
		col = "captaincore_environments.themes"
	}
	models.DB.Table("captaincore_sites").
		Select(col+" AS plugins, "+col+" AS themes").
		Joins("INNER JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active").
		Where("captaincore_environments.environment = ?", "Production").
		Find(&envs)

	queryLower := strings.ToLower(query)
	// Normalize: strip spaces/hyphens for fuzzy comparison
	queryNorm := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(queryLower)

	type slugMatch struct {
		slug  string
		title string
		count int
	}
	seen := make(map[string]*slugMatch)

	for _, r := range envs {
		var jsonData string
		if isPlugin {
			jsonData = r.Plugins
		} else {
			jsonData = r.Themes
		}
		if jsonData == "" {
			continue
		}
		var items []map[string]interface{}
		if json.Unmarshal([]byte(jsonData), &items) != nil {
			continue
		}
		for _, item := range items {
			name, _ := item["name"].(string)
			title, _ := item["title"].(string)
			if name == "" {
				continue
			}
			nameNorm := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(name))
			titleLower := strings.ToLower(title)
			if strings.Contains(nameNorm, queryNorm) || strings.Contains(titleLower, queryLower) {
				if m, ok := seen[name]; ok {
					m.count++
				} else {
					seen[name] = &slugMatch{slug: name, title: title, count: 1}
				}
			}
		}
	}

	if len(seen) == 0 {
		return
	}

	// Sort by count descending
	matches := make([]*slugMatch, 0, len(seen))
	for _, m := range seen {
		matches = append(matches, m)
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].count > matches[j].count
	})

	// Show up to 10 suggestions
	limit := 10
	if len(matches) < limit {
		limit = len(matches)
	}

	fmt.Printf("\nDid you mean?\n")
	for _, m := range matches[:limit] {
		fmt.Printf("  %s  (%s, %s sites)\n", m.slug, m.title, formatNumber(m.count))
	}
}

// steerConn holds SSH/SCP connection details for a site.
type steerConn struct {
	beforeSSH   string
	remoteOpts  string
	user        string
	host        string
	port        string
	commandPrep string
}

// newSteerConn builds SSH/SCP connection details for a site, following the same
// pattern as sshNative in cmd/ssh.go.
func newSteerConn(site *models.Site, env *models.Environment, system *config.SystemConfig) (*steerConn, error) {
	siteDetails := site.ParseDetails()

	remoteOpts := "-q -oStrictHostKeyChecking=no -oConnectTimeout=30 -oServerAliveInterval=60 -oServerAliveCountMax=10"
	beforeSSH := ""

	key := siteDetails.Key
	if key != "use_password" && key == "" {
		cid, _ := strconv.ParseUint(captainID, 10, 64)
		configValue, _ := models.GetConfiguration(uint(cid), "configurations")
		if configValue != "" {
			var configObj map[string]json.RawMessage
			if json.Unmarshal([]byte(configValue), &configObj) == nil {
				if defaultKeyRaw, ok := configObj["default_key"]; ok {
					var defaultKey string
					json.Unmarshal(defaultKeyRaw, &defaultKey)
					key = defaultKey
				}
			}
		}
	}

	if key != "use_password" {
		remoteOpts = fmt.Sprintf("%s -oPreferredAuthentications=publickey -i %s/%s/%s", remoteOpts, system.PathKeys, captainID, key)
	} else {
		beforeSSH = fmt.Sprintf("sshpass -p '%s'", env.Password)
	}

	conn := &steerConn{
		beforeSSH:  beforeSSH,
		remoteOpts: remoteOpts,
	}

	switch site.Provider {
	case "kinsta":
		conn.commandPrep = "cd public/ &&"
		conn.user = env.Username
		conn.host = env.Address
		conn.port = env.Port
	case "wpengine":
		conn.commandPrep = "cd sites/* &&"
		conn.user = site.Site
		conn.host = site.Site + ".ssh.wpengine.net"
		conn.port = ""
	case "rocketdotnet":
		conn.commandPrep = fmt.Sprintf("cd %s/ &&", env.HomeDirectory)
		conn.user = env.Username
		conn.host = env.Address
		conn.port = env.Port
	default:
		conn.commandPrep = fmt.Sprintf("cd %s/ &&", env.HomeDirectory)
		conn.user = env.Username
		conn.host = env.Address
		conn.port = env.Port
	}

	return conn, nil
}

func (c *steerConn) runSSH(command string) (string, error) {
	sshCmd := fmt.Sprintf("%s ssh %s %s@%s", c.beforeSSH, c.remoteOpts, c.user, c.host)
	if c.port != "" {
		sshCmd += " -p " + c.port
	}
	sshCmd += fmt.Sprintf(" \"%s %s\"", c.commandPrep, command)
	sshCmd = strings.TrimSpace(sshCmd)

	cmd := exec.Command("bash", "-c", sshCmd)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (c *steerConn) scpFrom(remotePath, localPath string) error {
	scpCmd := fmt.Sprintf("%s scp %s", c.beforeSSH, c.remoteOpts)
	if c.port != "" {
		scpCmd += " -P " + c.port
	}
	scpCmd += fmt.Sprintf(" %s@%s:%s %s", c.user, c.host, remotePath, localPath)
	scpCmd = strings.TrimSpace(scpCmd)

	cmd := exec.Command("bash", "-c", scpCmd)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (c *steerConn) scpTo(localPath, remotePath string) error {
	scpCmd := fmt.Sprintf("%s scp %s", c.beforeSSH, c.remoteOpts)
	if c.port != "" {
		scpCmd += " -P " + c.port
	}
	scpCmd += fmt.Sprintf(" %s %s@%s:%s", localPath, c.user, c.host, remotePath)
	scpCmd = strings.TrimSpace(scpCmd)

	cmd := exec.Command("bash", "-c", scpCmd)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type steerResult struct {
	Site        string `json:"site"`
	SiteID      uint   `json:"site_id"`
	Environment string `json:"environment"`
	OldVersion  string `json:"old_version"`
	NewVersion  string `json:"new_version"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

type driftedSiteWithVersion struct {
	driftSiteInfo
	version string
}

// driftSteer upgrades all drifted sites to the latest version by grabbing the
// plugin/theme zip from a source site and deploying it to each drifted site.
func driftSteer(sorted []string, versionMap map[string]*driftEntry, slug string, isPlugin bool, env string) {
	latestVersion := sorted[0]

	// Partition sites into source candidates and drifted
	var drifted []driftedSiteWithVersion
	onLatestCount := 0
	var sourceSite *driftSiteInfo

	for _, v := range sorted {
		entry := versionMap[v]
		if v == latestVersion {
			onLatestCount = entry.Count
			if len(entry.Sites) > 0 {
				s := entry.Sites[0]
				sourceSite = &s
			}
		} else {
			for _, s := range entry.Sites {
				drifted = append(drifted, driftedSiteWithVersion{
					driftSiteInfo: s,
					version:       v,
				})
			}
		}
	}

	if len(drifted) == 0 {
		fmt.Println("All sites are on the latest version. Nothing to steer.")
		return
	}

	if sourceSite == nil {
		fmt.Println("Error: No source site found on the latest version.")
		return
	}

	// Print summary
	componentType := "Plugin"
	if !isPlugin {
		componentType = "Theme"
	}
	outdatedVersions := len(sorted) - 1

	fmt.Printf("%s: %s\n", componentType, slug)
	fmt.Printf("Latest: %s (on %s sites)\n", latestVersion, formatNumber(onLatestCount))
	fmt.Printf("Drifted: %s sites across %d outdated versions\n\n", formatNumber(len(drifted)), outdatedVersions)
	fmt.Printf("Source: %s (%s) — will grab zip from this site\n\n", sourceSite.Site, sourceSite.Name)

	if !flagForce {
		fmt.Println("Add --force to proceed with upgrade.")
		return
	}

	// Load config once for SSH key paths
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Step 1: Extract zip from source site
	fmt.Printf("Extracting %s from %s...\n", slug, sourceSite.Site)

	sourceSiteModel, err := models.GetSiteByName(sourceSite.Site)
	if err != nil || sourceSiteModel == nil {
		fmt.Printf("Error: Source site '%s' not found in database.\n", sourceSite.Site)
		return
	}

	sourceEnvs, err := models.FindEnvironmentsBySiteID(sourceSiteModel.SiteID)
	if err != nil || len(sourceEnvs) == 0 {
		fmt.Printf("Error: No environments found for source site '%s'.\n", sourceSite.Site)
		return
	}

	var sourceEnv *models.Environment
	for _, e := range sourceEnvs {
		if strings.EqualFold(e.Environment, sourceSite.Environment) {
			sourceEnv = &e
			break
		}
	}
	if sourceEnv == nil {
		fmt.Printf("Error: %s environment not found for source site '%s'.\n", sourceSite.Environment, sourceSite.Site)
		return
	}

	conn, err := newSteerConn(sourceSiteModel, sourceEnv, system)
	if err != nil {
		fmt.Printf("Error building connection: %s\n", err)
		return
	}

	componentDir := "wp-content/plugins"
	wpCmd := "wp plugin install"
	if !isPlugin {
		componentDir = "wp-content/themes"
		wpCmd = "wp theme install"
	}

	remoteZip := fmt.Sprintf("/tmp/%s.zip", slug)
	localZip := fmt.Sprintf("/tmp/captaincore-steer-%s-%s.zip", slug, latestVersion)

	// SSH to source: zip the plugin/theme directory
	zipCmd := fmt.Sprintf("cd %s && zip -qr %s %s/", componentDir, remoteZip, slug)
	output, err := conn.runSSH(zipCmd)
	if err != nil {
		fmt.Printf("Error zipping on source site: %s\n%s\n", err, output)
		return
	}

	// SCP the zip down to the VPS
	err = conn.scpFrom(remoteZip, localZip)
	if err != nil {
		fmt.Printf("Error downloading zip from source: %s\n", err)
		return
	}

	// Cleanup remote zip on source
	conn.runSSH(fmt.Sprintf("rm -f %s", remoteZip))

	fmt.Printf("Deploying to %s sites...\n", formatNumber(len(drifted)))

	// Step 2: Deploy to all drifted sites in parallel
	parallel := flagParallel
	if parallel <= 0 {
		parallel = 10
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []steerResult
	successCount := 0
	failCount := 0

	for _, d := range drifted {
		wg.Add(1)
		go func(ds driftedSiteWithVersion) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := steerResult{
				Site:        ds.Site,
				SiteID:      ds.SiteID,
				Environment: ds.Environment,
				OldVersion:  ds.version,
				NewVersion:  latestVersion,
			}

			// Look up site
			dsite, err := models.GetSiteByName(ds.Site)
			if err != nil || dsite == nil {
				result.Status = "failed"
				result.Error = "site not found"
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			denvs, err := models.FindEnvironmentsBySiteID(dsite.SiteID)
			if err != nil || len(denvs) == 0 {
				result.Status = "failed"
				result.Error = "environment not found"
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			var denv *models.Environment
			for _, e := range denvs {
				if strings.EqualFold(e.Environment, ds.Environment) {
					denv = &e
					break
				}
			}
			if denv == nil {
				result.Status = "failed"
				result.Error = "environment not found"
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			dconn, err := newSteerConn(dsite, denv, system)
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			// SCP zip up to target site
			err = dconn.scpTo(localZip, remoteZip)
			if err != nil {
				result.Status = "failed"
				result.Error = "scp upload failed"
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			// Install the plugin/theme
			installCmd := fmt.Sprintf("%s %s --force 2>&1", wpCmd, remoteZip)
			installOutput, err := dconn.runSSH(installCmd)

			// Cleanup remote zip
			dconn.runSSH(fmt.Sprintf("rm -f %s", remoteZip))

			if err != nil {
				result.Status = "failed"
				result.Error = strings.TrimSpace(installOutput)
				mu.Lock()
				results = append(results, result)
				failCount++
				mu.Unlock()
				return
			}

			result.Status = "success"
			mu.Lock()
			results = append(results, result)
			successCount++
			mu.Unlock()
		}(d)
	}

	wg.Wait()

	// Cleanup local zip
	os.Remove(localZip)

	// Sort results by site name
	sort.Slice(results, func(i, j int) bool {
		return results[i].Site < results[j].Site
	})

	// Output results
	if flagDriftJSON {
		data := map[string]interface{}{
			"type":    componentType,
			"slug":    slug,
			"target":  latestVersion,
			"source":  sourceSite.Site,
			"success": successCount,
			"failed":  failCount,
			"total":   len(results),
			"results": results,
		}
		jsonResult, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(jsonResult))
		return
	}

	fmt.Printf("\nUpgrade complete: %s → %s\n\n", slug, latestVersion)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SITE\tSITE_ID\tENVIRONMENT\tOLD VERSION\tSTATUS")
	for _, r := range results {
		status := "✓"
		if r.Status == "failed" {
			status = "✗ (" + r.Error + ")"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", r.Site, r.SiteID, r.Environment, r.OldVersion, status)
	}
	w.Flush()

	fmt.Printf("\n%s of %s sites upgraded successfully.", formatNumber(successCount), formatNumber(len(results)))
	if failCount > 0 {
		fmt.Printf(" %s failed.", formatNumber(failCount))
	}
	fmt.Println()
}

// componentLabel returns a display label and slug for the current component.
func componentLabel(hasPlugin, hasTheme, hasCore bool) (string, string) {
	if hasPlugin {
		return "plugin", flagDriftPlugin
	}
	if hasTheme {
		return "theme", flagDriftTheme
	}
	return "core", "WordPress"
}

// outputDistribution renders the version distribution table.
func outputDistribution(sorted []string, versionMap map[string]*driftEntry, label, slug, env string) {
	totalSites := 0
	for _, e := range versionMap {
		totalSites += e.Count
	}

	if flagDriftJSON {
		versions := make([]map[string]interface{}, 0, len(sorted))
		for _, v := range sorted {
			entry := versionMap[v]
			sites := make([]map[string]interface{}, 0, len(entry.Sites))
			for _, s := range entry.Sites {
				sites = append(sites, map[string]interface{}{
					"site":        s.Site,
					"site_id":     s.SiteID,
					"environment": s.Environment,
					"name":        s.Name,
					"provider":    s.Provider,
					"home_url":    s.HomeURL,
				})
			}
			versions = append(versions, map[string]interface{}{
				"version": v,
				"count":   entry.Count,
				"sites":   sites,
			})
		}
		data := map[string]interface{}{
			"type":         label,
			"slug":         slug,
			"environment":  env,
			"total_sites":  totalSites,
			"total_versions": len(sorted),
			"versions":     versions,
		}
		result, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(result))
		return
	}

	// Capitalize label for display
	displayLabel := strings.ToUpper(label[:1]) + label[1:]
	fmt.Printf("%s: %s\n", displayLabel, slug)
	fmt.Printf("Environment: %s\n\n", env)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VERSION\tSITES")
	for _, v := range sorted {
		fmt.Fprintf(w, "%s\t%s\n", v, formatNumber(versionMap[v].Count))
	}
	w.Flush()

	fmt.Printf("\nTotal: %s sites across %d versions\n", formatNumber(totalSites), len(sorted))
}

// sortedVersionsFromHashMap returns version strings sorted descending and sorts hashes within each version by count descending.
func sortedVersionsFromHashMap(vhMap map[string]*driftVersionHashEntry) []string {
	versions := make([]string, 0, len(vhMap))
	for v := range vhMap {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	for _, vhEntry := range vhMap {
		sort.Slice(vhEntry.Hashes, func(i, j int) bool {
			return vhEntry.Hashes[i].Count > vhEntry.Hashes[j].Count
		})
	}
	return versions
}

// outputDistributionWithHashes renders the version distribution with hash sub-grouping.
func outputDistributionWithHashes(sorted []string, vhMap map[string]*driftVersionHashEntry, label, slug, env string) {
	totalSites := 0
	totalDistinctHashes := 0
	for _, vhEntry := range vhMap {
		totalSites += vhEntry.Count
		totalDistinctHashes += len(vhEntry.Hashes)
	}

	if flagDriftJSON {
		versions := make([]map[string]interface{}, 0, len(sorted))
		for _, v := range sorted {
			vhEntry := vhMap[v]
			hashes := make([]map[string]interface{}, 0, len(vhEntry.Hashes))
			for _, he := range vhEntry.Hashes {
				sites := make([]map[string]interface{}, 0, len(he.Sites))
				for _, s := range he.Sites {
					sites = append(sites, map[string]interface{}{
						"site":        s.Site,
						"site_id":     s.SiteID,
						"environment": s.Environment,
						"name":        s.Name,
						"provider":    s.Provider,
						"home_url":    s.HomeURL,
					})
				}
				hashes = append(hashes, map[string]interface{}{
					"hash":  he.Hash,
					"count": he.Count,
					"sites": sites,
				})
			}
			versions = append(versions, map[string]interface{}{
				"version":         v,
				"count":           vhEntry.Count,
				"distinct_hashes": len(vhEntry.Hashes),
				"hashes":          hashes,
			})
		}
		data := map[string]interface{}{
			"type":                  label,
			"slug":                  slug,
			"environment":           env,
			"total_sites":           totalSites,
			"total_versions":        len(sorted),
			"total_distinct_hashes": totalDistinctHashes,
			"versions":              versions,
		}
		result, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(result))
		return
	}

	// Table mode
	displayLabel := strings.ToUpper(label[:1]) + label[1:]
	fmt.Printf("%s: %s\n", displayLabel, slug)
	fmt.Printf("Environment: %s\n\n", env)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VERSION\tSITES\tHASHES")
	for _, v := range sorted {
		vhEntry := vhMap[v]
		fmt.Fprintf(w, "%s\t%s\t%d\n", v, formatNumber(vhEntry.Count), len(vhEntry.Hashes))
		for _, he := range vhEntry.Hashes {
			hashLabel := he.Hash
			if hashLabel == "" {
				hashLabel = "(no hash)"
			} else if len(hashLabel) > 8 {
				hashLabel = hashLabel[:8]
			}
			fmt.Fprintf(w, "  %s\t%s\t\n", hashLabel, formatNumber(he.Count))
		}
	}
	w.Flush()

	fmt.Printf("\nTotal: %s sites across %d versions, %d distinct hashes\n",
		formatNumber(totalSites), len(sorted), totalDistinctHashes)
}

// outputTarget renders the drift detection view with --target.
func outputTarget(sorted []string, versionMap map[string]*driftEntry, label, slug, env string) {
	// Resolve "latest" to the highest version found
	if flagDriftTarget == "latest" && len(sorted) > 0 {
		flagDriftTarget = sorted[0]
	}

	totalSites := 0
	onTarget := 0
	var driftedSites []driftSiteInfo

	for _, v := range sorted {
		entry := versionMap[v]
		totalSites += entry.Count
		if v == flagDriftTarget {
			onTarget = entry.Count
		} else {
			for i := range entry.Sites {
				driftedSites = append(driftedSites, entry.Sites[i])
			}
		}
	}

	// Sort drifted sites: by version (ascending) then name
	sort.Slice(driftedSites, func(i, j int) bool {
		// Find version for each site
		vi := findSiteVersion(driftedSites[i], versionMap)
		vj := findSiteVersion(driftedSites[j], versionMap)
		cmp := compareVersions(vi, vj)
		if cmp != 0 {
			return cmp < 0
		}
		return driftedSites[i].Name < driftedSites[j].Name
	})

	if flagDriftJSON {
		// Build drifted list with version info
		type driftedJSON struct {
			Version     string `json:"version"`
			Site        string `json:"site"`
			SiteID      uint   `json:"site_id"`
			Environment string `json:"environment"`
			Name        string `json:"name"`
			Provider    string `json:"provider"`
			HomeURL     string `json:"home_url"`
		}
		drifted := make([]driftedJSON, 0, len(driftedSites))
		for _, s := range driftedSites {
			drifted = append(drifted, driftedJSON{
				Version:     findSiteVersion(s, versionMap),
				Site:        s.Site,
				SiteID:      s.SiteID,
				Environment: s.Environment,
				Name:        s.Name,
				Provider:    s.Provider,
				HomeURL:     s.HomeURL,
			})
		}
		data := map[string]interface{}{
			"type":          label,
			"slug":          slug,
			"target":        flagDriftTarget,
			"environment":   env,
			"on_target":     onTarget,
			"drifted_count": len(driftedSites),
			"total_sites":   totalSites,
			"drifted":       drifted,
		}
		result, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(result))
		return
	}

	displayLabel := strings.ToUpper(label[:1]) + label[1:]
	fmt.Printf("%s: %s\n", displayLabel, slug)
	fmt.Printf("Target: %s\n", flagDriftTarget)
	fmt.Printf("Environment: %s\n\n", env)
	fmt.Printf("On target: %s sites\n\n", formatNumber(onTarget))

	if len(driftedSites) == 0 {
		fmt.Println("All sites are on the target version.")
		return
	}

	fmt.Printf("Drifted sites (%s):\n\n", formatNumber(len(driftedSites)))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "VERSION\tSITE\tSITE_ID\tENVIRONMENT\tDOMAIN\tHOME_URL\tPROVIDER")
	for _, s := range driftedSites {
		v := findSiteVersion(s, versionMap)
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n", v, s.Site, s.SiteID, s.Environment, s.Name, s.HomeURL, s.Provider)
	}
	w.Flush()

	fmt.Printf("\n%s of %s sites have drifted from target version %s\n",
		formatNumber(len(driftedSites)), formatNumber(totalSites), flagDriftTarget)
}

// findSiteVersion looks up which version a site belongs to in the version map.
func findSiteVersion(site driftSiteInfo, versionMap map[string]*driftEntry) string {
	for version, entry := range versionMap {
		for _, s := range entry.Sites {
			if s.SiteID == site.SiteID && s.Site == site.Site {
				return version
			}
		}
	}
	return "unknown"
}

// sortedVersions returns version strings sorted descending (newest first).
func sortedVersions(versionMap map[string]*driftEntry) []string {
	versions := make([]string, 0, len(versionMap))
	for v := range versionMap {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	return versions
}

// compareVersions compares two version strings.
// Returns >0 if a > b, <0 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}
	for i := 0; i < maxLen; i++ {
		var segA, segB string
		if i < len(partsA) {
			segA = partsA[i]
		}
		if i < len(partsB) {
			segB = partsB[i]
		}
		numA, errA := strconv.Atoi(segA)
		numB, errB := strconv.Atoi(segB)
		if errA == nil && errB == nil {
			if numA != numB {
				return numA - numB
			}
		} else {
			if segA != segB {
				if segA < segB {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func init() {
	rootCmd.AddCommand(driftCmd)
	driftCmd.Flags().BoolVar(&flagDriftJSON, "json", false, "Output as JSON")
	driftCmd.Flags().StringVar(&flagDriftPlugin, "plugin", "", "Plugin slug to check")
	driftCmd.Flags().StringVar(&flagDriftTheme, "theme", "", "Theme slug to check")
	driftCmd.Flags().BoolVar(&flagDriftCore, "core", false, "Check WordPress core versions")
	driftCmd.Flags().BoolVar(&flagDriftThemes, "themes", false, "Overview mode: rank themes instead of plugins")
	driftCmd.Flags().StringVar(&flagDriftTarget, "target", "", "Target version — use 'latest' to auto-detect (shows sites NOT on this version)")
	driftCmd.Flags().StringVar(&flagDriftProvider, "provider", "", "Filter by hosting provider")
	driftCmd.Flags().StringVar(&flagDriftEnvironment, "environment", "Production", "Filter by environment (Production, Staging, all)")
	driftCmd.Flags().IntVar(&flagDriftTop, "top", 20, "Overview mode: number of results to show")
	driftCmd.Flags().StringVar(&flagDriftSort, "sort", "volume", "Overview mode: sort by volume (drifted count) or spread (version count)")
	driftCmd.Flags().BoolVar(&flagDriftHashes, "hashes", false, "Show hash breakdown within each version (use with --plugin or --theme)")
	driftCmd.Flags().BoolVar(&flagDriftSteer, "steer", false, "Upgrade all drifted sites to the latest version")
	driftCmd.Flags().BoolVarP(&flagForce, "force", "", false, "Execute the steer upgrade (required with --steer)")
	driftCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 10, "Number of parallel deployments")
}
