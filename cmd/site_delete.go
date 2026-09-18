package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagDeleteKeepFiles, flagDeleteSkipSnapshot bool

// siteSlugPattern is the character set a site slug may use. Anything else
// (a slash, a dot, whitespace, an underscore) never reaches a filesystem or
// rclone path: the delete refuses instead.
var siteSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// rcloneRemotePattern matches the `remote:` prefix of an rclone path.
var rcloneRemotePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// errSiteFolderMissing means the site never had (or no longer has) a folder
// under the data path. The delete carries on; there is nothing to remove.
var errSiteFolderMissing = errors.New("site folder not present")

// siteFolderName is the on-disk (and on-B2) folder for a site: {slug}_{id}.
func siteFolderName(site *models.Site) (string, error) {
	if site == nil || site.SiteID == 0 {
		return "", errors.New("site has no id")
	}
	if !siteSlugPattern.MatchString(site.Site) {
		return "", fmt.Errorf("site slug %q contains characters outside [A-Za-z0-9-]", site.Site)
	}
	name := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	id, ok := parseSiteFolderName(name)
	if !ok || id != site.SiteID {
		return "", fmt.Errorf("folder name %q does not round-trip to site_id %d", name, site.SiteID)
	}
	return name, nil
}

// siteFolderForDelete resolves the one directory `site delete` may remove
// locally and proves it is a direct child of dataPath before returning it.
// Every check is against the resolved path, so a symlink under the data path
// pointing elsewhere is refused rather than followed.
func siteFolderForDelete(dataPath string, site *models.Site) (string, error) {
	name, err := siteFolderName(site)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(dataPath) == "" {
		return "", errors.New("system.path is empty in config")
	}
	root, err := filepath.Abs(dataPath)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) || filepath.Dir(root) == root {
		return "", fmt.Errorf("refusing to operate on filesystem root %q", root)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("data path %s: %w", root, err)
	}
	if info, err := os.Stat(resolvedRoot); err != nil || !info.IsDir() {
		return "", fmt.Errorf("data path %s is not a directory", root)
	}

	folder := filepath.Join(root, name)
	if filepath.Base(folder) != name || filepath.Dir(folder) != root {
		return "", fmt.Errorf("folder %q is not a direct child of %s", folder, root)
	}

	info, err := os.Lstat(folder)
	if err != nil {
		if os.IsNotExist(err) {
			return folder, errSiteFolderMissing
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to delete %s: it is a symlink", folder)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("refusing to delete %s: not a directory", folder)
	}
	if !pathUnderRoot(folder, root) {
		return "", fmt.Errorf("refusing to delete %s: resolves outside %s", folder, root)
	}
	resolvedFolder, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return "", err
	}
	if filepath.Dir(resolvedFolder) != resolvedRoot || filepath.Base(resolvedFolder) != name {
		return "", fmt.Errorf("refusing to delete %s: resolves to %s, not a child of %s", folder, resolvedFolder, resolvedRoot)
	}
	return folder, nil
}

// siteRemoteForDelete builds the one rclone path `site delete` may purge and
// returns it with its parent (the configured backup root). The path is
// remote:bucket[/prefix...]/{slug}_{id}; a bare remote, a bucket root, an
// empty segment or a `..` anywhere is refused.
func siteRemoteForDelete(rcloneBackup string, site *models.Site) (parent, target string, err error) {
	name, err := siteFolderName(site)
	if err != nil {
		return "", "", err
	}
	parent = strings.TrimRight(strings.TrimSpace(rcloneBackup), "/")
	if parent == "" {
		return "", "", errors.New("rclone_backup is empty in config")
	}
	if strings.ContainsAny(parent, " \t\r\n") {
		return "", "", fmt.Errorf("rclone_backup %q contains whitespace", parent)
	}
	idx := strings.Index(parent, ":")
	if idx <= 0 {
		return "", "", fmt.Errorf("rclone_backup %q is not remote:path", parent)
	}
	remote, path := parent[:idx], parent[idx+1:]
	if !rcloneRemotePattern.MatchString(remote) {
		return "", "", fmt.Errorf("rclone_backup remote %q is not a plain remote name", remote)
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", fmt.Errorf("rclone_backup %q has no bucket or path; refusing to purge at a remote root", parent)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", "", fmt.Errorf("rclone_backup %q has an empty or relative path segment", parent)
		}
	}
	parent = remote + ":" + path
	target = parent + "/" + name
	return parent, target, nil
}

// remoteFolderExists lists the parent on the remote and reports whether the
// site folder is one of its direct children. A listing failure is an error,
// not a "no", so a bad remote never turns into a skipped purge that looks
// clean.
func remoteFolderExists(parent, name string) (bool, error) {
	out, err := exec.Command("rclone", "lsf", "--dirs-only", parent+"/").Output()
	if err != nil {
		return false, fmt.Errorf("rclone lsf %s: %w", parent, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimRight(strings.TrimSpace(line), "/") == name {
			return true, nil
		}
	}
	return false, nil
}

// finalSnapshotLanded checks that a snapshot newer than sinceID exists for the
// environment and that its archive sits on the snapshot remote with a
// non-zero size. `snapshot add` records the archive before the upload is
// checked, so the record alone is not proof; the remote listing is.
func finalSnapshotLanded(captain *config.CaptainConfig, system *config.SystemConfig, envID, sinceID uint) error {
	snapshot, err := models.LatestSnapshotByEnvironmentID(envID)
	if err != nil || snapshot == nil {
		return errors.New("no snapshot record was created")
	}
	if snapshot.SnapshotID <= sinceID {
		return errors.New("no new snapshot record was created")
	}
	if snapshot.SnapshotName == "" || snapshot.SnapshotName != filepath.Base(snapshot.SnapshotName) {
		return fmt.Errorf("snapshot record %d has an unusable archive name %q", snapshot.SnapshotID, snapshot.SnapshotName)
	}
	remote := ""
	if idx := strings.Index(system.RcloneSnapshot, ":"); idx > 0 {
		remote = system.RcloneSnapshot[:idx]
	}
	b2Snapshots, _ := getB2SnapshotsPath(captain, system)
	if remote == "" || b2Snapshots == "" {
		return errors.New("no snapshot remote configured (system.rclone_snapshot)")
	}
	target := fmt.Sprintf("%s:%s/%s", remote, strings.Trim(b2Snapshots, "/"), snapshot.SnapshotName)
	out, err := exec.Command("rclone", "size", "--json", target).Output()
	if err != nil {
		return fmt.Errorf("rclone size %s: %w", target, err)
	}
	var size struct {
		Count int64 `json:"count"`
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal(out, &size); err != nil {
		return fmt.Errorf("rclone size %s: %w", target, err)
	}
	if size.Count != 1 || size.Bytes <= 0 {
		return fmt.Errorf("archive %s is not on the snapshot remote", target)
	}
	fmt.Printf("Final snapshot %s is on the snapshot remote (%s).\n", snapshot.SnapshotName, formatBytes(strconv.FormatInt(size.Bytes, 10)))
	return nil
}

// siteDeleteNative implements `captaincore site delete <site>`.
//
// Order matters: the final snapshot runs first, so a failure there stops the
// delete before anything is removed; the local folder and the remote backup
// folder go next, each through its own guard; the database rows and the
// Manager notification come last, so a delete that failed part way can be
// re-run and picks up where it stopped.
func siteDeleteNative(cmd *cobra.Command, args []string) {
	siteArg := args[0]
	var site *models.Site
	var err error

	// If numeric, treat as site_id; otherwise parse site argument
	if id, parseErr := strconv.ParseUint(siteArg, 10, 64); parseErr == nil {
		site, err = models.GetSiteByID(uint(id))
	} else {
		sa := parseSiteArgument(siteArg)
		site, err = sa.LookupSite()
	}

	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", siteArg)
		os.Exit(1)
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		os.Exit(1)
	}

	name, err := siteFolderName(site)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if flagDryRun {
		fmt.Printf("Dry run for site %s (site_id %d):\n", site.Site, site.SiteID)
	}

	if !flagDeleteKeepFiles {
		// 1. Final snapshot of every environment that has a backup repo, so a
		//    full-site archive outlives the purge below.
		if !flagDeleteSkipSnapshot {
			type snapshotTarget struct {
				arg   string
				envID uint
			}
			var targets []snapshotTarget
			envs, err := models.FindEnvironmentsBySiteID(site.SiteID)
			if err != nil || len(envs) == 0 {
				fmt.Printf("Error: no environments found for %s, so no final snapshot can be taken. Re-run with --skip-snapshot to delete without one.\n", site.Site)
				os.Exit(1)
			}
			for _, env := range envs {
				switch strings.ToLower(env.Environment) {
				case "production":
					targets = append(targets, snapshotTarget{site.Site, env.EnvironmentID})
				case "staging":
					targets = append(targets, snapshotTarget{site.Site + "-staging", env.EnvironmentID})
				}
			}
			adminEmail := getVarString(captain, "captaincore_admin_email")
			for _, target := range targets {
				snapArgs := []string{"snapshot", "generate", target.arg, "--notes=Final snapshot before site delete", "--captain-id=" + captainID}
				if adminEmail != "" {
					snapArgs = append(snapArgs, "--email="+adminEmail)
				}
				if flagDryRun {
					fmt.Printf("  would run: captaincore %s\n", strings.Join(snapArgs, " "))
					continue
				}
				// The snapshot script exits 0 whatever happened inside it, so
				// its exit status proves nothing. Remember the newest snapshot
				// record before the run and afterwards require a newer one
				// whose archive is really on the snapshot remote.
				var lastID uint
				if prev, err := models.LatestSnapshotByEnvironmentID(target.envID); err == nil && prev != nil {
					lastID = prev.SnapshotID
				}
				fmt.Printf("Generating final snapshot for %s...\n", target.arg)
				snap := exec.Command("captaincore", snapArgs...)
				snap.Stdout = os.Stdout
				snap.Stderr = os.Stderr
				snap.Run()
				if err := finalSnapshotLanded(captain, system, target.envID, lastID); err != nil {
					fmt.Printf("Error: final snapshot for %s did not land (%v). Nothing was deleted. Re-run with --skip-snapshot to delete without one.\n", target.arg, err)
					os.Exit(1)
				}
			}
		}

		// 2. Local folder, guarded to a direct child of the data path.
		folder, err := siteFolderForDelete(system.Path, site)
		switch {
		case errors.Is(err, errSiteFolderMissing):
			fmt.Printf("Local folder %s not present, nothing to remove.\n", folder)
		case err != nil:
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		case flagDryRun:
			fmt.Printf("  would remove local folder %s\n", folder)
		default:
			fmt.Printf("Removing local folder %s...\n", folder)
			if err := os.RemoveAll(folder); err != nil {
				fmt.Printf("Error: removing %s: %v\n", folder, err)
				os.Exit(1)
			}
		}

		// 3. Remote backup folder, guarded to a direct child of rclone_backup
		//    and only when the remote listing actually shows it.
		parent, target, err := siteRemoteForDelete(getRcloneBackup(captain, system), site)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		exists, err := remoteFolderExists(parent, name)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		switch {
		case !exists:
			fmt.Printf("Remote folder %s not present, nothing to purge.\n", target)
		case flagDryRun:
			fmt.Printf("  would purge remote folder %s (listed under %s/)\n", target, parent)
		default:
			fmt.Printf("Purging remote folder %s...\n", target)
			purge := exec.Command("rclone", "purge", "--fast-list", target)
			purge.Stdout = os.Stdout
			purge.Stderr = os.Stderr
			if err := purge.Run(); err != nil {
				fmt.Printf("Error: rclone purge %s: %v\n", target, err)
				os.Exit(1)
			}
		}
	}

	if flagDryRun {
		fmt.Printf("  would delete site %d from the local database and notify the Manager\n", site.SiteID)
		return
	}

	// 4. Delete from local database
	if err := models.DeleteSiteByID(site.SiteID); err != nil {
		fmt.Printf("Error: removing site %d from the local database: %v\n", site.SiteID, err)
		os.Exit(1)
	}

	// 5. Post to CaptainCore API
	client := newAPIClient(system, captain)
	resp, err := client.Post("site-delete", map[string]interface{}{
		"site_id": site.SiteID,
	})
	if err == nil {
		fmt.Print(string(resp))
	}
}
