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
	"sync"
	"sync/atomic"
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

// resticSnapshotSummary holds the backup summary embedded in restic 0.17+ snapshots.
type resticSnapshotSummary struct {
	TotalBytesProcessed uint64 `json:"total_bytes_processed"`
}

// resticSnapshot represents a snapshot entry from restic snapshots --json output.
type resticSnapshot struct {
	ID       string                 `json:"id"`
	ShortID  string                 `json:"short_id"`
	Time     string                 `json:"time"`
	Hostname string                 `json:"hostname"`
	Summary  *resticSnapshotSummary `json:"summary,omitempty"`
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

var backupUnlockCmd = &cobra.Command{
	Use:   "unlock <site>",
	Short: "Removes stale locks from a restic backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, backupUnlockNative)
	},
}

// backupUnlockNative implements `captaincore backup unlock <site>` natively in Go.
func backupUnlockNative(cmd *cobra.Command, args []string) {
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

	fmt.Printf("Unlocking backup repo for %s-%s\n", site.Site, envName)

	resticArgs := []string{
		"unlock",
		"--repo", resticRepo,
		"--password-file=" + resticKey,
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
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

var flagSizes bool
var flagPrune bool

var backupSnapshotsCmd = &cobra.Command{
	Use:   "snapshots <site> [snapshot-id]",
	Short: "Lists all snapshots in a site's backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		backupSnapshotsNative(cmd, args)
	},
}

// resticStats represents the JSON output from restic stats.
type resticStats struct {
	TotalSize      uint64 `json:"total_size"`
	TotalFileCount uint64 `json:"total_file_count"`
}

// fetchSnapshotSizes fetches restore sizes concurrently, skipping snapshots that already have summary data.
func fetchSnapshotSizes(snapshots []resticSnapshot, resticRepo, resticKey string) []uint64 {
	sizes := make([]uint64, len(snapshots))

	// Pre-populate from summary and collect indices that need fetching
	var missing []int
	for i, snap := range snapshots {
		if snap.Summary != nil {
			sizes[i] = snap.Summary.TotalBytesProcessed
		} else {
			missing = append(missing, i)
		}
	}

	if len(missing) == 0 {
		return sizes
	}

	var completed int64
	total := int64(len(missing))
	fmt.Fprintf(os.Stderr, "Fetching sizes for %d snapshots without summary data...\n", total)

	var wg sync.WaitGroup
	work := make(chan int, len(missing))

	// Start 10 workers
	workers := 10
	if int(total) < workers {
		workers = int(total)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				statsCmd := exec.Command("restic", "stats", snapshots[i].ID, "--mode", "restore-size", "--repo", resticRepo, "--password-file="+resticKey, "--json")
				statsOutput, statsErr := statsCmd.Output()
				if statsErr == nil {
					var stats resticStats
					if json.Unmarshal(statsOutput, &stats) == nil {
						sizes[i] = stats.TotalSize
					}
				}
				done := atomic.AddInt64(&completed, 1)
				fmt.Fprintf(os.Stderr, "\rFetching sizes... %d/%d", done, total)
			}
		}()
	}

	for _, i := range missing {
		work <- i
	}
	close(work)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "\r\033[K")
	return sizes
}

// backupSnapshotsNative implements `captaincore backup snapshots <site>` natively in Go.
func backupSnapshotsNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

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

	resticCmd := exec.Command("restic", "snapshots", "--repo", resticRepo, "--password-file="+resticKey, "--json")
	output, err := resticCmd.Output()
	if err != nil {
		fmt.Println("Error: Backup repo not found.")
		return
	}

	var snapshots []resticSnapshot
	if json.Unmarshal(output, &snapshots) != nil {
		fmt.Println("Error: Failed to parse snapshot data.")
		return
	}

	// Filter to a specific snapshot if ID provided
	if len(args) > 1 {
		snapshotID := args[1]
		var filtered []resticSnapshot
		for _, snap := range snapshots {
			if snap.ID == snapshotID || snap.ShortID == snapshotID {
				filtered = append(filtered, snap)
				break
			}
		}
		if len(filtered) == 0 {
			fmt.Printf("Error: Snapshot '%s' not found in repo.\n", snapshotID)
			return
		}
		snapshots = filtered
		flagSizes = true
	}

	// Fetch sizes concurrently if requested
	var snapshotSizes []uint64
	if flagSizes {
		snapshotSizes = fetchSnapshotSizes(snapshots, resticRepo, resticKey)
	}

	if flagFormat == "json" {
		if flagSizes {
			type snapshotWithSize struct {
				ID       string `json:"id"`
				ShortID  string `json:"short_id"`
				Time     string `json:"time"`
				Hostname string `json:"hostname"`
				Size     uint64 `json:"size,omitempty"`
			}
			results := make([]snapshotWithSize, len(snapshots))
			for i, snap := range snapshots {
				results[i] = snapshotWithSize{
					ID:       snap.ID,
					ShortID:  snap.ShortID,
					Time:     snap.Time,
					Hostname: snap.Hostname,
					Size:     snapshotSizes[i],
				}
			}
			resultJSON, _ := json.MarshalIndent(results, "", "    ")
			fmt.Println(string(resultJSON))
		} else {
			resultJSON, _ := json.MarshalIndent(snapshots, "", "    ")
			fmt.Println(string(resultJSON))
		}
		return
	}

	// Table output
	if flagSizes {
		fmt.Printf("%-12s %-22s %-16s %s\n", "ID", "Date", "Hostname", "Size")
		for i, snap := range snapshots {
			t, _ := time.Parse(time.RFC3339Nano, snap.Time)
			dateStr := t.Format("2006-01-02 15:04:05")
			fmt.Printf("%-12s %-22s %-16s %s\n", snap.ShortID, dateStr, snap.Hostname, formatBytes(strconv.FormatUint(snapshotSizes[i], 10)))
		}
	} else {
		fmt.Printf("%-12s %-22s %-16s %s\n", "ID", "Date", "Hostname", "Size")
		for _, snap := range snapshots {
			t, _ := time.Parse(time.RFC3339Nano, snap.Time)
			dateStr := t.Format("2006-01-02 15:04:05")
			sizeStr := "--"
			if snap.Summary != nil {
				sizeStr = formatBytes(strconv.FormatUint(snap.Summary.TotalBytesProcessed, 10))
			}
			fmt.Printf("%-12s %-22s %-16s %s\n", snap.ShortID, dateStr, snap.Hostname, sizeStr)
		}
	}

	fmt.Printf("\n%d snapshots\n", len(snapshots))
}

var backupRepoInfoCmd = &cobra.Command{
	Use:   "repo-info <site>",
	Short: "Shows summary information about a site's backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		backupRepoInfoNative(cmd, args)
	},
}

// backupRepoInfoNative implements `captaincore backup repo-info <site>` natively in Go.
func backupRepoInfoNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

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

	// Fetch snapshots
	snapshotsCmd := exec.Command("restic", "snapshots", "--repo", resticRepo, "--password-file="+resticKey, "--json")
	snapshotsOutput, err := snapshotsCmd.Output()
	if err != nil {
		fmt.Println("Error: Backup repo not found.")
		return
	}

	var snapshots []resticSnapshot
	if json.Unmarshal(snapshotsOutput, &snapshots) != nil {
		fmt.Println("Error: Failed to parse snapshot data.")
		return
	}

	// Find oldest and newest snapshot times
	var oldest, newest time.Time
	for i, snap := range snapshots {
		t, _ := time.Parse(time.RFC3339Nano, snap.Time)
		if i == 0 || t.Before(oldest) {
			oldest = t
		}
		if i == 0 || t.After(newest) {
			newest = t
		}
	}

	// Get latest snapshot size from embedded summary (fast, no extra restic call)
	var latestSize uint64
	var hasLatestSize bool
	if len(snapshots) > 0 {
		latestIdx := 0
		for i, snap := range snapshots {
			t, _ := time.Parse(time.RFC3339Nano, snap.Time)
			tn, _ := time.Parse(time.RFC3339Nano, snapshots[latestIdx].Time)
			if t.After(tn) {
				latestIdx = i
			}
		}
		if snapshots[latestIdx].Summary != nil {
			latestSize = snapshots[latestIdx].Summary.TotalBytesProcessed
			hasLatestSize = true
		}
	}

	// Get repo size on disk via rclone size (fast)
	rclonePath := fmt.Sprintf("%s/%s/%s/restic-repo", rcloneBackup, siteDir, envName)
	type rcloneSize struct {
		Count int64  `json:"count"`
		Bytes uint64 `json:"bytes"`
	}
	var repoSize rcloneSize
	rcloneSizeCmd := exec.Command("rclone", "size", "--json", rclonePath)
	if rcloneSizeOutput, err := rcloneSizeCmd.Output(); err == nil {
		json.Unmarshal(rcloneSizeOutput, &repoSize)
	}

	// Optionally fetch full repo stats (slow)
	var restoreStats, rawStats resticStats
	flagStats, _ := cmd.Flags().GetBool("stats")
	if flagStats {
		restoreSizeCmd := exec.Command("restic", "stats", "--mode", "restore-size", "--repo", resticRepo, "--password-file="+resticKey, "--json")
		if restoreOutput, err := restoreSizeCmd.Output(); err == nil {
			json.Unmarshal(restoreOutput, &restoreStats)
		}
		rawDataCmd := exec.Command("restic", "stats", "--mode", "raw-data", "--repo", resticRepo, "--password-file="+resticKey, "--json")
		if rawOutput, err := rawDataCmd.Output(); err == nil {
			json.Unmarshal(rawOutput, &rawStats)
		}
	}

	if flagFormat == "json" {
		type repoInfo struct {
			Repository     string `json:"repository"`
			Snapshots      int    `json:"snapshots"`
			LatestSize     uint64 `json:"latest_size,omitempty"`
			RepoSize       uint64 `json:"repo_size"`
			RepoFileCount  int64  `json:"repo_file_count"`
			TotalSize      uint64 `json:"total_size,omitempty"`
			TotalFileCount uint64 `json:"total_file_count,omitempty"`
			RawSize        uint64 `json:"raw_size,omitempty"`
			RawFileCount   uint64 `json:"raw_file_count,omitempty"`
			Oldest         string `json:"oldest,omitempty"`
			Newest         string `json:"newest,omitempty"`
		}
		info := repoInfo{
			Repository:    resticRepo,
			Snapshots:     len(snapshots),
			RepoSize:      repoSize.Bytes,
			RepoFileCount: repoSize.Count,
		}
		if hasLatestSize {
			info.LatestSize = latestSize
		}
		if flagStats {
			info.TotalSize = restoreStats.TotalSize
			info.TotalFileCount = restoreStats.TotalFileCount
			info.RawSize = rawStats.TotalSize
			info.RawFileCount = rawStats.TotalFileCount
		}
		if len(snapshots) > 0 {
			info.Oldest = oldest.Format(time.RFC3339)
			info.Newest = newest.Format(time.RFC3339)
		}
		resultJSON, _ := json.MarshalIndent(info, "", "    ")
		fmt.Println(string(resultJSON))
		return
	}

	siteLabel := fmt.Sprintf("%s-%s", site.Site, envName)
	fmt.Printf("Repository info for %s\n", siteLabel)
	fmt.Printf("  Repository:  %s\n", resticRepo)
	fmt.Printf("  Snapshots:   %d\n", len(snapshots))
	if hasLatestSize {
		fmt.Printf("  Latest Size: %s\n", formatBytes(strconv.FormatUint(latestSize, 10)))
	}
	fmt.Printf("  Repo Size:   %s (%d objects)\n", formatBytes(strconv.FormatUint(repoSize.Bytes, 10)), repoSize.Count)
	if flagStats {
		fmt.Printf("  Total Size:  %s (%d files, restore size)\n", formatBytes(strconv.FormatUint(restoreStats.TotalSize, 10)), restoreStats.TotalFileCount)
		fmt.Printf("  Raw Data:    %s (%d files, deduplicated)\n", formatBytes(strconv.FormatUint(rawStats.TotalSize, 10)), rawStats.TotalFileCount)
	}
	if len(snapshots) > 0 {
		fmt.Printf("  Oldest:      %s\n", oldest.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Newest:      %s\n", newest.Format("2006-01-02 15:04:05"))
	}
}

var backupForgetCmd = &cobra.Command{
	Use:   "forget <site> <snapshot-id>",
	Short: "Removes a specific snapshot from the backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <site> and <snapshot-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		backupForgetNative(cmd, args)
	},
}

// backupForgetNative implements `captaincore backup forget <site> <snapshot-id>` natively in Go.
func backupForgetNative(cmd *cobra.Command, args []string) {
	if !ensureDB() || !dbHasData() {
		fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
		return
	}

	sa := parseSiteArgument(args[0])
	snapshotID := args[1]

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

	// Look up the snapshot to confirm it exists
	resticCmd := exec.Command("restic", "snapshots", "--repo", resticRepo, "--password-file="+resticKey, "--json")
	output, err := resticCmd.Output()
	if err != nil {
		fmt.Println("Error: Backup repo not found.")
		return
	}

	var snapshots []resticSnapshot
	if json.Unmarshal(output, &snapshots) != nil {
		fmt.Println("Error: Failed to parse snapshot data.")
		return
	}

	// Find matching snapshot
	var matched *resticSnapshot
	for _, snap := range snapshots {
		if snap.ID == snapshotID || snap.ShortID == snapshotID {
			matched = &snap
			break
		}
	}

	if matched == nil {
		fmt.Printf("Error: Snapshot '%s' not found in repo.\n", snapshotID)
		return
	}

	t, _ := time.Parse(time.RFC3339Nano, matched.Time)
	dateStr := t.Format("2006-01-02 15:04:05")

	if !flagConfirm {
		fmt.Printf("Snapshot to forget:\n")
		fmt.Printf("  ID:       %s (%s)\n", matched.ShortID, matched.ID)
		fmt.Printf("  Date:     %s\n", dateStr)
		fmt.Printf("  Hostname: %s\n", matched.Hostname)
		fmt.Printf("\nRun with --confirm to delete this snapshot.\n")
		return
	}

	fmt.Printf("Forgetting snapshot %s (%s) from %s-%s\n", matched.ShortID, dateStr, site.Site, envName)

	forgetArgs := []string{
		"forget", matched.ID,
		"--repo", resticRepo,
		"--password-file=" + resticKey,
		"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
	}

	forgetCmd := exec.Command("restic", forgetArgs...)
	forgetCmd.Stdout = os.Stdout
	forgetCmd.Stderr = os.Stderr
	if err := forgetCmd.Run(); err != nil {
		fmt.Printf("Error: restic forget failed: %v\n", err)
		return
	}

	fmt.Println("Snapshot forgotten successfully.")

	if flagPrune {
		fmt.Printf("\nPruning backup repo for %s-%s\n", site.Site, envName)
		pruneArgs := []string{
			"prune",
			"--repo", resticRepo,
			"--password-file=" + resticKey,
			"-o", "rclone.args=serve restic --stdio --b2-hard-delete --timeout=300s --contimeout=60s",
		}
		pruneCmd := exec.Command("restic", pruneArgs...)
		pruneCmd.Stdout = os.Stdout
		pruneCmd.Stderr = os.Stderr
		pruneCmd.Run()
	}

	// Regenerate snapshot list
	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, envName)
	fmt.Printf("\nRegenerating snapshot list for %s\n", siteEnvArg)
	listGenCmd := exec.Command("captaincore", "backup", "list-generate", siteEnvArg, "--captain-id="+captainID)
	listGenCmd.Stdout = os.Stdout
	listGenCmd.Stderr = os.Stderr
	listGenCmd.Run()
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
	backupCmd.AddCommand(backupSnapshotsCmd)
	backupCmd.AddCommand(backupForgetCmd)
	backupCmd.AddCommand(backupStorageCleanupCmd)
	backupCmd.AddCommand(backupUnlockCmd)
	backupCmd.AddCommand(backupRepoInfoCmd)
	backupRepoInfoCmd.Flags().Bool("stats", false, "Include full repo stats (slow, runs restic stats)")
	backupSnapshotsCmd.Flags().BoolVar(&flagSizes, "sizes", false, "Fetch per-snapshot restore size (slow)")
	backupSnapshotsCmd.Flags().StringVar(&flagFormat, "format", "", "Output format (json)")
	backupForgetCmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Actually delete the snapshot (default is preview)")
	backupForgetCmd.Flags().BoolVar(&flagPrune, "prune", false, "Run restic prune after forget to reclaim space")
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
