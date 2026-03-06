package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagBackupZipName, flagBackupSite, flagBackupSiteID string

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup commands",
}

var backupCheckCmd = &cobra.Command{
	Use:   "check <site>",
	Short: "Checks integrity of backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if flagInit {
			os.Setenv("FLAG_INIT", "true")
		}
		resolveCommand(cmd, args)
	},
}

var backupDownloadCmd = &cobra.Command{
	Use:   "download <site> <backup-id> <payload> [--email=<email>]",
	Short: "Download a backup for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <backup-id> <payload> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "Generates new backup for a site",
	Run: func(cmd *cobra.Command, args []string) {
		if flagDryRun && len(args) > 0 {
			dryRunGenerate(args[0], "backups")
			return
		}
		resolveCommand(cmd, args)
	},
}

var backupGetCmd = &cobra.Command{
	Use:   "get <site> <backup-id>",
	Short: "Fetches backup for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <site> and <backup-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupGetGenerateCmd = &cobra.Command{
	Use:   "get-generate <site> <backup-id>",
	Short: "Generate contents of a backup",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupGetGenerateNative)
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "Fetches list of snapshots for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "Generates list of snapshots for a site",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupListGenerateNative)
	},
}

var backupListMissingCmd = &cobra.Command{
	Use:   "list-missing <site>",
	Short: "Generates list of snapshots for a site that haven't been generated",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupListMissingNative)
	},
}

// backupListMissingNative implements `captaincore backup list-missing <site>` natively in Go.
func backupListMissingNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	listPath := filepath.Join(system.Path, siteDir, sa.Environment, "backups", "list.json")
	data, err := os.ReadFile(listPath)
	if err != nil {
		return
	}

	var snapshots []struct {
		ID json.Number `json:"id"`
	}
	if json.Unmarshal(data, &snapshots) != nil {
		return
	}

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)

	for _, snapshot := range snapshots {
		snapshotID := snapshot.ID.String()
		backupFilesLink := fmt.Sprintf("%s/%s/%s/backups/snapshot-%s.json", system.RcloneUploadURI, siteDir, sa.Environment, snapshotID)

		// Check HTTP status with HEAD request
		resp, err := http.Head(backupFilesLink)
		if err != nil || resp.StatusCode != 200 {
			fmt.Printf("Generating missing %s/%s/backups/snapshot-%s.json\n", siteDir, sa.Environment, snapshotID)
			getGenCmd := exec.Command("captaincore", "backup", "get-generate", siteEnvArg, snapshotID, "--captain-id="+captainID)
			getGenCmd.Stdout = os.Stdout
			getGenCmd.Stderr = os.Stderr
			getGenCmd.Run()
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
}

// backupListGenerateNative implements `captaincore backup list-generate <site>` natively in Go.
func backupListGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)

	resticCmd := exec.Command("restic", "snapshots", "--repo", resticRepo, "--password-file="+resticKey, "--json")
	output, err := resticCmd.Output()
	if err != nil {
		fmt.Println("Error: Backup repo not found.")
		return
	}

	var snapshots []map[string]interface{}
	if json.Unmarshal(output, &snapshots) != nil {
		return
	}

	// Strip unwanted fields
	for _, snap := range snapshots {
		delete(snap, "hostname")
		delete(snap, "username")
		delete(snap, "paths")
		delete(snap, "uid")
		delete(snap, "excludes")
		delete(snap, "gid")
	}

	result, _ := json.MarshalIndent(snapshots, "", "    ")

	// Write to list.json file
	backupsDir := filepath.Join(system.Path, siteDir, envName, "backups")
	os.MkdirAll(backupsDir, 0755)
	listPath := filepath.Join(backupsDir, "list.json")
	fmt.Printf("Generating %s/%s/backups/list.json\n", siteDir, envName)
	os.WriteFile(listPath, result, 0644)

	// Update environment details with backup_count
	updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
		"backup_count": len(snapshots),
	}, system, captain)
}

// resticItem represents a parsed line from restic ls JSON output.
type resticItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// folderUsageInfo tracks aggregated size and count for omitted directories.
type folderUsageInfo struct {
	Size  int64
	Count int
}

// FileNode represents a node in the backup file tree.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Type     string     `json:"type"`
	Count    int        `json:"count"`
	Size     int64      `json:"size"`
	Ext      string     `json:"ext,omitempty"`
	Children []FileNode `json:"children,omitempty"`
}

// backupGetGenerateNative implements `captaincore backup get-generate <site> [backup-id]` natively in Go.
func backupGetGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)

	// Determine backup_id
	backupID := ""
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		backupID = args[1]
	}

	if backupID == "" {
		// Try reading list.json for latest
		listPath := filepath.Join(system.Path, siteDir, envName, "backups", "list.json")
		data, readErr := os.ReadFile(listPath)
		if readErr != nil {
			// Run list-generate first
			siteEnvArg := fmt.Sprintf("%s-%s", site.Site, envName)
			listGenCmd := exec.Command("captaincore", "backup", "list-generate", siteEnvArg, "--captain-id="+captainID)
			listGenCmd.Stdout = os.Stdout
			listGenCmd.Stderr = os.Stderr
			listGenCmd.Run()
			data, readErr = os.ReadFile(listPath)
		}
		if readErr == nil {
			var snapshots []struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(data, &snapshots) == nil && len(snapshots) > 0 {
				backupID = snapshots[len(snapshots)-1].ID
			}
		}
		if backupID == "" {
			fmt.Println("Error: No backup ID found.")
			return
		}
		fmt.Printf("Backup id not selected. Generating response for latest ID %s\n", backupID)
	}

	// Run restic ls
	resticCmd := exec.Command("restic", "ls", "-l", backupID, "/", "--recursive", "--repo", resticRepo, "--json", "--password-file="+resticKey)
	resticOutput, err := resticCmd.Output()
	if err != nil {
		fmt.Println("Error: Backup repo not found.")
		return
	}

	fmt.Printf("Generating %s/%s/backups/snapshot-%s.json\n", siteDir, envName, backupID)

	// Parse JSONL output
	var items []resticItem
	omitItems := []string{"/wp-content/uploads/", "/wp-content/blog.dir/"}
	scanner := bufio.NewScanner(strings.NewReader(string(resticOutput)))
	// Increase scanner buffer for large outputs
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var row resticItem
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.Path == "" {
			continue
		}
		items = append(items, row)
	}

	omit := len(items) > 50000
	folderUsage := make(map[string]folderUsageInfo)

	var filteredItems []resticItem
	for _, item := range items {
		if omit && item.Type == "file" {
			shouldOmit := false
			for _, prefix := range omitItems {
				if strings.HasPrefix(item.Path, prefix) {
					shouldOmit = true
					break
				}
			}
			if shouldOmit {
				dir := filepath.Dir(item.Path)
				usage := folderUsage[dir]
				usage.Size += item.Size
				usage.Count++
				folderUsage[dir] = usage
				continue
			}
		}
		filteredItems = append(filteredItems, item)
	}

	// Apply folder usage to dir entries
	for i, item := range filteredItems {
		if usage, ok := folderUsage[item.Path]; ok && item.Type == "dir" {
			filteredItems[i].Size = usage.Size
		}
	}

	// Build hierarchical tree
	omitted, tree := buildFileTree(filteredItems, folderUsage)

	// Sort tree recursively
	sortFileTree(tree)

	result := map[string]interface{}{
		"omitted": omitted,
		"files":   tree,
	}
	resultJSON, _ := json.Marshal(result)

	// Write snapshot file
	backupsDir := filepath.Join(system.Path, siteDir, envName, "backups")
	os.MkdirAll(backupsDir, 0755)
	snapshotPath := filepath.Join(backupsDir, fmt.Sprintf("snapshot-%s.json", backupID))
	os.WriteFile(snapshotPath, resultJSON, 0644)

	// Move to rclone upload remote
	if system.RcloneUpload != "" {
		rcloneDest := fmt.Sprintf("%s%s/%s/backups/", system.RcloneUpload, siteDir, envName)
		moveCmd := exec.Command("rclone", "move", snapshotPath, rcloneDest)
		moveCmd.Run()
	}
}

// buildFileTree converts flat restic items into a hierarchical tree.
func buildFileTree(items []resticItem, folderUsage map[string]folderUsageInfo) (bool, []FileNode) {
	type treeEntry struct {
		path     string
		typ      string
		size     int64
		count    int
		ext      string
		children map[string]*treeEntry
	}

	root := &treeEntry{children: make(map[string]*treeEntry)}
	omitted := false

	for _, item := range items {
		parts := strings.Split(item.Path, "/")
		current := root
		for _, part := range parts {
			if part == "" {
				continue
			}
			if current.children == nil {
				current.children = make(map[string]*treeEntry)
			}
			if _, ok := current.children[part]; !ok {
				ext := ""
				if strings.Contains(part, ".") {
					ext = part[strings.Index(part, ".")+1:]
				}
				count := 1
				size := item.Size
				// Check folder usage for omitted dirs
				if usage, ok := folderUsage[item.Path]; ok && item.Type == "dir" {
					size = usage.Size
					count = usage.Count
					omitted = true
				}
				current.children[part] = &treeEntry{
					path:     item.Path,
					typ:      item.Type,
					size:     size,
					count:    count,
					ext:      ext,
					children: make(map[string]*treeEntry),
				}
			}
			current = current.children[part]
		}
	}

	var buildNodes func(children map[string]*treeEntry) []FileNode
	buildNodes = func(children map[string]*treeEntry) []FileNode {
		var nodes []FileNode
		for name, entry := range children {
			node := FileNode{
				Name:  name,
				Path:  entry.path,
				Type:  entry.typ,
				Count: entry.count,
				Size:  entry.size,
				Ext:   entry.ext,
			}
			if len(entry.children) > 0 {
				node.Children = buildNodes(entry.children)
			}
			nodes = append(nodes, node)
		}
		return nodes
	}

	return omitted, buildNodes(root.children)
}

// sortFileTree sorts the tree recursively: dirs first (type ascending), then by name.
func sortFileTree(nodes []FileNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type < nodes[j].Type
		}
		return nodes[i].Name < nodes[j].Name
	})
	for i := range nodes {
		if len(nodes[i].Children) > 0 {
			sortFileTree(nodes[i].Children)
		}
	}
}

var backupVerifyCmd = &cobra.Command{
	Use:   "verify <site>",
	Short: "Verifies backup health for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupVerifyNative)
	},
}

// resticSnapshot represents a snapshot entry from restic snapshots --json output.
type resticSnapshot struct {
	ID   string `json:"id"`
	Time string `json:"time"`
}

// backupVerifyChecks runs the restic verification checks and returns any issues found.
func backupVerifyChecks(resticRepo, resticKey, envCore string) []string {
	var issues []string

	// Check 1: Snapshot freshness — query latest snapshot per host, then find the newest
	resticCmd := exec.Command("restic", "snapshots", "--repo", resticRepo, "--password-file="+resticKey, "--json", "--latest", "1")
	output, err := resticCmd.Output()
	if err != nil {
		issues = append(issues, "Backup repo not accessible or not initialized")
		return issues
	}

	var snapshots []resticSnapshot
	if json.Unmarshal(output, &snapshots) != nil || len(snapshots) == 0 {
		issues = append(issues, "No snapshots found in backup repo")
		return issues
	}

	// --latest 1 returns one per host/path combo; find the newest
	var latestSnapshot resticSnapshot
	var latestTime time.Time
	for _, snap := range snapshots {
		t, parseErr := time.Parse(time.RFC3339Nano, snap.Time)
		if parseErr != nil {
			t, parseErr = time.Parse("2006-01-02T15:04:05Z07:00", snap.Time)
		}
		if parseErr == nil && t.After(latestTime) {
			latestTime = t
			latestSnapshot = snap
		}
	}
	if latestTime.IsZero() {
		issues = append(issues, "Unable to parse snapshot times")
		return issues
	}

	age := time.Since(latestTime)
	if age > 36*time.Hour {
		issues = append(issues, fmt.Sprintf("Latest snapshot is stale (%.1f hours old, threshold 36h)", age.Hours()))
	}

	// Check 2: Database presence — only for WordPress sites
	if envCore != "" {
		lsCmd := exec.Command("restic", "ls", latestSnapshot.ID, "/database-backup.sql", "--repo", resticRepo, "--password-file="+resticKey)
		lsOutput, lsErr := lsCmd.Output()
		if lsErr != nil || !strings.Contains(string(lsOutput), "database-backup.sql") {
			issues = append(issues, "Database backup (database-backup.sql) missing from latest snapshot")
		}
	}

	return issues
}

// backupVerifyNative implements `captaincore backup verify <site>` natively in Go.
func backupVerifyNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
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

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)

	// Run checks with up to 3 attempts, 5s delay between retries
	maxAttempts := 3
	var issues []string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		issues = backupVerifyChecks(resticRepo, resticKey, env.Core)
		if len(issues) == 0 {
			break
		}
		if attempt < maxAttempts {
			fmt.Printf("Backup verify attempt #%d failed, retrying in 5s...\n", attempt)
			time.Sleep(5 * time.Second)
		}
	}

	siteEnvLabel := fmt.Sprintf("%s-%s", site.Site, envName)

	if len(issues) > 0 {
		fmt.Printf("Backup verify FAILED for %s:\n", siteEnvLabel)
		for _, issue := range issues {
			fmt.Printf("  - %s\n", issue)
		}

		// Update environment details with backup_health: failed
		updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
			"backup_health":        "failed",
			"backup_health_issues": issues,
		}, system, captain)

		// Send alert email via monitor-notify
		adminEmail := getVarString(captain, "captaincore_admin_email")
		if adminEmail != "" {
			emailContent := fmt.Sprintf("<div style=\"text-align: left;\">Backup verification failed for <strong>%s</strong>.<br /><br />", siteEnvLabel)
			emailContent += "Issues found:<br /><ul style=\"text-align: left;\">"
			for _, issue := range issues {
				emailContent += fmt.Sprintf("<li>%s</li>", issue)
			}
			emailContent += "</ul></div>"

			contentJSON, _ := json.Marshal(emailContent)
			client := newAPIClient(system, captain)
			client.Post("monitor-notify", map[string]interface{}{
				"data": map[string]interface{}{
					"email":   adminEmail,
					"subject": "Backup Alert: " + siteEnvLabel,
					"content": json.RawMessage(contentJSON),
				},
			})
		}
	} else {
		fmt.Printf("Backup verify OK for %s\n", siteEnvLabel)

		// Update environment details with backup_health: ok
		updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
			"backup_health":        "ok",
			"backup_health_issues": nil,
		}, system, captain)
	}
}

var backupPruneCmd = &cobra.Command{
	Use:   "prune <site>",
	Short: "Prunes restic backup repo to repack and remove unused data",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupPruneNative)
	},
}

// backupPruneNative implements `captaincore backup prune <site>` natively in Go.
func backupPruneNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
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

	rcloneBackup := getRcloneBackup(captain, system)
	resticKey := getResticKeyPath()
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	resticRepo := fmt.Sprintf("rclone:%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)

	fmt.Printf("Pruning backup repo for %s-%s\n", site.Site, envName)

	resticArgs := []string{
		"prune",
		"--repo", resticRepo,
		"--password-file=" + resticKey,
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
	}

	if flagDryRun {
		resticArgs = append(resticArgs, "--dry-run")
	}

	resticCmd := exec.Command("restic", resticArgs...)
	resticCmd.Stdout = os.Stdout
	resticCmd.Stderr = os.Stderr
	resticCmd.Run()
}

var backupShowCmd = &cobra.Command{
	Use:   "show <site> <backup-id> <file-id>",
	Short: "Retrieve individual file from site backup",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires a <site> <backup-id> and <file-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupRuntimeCmd = &cobra.Command{
	Use:   "runtime <site>",
	Short: "Returns runtimes of previous backups",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site>")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupRuntimeNative)
	},
}

// backupRuntimeNative implements `captaincore backup runtime <site>` natively in Go.
func backupRuntimeNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Println("Error: Site not found.")
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	runtimePath := filepath.Join(system.Path, siteDir, sa.Environment, "backups", "runtime")

	data, err := os.ReadFile(runtimePath)
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		start, err1 := strconv.ParseInt(parts[0], 10, 64)
		finish, err2 := strconv.ParseInt(parts[1], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}

		startTime := time.Unix(start, 0)
		duration := finish - start
		fmt.Printf("%s - %s\n", formatDateTimeHuman(startTime), secondsToTimeString(duration))
	}
}

var backupCleanupCmd = &cobra.Command{
	Use:   "cleanup <site>",
	Short: "Removes local backup/ folders for sites using remote backups",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupCleanupNative)
	},
}

// backupCleanupNative implements `captaincore backup cleanup <site>` natively in Go.
func backupCleanupNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	type cleanupTarget struct {
		SiteDir     string
		Environment string
		SiteID      uint
	}

	var targets []cleanupTarget
	target := args[0]
	isBulk := strings.HasPrefix(target, "@")

	if isBulk {
		environment, minorTargets := models.ParseTargetString(target)
		queryArgs := models.FetchSiteMatchingArgs{
			Environment: environment,
			Targets:     minorTargets,
		}
		results, err := models.FetchSitesMatching(queryArgs)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		for _, r := range results {
			targets = append(targets, cleanupTarget{
				SiteDir:     fmt.Sprintf("%s_%d", r.Site, r.SiteID),
				Environment: strings.ToLower(r.Environment),
				SiteID:      r.SiteID,
			})
		}
	} else {
		sa := parseSiteArgument(target)
		site, err := sa.LookupSite()
		if err != nil || site == nil {
			fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
			return
		}
		env, err := sa.LookupEnvironment(site.SiteID)
		if err != nil || env == nil {
			fmt.Printf("Error: Environment not found for '%s'.\n", target)
			return
		}
		targets = append(targets, cleanupTarget{
			SiteDir:     fmt.Sprintf("%s_%d", site.Site, site.SiteID),
			Environment: strings.ToLower(env.Environment),
			SiteID:      site.SiteID,
		})
	}

	var totalSize int64
	var cleanedCount int

	for _, t := range targets {
		// Look up site to check backup settings
		site, err := models.GetSiteByID(t.SiteID)
		if err != nil || site == nil {
			continue
		}
		siteDetails := site.ParseDetails()
		if siteDetails.BackupSettings.Mode == "local" {
			continue
		}

		backupPath := filepath.Join(system.Path, t.SiteDir, t.Environment, "backup")

		// Safety: resolve symlinks and verify the path is within system.Path
		absSystemPath, _ := filepath.Abs(system.Path)
		realBackupPath, err := filepath.EvalSymlinks(backupPath)
		if err == nil {
			// Path exists — verify it's under the data directory
			if !strings.HasPrefix(realBackupPath, absSystemPath+string(os.PathSeparator)) {
				fmt.Printf("Skipping %s/%s/backup/ (resolves outside data directory)\n", t.SiteDir, t.Environment)
				continue
			}
		}

		// Verify the path ends with /backup as expected
		if filepath.Base(backupPath) != "backup" {
			continue
		}

		info, err := os.Stat(backupPath)
		if err != nil || !info.IsDir() {
			continue
		}

		size, err := dirSize(backupPath)
		if err != nil || size == 0 {
			continue
		}

		if flagDryRun {
			if cleanedCount == 0 {
				fmt.Printf("%-40s %-14s %s\n", "Site", "Environment", "Size")
			}
			fmt.Printf("%-40s %-14s %s\n", t.SiteDir, t.Environment, formatBytes(strconv.FormatInt(size, 10)))
		} else {
			if err := os.RemoveAll(backupPath); err != nil {
				fmt.Printf("Error cleaning %s/%s/backup/: %v\n", t.SiteDir, t.Environment, err)
				continue
			}
			fmt.Printf("Cleaned up %s/%s/backup/ (%s)\n", t.SiteDir, t.Environment, formatBytes(strconv.FormatInt(size, 10)))
		}

		totalSize += size
		cleanedCount++
	}

	if cleanedCount > 0 {
		if flagDryRun {
			fmt.Printf("\nTotal reclaimable: %s across %d environments\n", formatBytes(strconv.FormatInt(totalSize, 10)), cleanedCount)
		} else {
			fmt.Printf("\nTotal reclaimed: %s across %d environments\n", formatBytes(strconv.FormatInt(totalSize, 10)), cleanedCount)
		}
	}
}

var flagConfirm bool

var backupStorageCleanupCmd = &cobra.Command{
	Use:   "storage-cleanup",
	Short: "Removes orphaned site folders from B2 backup storage",
	Run: func(cmd *cobra.Command, args []string) {
		backupStorageCleanupNative(cmd, args)
	},
}

// backupStorageCleanupNative implements `captaincore backup storage-cleanup` natively in Go.
func backupStorageCleanupNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)

	// Build set of active site folders from database
	results, err := models.FetchSitesMatching(models.FetchSiteMatchingArgs{})
	if err != nil {
		fmt.Printf("Error fetching sites: %v\n", err)
		return
	}

	activeFolders := make(map[string]bool)
	for _, r := range results {
		folder := fmt.Sprintf("%s_%d", r.Site, r.SiteID)
		activeFolders[folder] = true
	}

	if len(activeFolders) == 0 {
		fmt.Println("Error: No active sites found in database. Aborting to prevent accidental deletion.")
		return
	}

	fmt.Printf("Found %d active site folders in database\n", len(activeFolders))

	if rcloneBackup == "" {
		fmt.Println("Error: Backup storage path is empty. Check rclone_backup in your config.")
		return
	}

	// List folders in B2 storage
	lsdCmd := exec.Command("rclone", "lsd", rcloneBackup+"/")
	output, err := lsdCmd.Output()
	if err != nil {
		fmt.Printf("Error listing B2 storage: %v\n", err)
		return
	}

	var orphans []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		folder := fields[len(fields)-1]
		// Only consider folders matching site_id format (contains underscore with trailing number)
		parts := strings.Split(folder, "_")
		if len(parts) < 2 {
			continue
		}
		lastPart := parts[len(parts)-1]
		if _, err := strconv.Atoi(lastPart); err != nil {
			continue
		}
		if !activeFolders[folder] {
			orphans = append(orphans, folder)
		}
	}

	if len(orphans) == 0 {
		fmt.Println("No orphaned folders found.")
		return
	}

	fmt.Printf("\nFound %d orphaned folders:\n\n", len(orphans))
	fmt.Printf("%-50s %s\n", "Folder", "Size")

	var totalSize int64
	for _, folder := range orphans {
		remotePath := rcloneBackup + "/" + folder
		sizeCmd := exec.Command("rclone", "size", remotePath, "--json")
		sizeOutput, err := sizeCmd.Output()
		sizeStr := "unknown"
		if err == nil {
			var sizeResult struct {
				Bytes int64 `json:"bytes"`
			}
			if json.Unmarshal(sizeOutput, &sizeResult) == nil {
				totalSize += sizeResult.Bytes
				sizeStr = formatBytes(strconv.FormatInt(sizeResult.Bytes, 10))
			}
		}
		fmt.Printf("%-50s %s\n", folder, sizeStr)
	}

	fmt.Printf("\nTotal reclaimable: %s across %d folders\n", formatBytes(strconv.FormatInt(totalSize, 10)), len(orphans))

	if !flagConfirm {
		fmt.Println("\nRun with --confirm to delete these folders.")
		return
	}

	fmt.Println()
	for _, folder := range orphans {
		remotePath := rcloneBackup + "/" + folder
		fmt.Printf("Deleting %s...\n", folder)
		purgeCmd := exec.Command("rclone", "purge", remotePath)
		purgeCmd.Stdout = os.Stdout
		purgeCmd.Stderr = os.Stderr
		if err := purgeCmd.Run(); err != nil {
			fmt.Printf("Error deleting %s: %v\n", folder, err)
		}
	}

	fmt.Printf("\nDeleted %d orphaned folders.\n", len(orphans))
}

var backupFetchLinkCmd = &cobra.Command{
	Use:   "fetch-link",
	Short: "Fetches download link for a backup restore zip",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupFetchLinkNative)
	},
}

// backupFetchLinkNative implements `captaincore backup fetch-link` natively in Go.
func backupFetchLinkNative(cmd *cobra.Command, args []string) {
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	rcloneSnapshotPath := system.RcloneSnapshot
	if system.CaptainCoreFleet == "true" {
		rcloneSnapshotPath = rcloneSnapshotPath + "/" + captainID
	}
	siteFolder := fmt.Sprintf("%s_%s", flagBackupSite, flagBackupSiteID)
	rclonePath := fmt.Sprintf("%s/%s/%s", rcloneSnapshotPath, siteFolder, flagBackupZipName)

	out, err := exec.Command("rclone", "link", rclonePath, "--expire", "168h").Output()
	if err != nil {
		fmt.Printf("Error: Failed to generate download link: %v\n", err)
		return
	}
	fmt.Print(strings.TrimSpace(string(out)))
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupCheckCmd)
	backupCmd.AddCommand(backupDownloadCmd)
	backupCmd.AddCommand(backupGenerateCmd)
	backupCmd.AddCommand(backupGetCmd)
	backupCmd.AddCommand(backupGetGenerateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupListGenerateCmd)
	backupCmd.AddCommand(backupListMissingCmd)
	backupCmd.AddCommand(backupVerifyCmd)
	backupCmd.AddCommand(backupShowCmd)
	backupCmd.AddCommand(backupRuntimeCmd)
	backupCmd.AddCommand(backupCleanupCmd)
	backupCmd.AddCommand(backupFetchLinkCmd)
	backupCmd.AddCommand(backupPruneCmd)
	backupCmd.AddCommand(backupStorageCleanupCmd)
	backupStorageCleanupCmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Actually delete orphaned folders (default is dry-run)")
	backupPruneCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview what prune would do without making changes")
	backupCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Calculate reclaimable space without deleting")
	backupCheckCmd.Flags().BoolVarP(&flagInit, "init", "", false, "Initialize repo if missing")
	backupDownloadCmd.Flags().StringVarP(&flagEmail, "email", "e", "", "Email notify")
	backupGenerateCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 3, "Number of sites to backup at same time")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipDB, "skip-db", "", false, "Skip database backup")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipRemote, "skip-remote", "", false, "Skip remote backup")
	backupGenerateCmd.Flags().StringVarP(&flagSkipIfRecent, "skip-if-recent", "", "", "Skip if backup generated within timeframe (e.g. 24h)")
	backupGenerateCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview which environments would be processed without executing")
	backupFetchLinkCmd.Flags().StringVarP(&flagBackupZipName, "zip-name", "", "", "Name of the zip file")
	backupFetchLinkCmd.Flags().StringVarP(&flagBackupSite, "site", "", "", "Site slug")
	backupFetchLinkCmd.Flags().StringVarP(&flagBackupSiteID, "site-id", "", "", "Site ID")
}
