package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
)

// Restore to a new site: any site's long-term backup (its restic repo on B2)
// becomes a new site on one of our servers. The Manager creates the site with
// start "restore" and the source in its details (`host-site-prepare` returns
// it); `host site create` builds the container, then the node's
// `cc-backups site-restore` restores the snapshot straight from B2 (restic on
// the node, credentials passed on stdin), imports the database dump and moves
// the site onto its temporary address. Nothing is written to the CLI server's
// disk on the way.

type hostRestoreSource struct {
	SiteID      uint   `json:"site_id"`
	Environment string `json:"environment"`
	BackupID    string `json:"backup_id"`
	Cron        string `json:"cron"`
}

// A snapshot id, or "latest" (the newest backup of the source environment).
var reResticID = regexp.MustCompile(`^([0-9a-f]{8,64}|latest)$`)

var flagSiteCreateEnv string

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// reEnvName is an environment's name as the CLI uses it: one lowercase word
// (production, staging, development, dev2), the same rule as the Manager's
// Environments::valid_name.
var reEnvName = regexp.MustCompile(`^[a-z][a-z0-9]{0,19}$`)

// restoreRepo is the source environment's restic repo and the password file.
func restoreRepo(system *config.SystemConfig, captain *config.CaptainConfig, src hostRestoreSource) (string, string, *models.Site, error) {
	if !reResticID.MatchString(src.BackupID) {
		return "", "", nil, fmt.Errorf("invalid backup id %q", src.BackupID)
	}
	site, err := models.GetSiteByID(src.SiteID)
	if err != nil || site == nil {
		_ = selfRun("site", "sync", fmt.Sprint(src.SiteID))
		site, err = models.GetSiteByID(src.SiteID)
	}
	if err != nil || site == nil {
		return "", "", nil, fmt.Errorf("source site %d not found", src.SiteID)
	}
	env := strings.ToLower(src.Environment)
	if !reEnvName.MatchString(env) {
		return "", "", nil, fmt.Errorf("invalid source environment %q", src.Environment)
	}
	repo := fmt.Sprintf("rclone:%s/%s_%d/%s/restic-repo", getRcloneBackup(captain, system), site.Site, site.SiteID, env)
	return repo, getResticKeyPath(), site, nil
}

// restoreSnapshotBytes is how much the snapshot holds (restic's summary, else
// the size tag the backup was made with), 0 when unknown.
func restoreSnapshotBytes(repo, key, id string) (float64, string, string, error) {
	out, err := exec.Command("restic", "snapshots", id, "--json", "--repo", repo, "--password-file", key, "--no-lock").Output()
	if err != nil {
		return 0, "", "", fmt.Errorf("reading backup %s: %v", id, err)
	}
	var snaps []struct {
		ID      string   `json:"id"`
		Time    string   `json:"time"`
		Tags    []string `json:"tags"`
		Summary struct {
			Bytes float64 `json:"total_bytes_processed"`
		} `json:"summary"`
	}
	if json.Unmarshal(out, &snaps) != nil || len(snaps) == 0 {
		return 0, "", "", fmt.Errorf("backup %s not found in the repo", id)
	}
	s := snaps[0]
	if s.Summary.Bytes > 0 {
		return s.Summary.Bytes, s.Time, s.ID, nil
	}
	units := map[string]float64{"K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40}
	for _, t := range s.Tags {
		if m := regexp.MustCompile(`^size:([\d.]+)([KMGT])$`).FindStringSubmatch(t); m != nil {
			var n float64
			fmt.Sscanf(m[1], "%g", &n)
			return n * units[m[2]], s.Time, s.ID, nil
		}
	}
	return 0, s.Time, s.ID, nil
}

// restoreDiskGiB sizes the files volume for a restore: the backup plus a
// quarter for growth and 2 GiB for the unpacked dump, never under the default.
func restoreDiskGiB(bytes float64, floor int) int {
	need := int(math.Ceil(bytes*1.25/float64(1<<30))) + 2
	if need < floor {
		need = floor
	}
	if need > 200 {
		need = 200
	}
	return need
}

// restoreB2Creds turns the repo's rclone remote into what restic's own B2
// backend needs, when the remote is B2: "b2:<bucket>:<path>" and the key.
// Read from rclone's config, never printed.
func restoreB2Creds(repo string) (map[string]string, bool) {
	spec := strings.TrimPrefix(repo, "rclone:")
	i := strings.Index(spec, ":")
	if i <= 0 {
		return nil, false
	}
	remote, path := spec[:i], strings.Trim(spec[i+1:], "/")
	out, err := exec.Command("rclone", "config", "dump").Output()
	if err != nil {
		return nil, false
	}
	var cfg map[string]map[string]string
	if json.Unmarshal(out, &cfg) != nil {
		return nil, false
	}
	r, ok := cfg[remote]
	if !ok || r["type"] != "b2" || r["account"] == "" || r["key"] == "" {
		return nil, false
	}
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return nil, false
	}
	return map[string]string{"repo": "b2:" + parts[0] + ":" + parts[1], "b2_id": r["account"], "b2_key": r["key"]}, true
}

// hostRestoreRun puts the backup into the container and returns the node's
// report. On a B2 repo the node runs restic restore itself (parallel, straight
// from B2) with the credentials on stdin; otherwise the CLI pipes
// `restic dump … --archive tar` through ssh (one piece at a time: slow).
func hostRestoreRun(t *hostTargetInfo, repo, key, id, host, cron string, replace bool) (map[string]interface{}, error) {
	remote := "sudo /usr/local/sbin/cc-backups site-restore " + shellSingleQuote(t.container) + " " + shellSingleQuote(host) + " " + shellSingleQuote(cron)
	mode := "tar"
	if _, ok := restoreB2Creds(repo); ok {
		mode = "restic"
	}
	if replace {
		mode += "-replace"
	}
	remote += " " + mode
	var dump *exec.Cmd
	var dumpErr bytes.Buffer
	node := exec.Command("ssh", append(append([]string{}, t.sshArgs...), remote)...)
	if creds, ok := restoreB2Creds(repo); ok {
		pass, err := os.ReadFile(key)
		if err != nil {
			return nil, fmt.Errorf("reading the restic key: %v", err)
		}
		creds["password"] = strings.TrimRight(strings.SplitN(string(pass), "\n", 2)[0], "\r")
		creds["snapshot"] = id
		line, _ := json.Marshal(creds)
		node = exec.Command("ssh", append(append([]string{}, t.sshArgs...), remote)...)
		node.Stdin = bytes.NewReader(append(line, '\n'))
	} else {
		fmt.Println("  (not a B2 repo: streaming through this server, which is slower)")
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		dump = exec.Command("restic", "dump", id, "/", "--archive", "tar", "--repo", repo, "--password-file", key, "--no-lock", "--quiet")
		dump.Stdout, dump.Stderr = pw, &dumpErr
		node.Stdin = pr
		if err := dump.Start(); err != nil {
			pr.Close()
			pw.Close()
			return nil, fmt.Errorf("restic: %v", err)
		}
		// The child holds its own copy; ours would keep the stream open.
		defer pr.Close()
		pw.Close()
	}
	var out, errb bytes.Buffer
	node.Stdout, node.Stderr = &out, &errb
	if err := node.Start(); err != nil {
		if dump != nil {
			_ = dump.Process.Kill()
		}
		return nil, fmt.Errorf("ssh: %v", err)
	}
	var dumpWait error
	if dump != nil {
		dumpWait = dump.Wait()
	}
	_ = node.Wait()
	line := strings.TrimSpace(out.String())
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var res map[string]interface{}
	if json.Unmarshal([]byte(line), &res) != nil {
		return nil, fmt.Errorf("no reply from the node: %s %s", strings.TrimSpace(out.String()), strings.TrimSpace(errb.String()))
	}
	if res["status"] != "ok" {
		msg := fmt.Sprint(res["error"])
		if dumpWait != nil {
			msg += " (restic: " + strings.TrimSpace(dumpErr.String()) + ")"
		}
		return res, fmt.Errorf("%s", msg)
	}
	if dumpWait != nil {
		return res, fmt.Errorf("restic ended with an error, the copy may be incomplete: %s", strings.TrimSpace(dumpErr.String()))
	}
	return res, nil
}

func fmtBytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", b/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB", b/(1<<10))
	}
}

// hostSiteRestore runs inside `host site create` for a site whose start is
// "restore", after its container is built and before keys and apply.
func hostSiteRestore(system *config.SystemConfig, captain *config.CaptainConfig, t *hostTargetInfo, src hostRestoreSource, repo, key, host string) {
	cron := "off"
	if src.Cron == "on" {
		cron = "on"
	}
	fmt.Printf("Restoring backup %s into %s (files, then the database)…\n", shortID(src.BackupID), t.container)
	started := time.Now()
	res, err := hostRestoreRun(t, repo, key, src.BackupID, host, cron, false)
	if err != nil {
		fail("restore: %v", err)
	}
	fmt.Printf("  files %s in %vs, database %s in %vs (%s total)\n", fmtBytes(num(res["files_bytes"])), res["files_seconds"],
		fmtBytes(num(res["database_bytes"])), res["database_seconds"], time.Since(started).Round(time.Second))
	fmt.Printf("  was %v, now %v\n", res["old_home"], res["home"])
	if moved, _ := res["moved_aside"].([]interface{}); len(moved) > 0 {
		fmt.Printf("  moved the old host's drop-ins aside: %v\n", moved)
	}
	fmt.Printf("  the database dump is kept in %v (outside the web root); cron is %s; health %v\n", res["kept_in"], cron, res["health"])
	payload := map[string]interface{}{"site_id": t.site.SiteID, "environment": t.env.Environment, "result": res}
	if out, err := newAPIClient(system, captain).Post("host-site-restored", payload); err != nil {
		fmt.Printf("  (the Manager did not take the result: %v)\n", err)
	} else {
		fmt.Println("  " + strings.TrimSpace(string(out)))
	}
}

func num(v interface{}) float64 {
	f, _ := v.(float64)
	return f
}
