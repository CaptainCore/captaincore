package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var logsArchiveCmd = &cobra.Command{
	Use:   "archive <site|@target>",
	Short: "Archive rotated access/error logs to B2 for long-term retention",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|@target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		isBulk := strings.HasPrefix(target, "@production") ||
			strings.HasPrefix(target, "@staging") ||
			strings.HasPrefix(target, "@all")

		if isBulk {
			cfg := BulkConfig{
				Command:   "logs/archive",
				Targets:   []string{target},
				Flags:     collectLogsArchiveFlags(),
				CaptainID: captainID,
				Parallel:  flagParallel,
				Label:     flagLabel,
				Debug:     flagDebug,
			}
			if err := runBulk(cfg); err != nil {
				os.Exit(1)
			}
			return
		}

		resolveNativeOrWP(cmd, args, logsArchiveSingle)
	},
}

func collectLogsArchiveFlags() []string {
	var flags []string
	if flagDryRun {
		flags = append(flags, "--dry-run")
	}
	if flagSkipIfRecent != "" {
		flags = append(flags, "--skip-if-recent="+flagSkipIfRecent)
	}
	if flagDebug {
		flags = append(flags, "--debug")
	}
	return flags
}

// logsArchiveSingle archives logs for a single site/environment.
func logsArchiveSingle(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Printf("Error: Environment '%s' not found for '%s'.\n", sa.Environment, site.Name)
		return
	}

	if site.Provider != "kinsta" {
		fmt.Printf("Skipping %s-%s (provider=%s, only Kinsta supported)\n",
			site.Site, strings.ToLower(env.Environment), site.Provider)
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	envName := strings.ToLower(env.Environment)
	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	listPath := filepath.Join(system.Path, siteDir, envName, "logs", "list.json")

	if flagSkipIfRecent != "" && checkLastRun(listPath, flagSkipIfRecent) {
		fmt.Printf("Skipping %s-%s (archived recently)\n", site.Site, envName)
		return
	}

	lockPath := filepath.Join(system.Path, siteDir, envName, "logs-archive.lock")
	if !acquireBackupLock(lockPath) {
		fmt.Printf("Skipping %s-%s (another archive is running)\n", site.Site, envName)
		return
	}
	defer releaseBackupLock(lockPath)

	conn, err := newLogsConn(site, env, system)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	scriptPath := filepath.Join(getCaptainCorePath(), "lib", "remote-scripts", "archive-logs")
	if _, err := os.Stat(scriptPath); err != nil {
		fmt.Printf("Error: archive-logs script not found at %s\n", scriptPath)
		return
	}

	output, err := conn.runScript(scriptPath)
	if err != nil {
		fmt.Printf("Error enumerating logs on %s-%s: %v\n", site.Site, envName, err)
		return
	}

	files := parseArchiveLogsOutput(output)
	if len(files) == 0 {
		fmt.Printf("No new logs to archive for %s-%s\n", site.Site, envName)
		updateLogsArchiveList(listPath, nil)
		return
	}

	rcloneBackup := getRcloneBackup(captain, system)
	destBase := fmt.Sprintf("%s/%s/%s/logs", rcloneBackup, siteDir, envName)

	if flagDryRun {
		fmt.Printf("Dry run for %s-%s — would archive %d file(s):\n",
			site.Site, envName, len(files))
		for _, f := range files {
			dest := buildLogDest(destBase, f)
			fmt.Printf("  %s -> %s\n", f.Path, dest)
		}
		return
	}

	uploaded, skipped := 0, 0
	for _, f := range files {
		dest := buildLogDest(destBase, f)

		exists, err := rcloneObjectExists(dest)
		if err != nil {
			fmt.Printf("  ! %s: rclone check failed: %v\n", f.Path, err)
			continue
		}
		if exists {
			skipped++
			continue
		}

		if err := streamLogToB2(conn, f, dest); err != nil {
			fmt.Printf("  ! %s: upload failed: %v\n", f.Path, err)
			continue
		}
		fmt.Printf("  + %s -> %s\n", f.Path, dest)
		uploaded++
	}

	fmt.Printf("%s-%s: %d uploaded, %d already in B2\n",
		site.Site, envName, uploaded, skipped)

	updateLogsArchiveList(listPath, files)
}

// archiveLogFile describes one rotated log file from the remote enumeration.
type archiveLogFile struct {
	Path     string
	Basename string
	Type     string // "access" or "error"
	Gzipped  bool
}

func parseArchiveLogsOutput(output string) []archiveLogFile {
	var files []archiveLogFile
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		files = append(files, archiveLogFile{
			Path:     parts[0],
			Basename: parts[1],
			Type:     parts[2],
			Gzipped:  parts[3] == "1",
		})
	}
	return files
}

// buildLogDest returns the deterministic B2 path for a log file.
// Preserves Kinsta's original basename (which embeds the rotation date+epoch)
// and appends .gz unless already gzipped.
func buildLogDest(destBase string, f archiveLogFile) string {
	name := f.Basename
	if !f.Gzipped {
		name += ".gz"
	}
	return fmt.Sprintf("%s/%s/%s", destBase, f.Type, name)
}

// rcloneObjectExists returns true if the given rclone path resolves to an object.
func rcloneObjectExists(remote string) (bool, error) {
	cmd := exec.Command("rclone", "lsf", "--files-only", remote)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "not found") || strings.Contains(s, "object not found") || strings.Contains(s, "directory not found") {
			return false, nil
		}
		return false, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// streamLogToB2 pipes a remote log file through gzip (if needed) into rclone rcat.
func streamLogToB2(conn *logsConn, f archiveLogFile, dest string) error {
	remoteCmd := fmt.Sprintf("cat %s", shellQuote(f.Path))
	if !f.Gzipped {
		remoteCmd += " | gzip -c"
	}

	sshCmd := conn.buildSSHCommand(remoteCmd)
	pipeline := fmt.Sprintf("set -o pipefail; %s | rclone rcat %s", sshCmd, shellQuote(dest))

	cmd := exec.Command("bash", "-c", pipeline)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// shellQuote single-quotes a string for safe inclusion in a bash command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// updateLogsArchiveList persists a list.json record of the most recent archive run.
func updateLogsArchiveList(listPath string, files []archiveLogFile) {
	if err := os.MkdirAll(filepath.Dir(listPath), 0755); err != nil {
		return
	}
	type record struct {
		Time  string           `json:"time"`
		Files []archiveLogFile `json:"files"`
	}
	rec := []record{{
		Time:  time.Now().UTC().Format(time.RFC3339),
		Files: files,
	}}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(listPath, data, 0644)
}

// getCaptainCorePath returns the root captaincore install path (~/.captaincore).
func getCaptainCorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".captaincore")
}

// logsConn holds SSH connection details for one site/environment.
type logsConn struct {
	beforeSSH   string
	remoteOpts  string
	user        string
	host        string
	port        string
	commandPrep string
}

// newLogsConn builds an SSH connection descriptor for a site, mirroring sshNative.
func newLogsConn(site *models.Site, env *models.Environment, system *config.SystemConfig) (*logsConn, error) {
	if env.Protocol != "sftp" {
		return nil, fmt.Errorf("SSH not supported (protocol is %s)", env.Protocol)
	}

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
		remoteOpts = fmt.Sprintf("%s -oPreferredAuthentications=publickey -i %s/%s/%s",
			remoteOpts, system.PathKeys, captainID, key)
	} else {
		beforeSSH = fmt.Sprintf("sshpass -p '%s'", env.Password)
	}

	conn := &logsConn{
		beforeSSH:  beforeSSH,
		remoteOpts: remoteOpts,
		user:       env.Username,
		host:       env.Address,
		port:       env.Port,
	}

	switch site.Provider {
	case "kinsta":
		conn.commandPrep = ""
	case "wpengine":
		conn.commandPrep = ""
		conn.user = site.Site
		conn.host = site.Site + ".ssh.wpengine.net"
		conn.port = ""
	case "rocketdotnet":
		conn.commandPrep = ""
	default:
		conn.commandPrep = ""
	}

	return conn, nil
}

// buildSSHCommand returns a shell-runnable SSH command string that runs the given remote command.
func (c *logsConn) buildSSHCommand(remoteCmd string) string {
	sshCmd := fmt.Sprintf("%s ssh %s %s@%s", c.beforeSSH, c.remoteOpts, c.user, c.host)
	if c.port != "" {
		sshCmd += " -p " + c.port
	}
	body := remoteCmd
	if c.commandPrep != "" {
		body = c.commandPrep + " " + remoteCmd
	}
	sshCmd += " " + shellQuote(body)
	return strings.TrimSpace(sshCmd)
}

// runScript pipes a local script file to bash on the remote host and returns stdout.
func (c *logsConn) runScript(scriptPath string) (string, error) {
	sshBase := c.buildSSHCommand("bash -s")
	pipeline := fmt.Sprintf("%s < %s", sshBase, shellQuote(scriptPath))

	cmd := exec.Command("bash", "-c", pipeline)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func init() {
	logsCmd.AddCommand(logsArchiveCmd)
	logsArchiveCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "List files that would be archived without uploading")
	logsArchiveCmd.Flags().StringVar(&flagSkipIfRecent, "skip-if-recent", "", "Skip environments archived within the given duration (e.g., 24h)")
	logsArchiveCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 5, "Number of sites to archive at the same time (bulk mode)")
}
