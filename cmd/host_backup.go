package cmd

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

// `captaincore host backup` manages local backups (ZFS snapshots of a site's
// files and database volumes, taken together) on our own servers, through the
// node tool cc-backups (cmd/host_backups.py). The tool is installed or updated
// on the node whenever its hash differs, along with a daily systemd timer, so
// local backups keep running even when CaptainCore is unreachable.

//go:embed host_backups.py
var hostBackupsScript string

const hostBackupsTimer = `[Unit]
Description=CaptainCore local backups (daily)

[Timer]
OnCalendar=*-*-* 08:30:00 UTC
RandomizedDelaySec=20m
Persistent=true

[Install]
WantedBy=timers.target
`

const hostMetricsTimer = `[Unit]
Description=CaptainCore server metrics (every minute)

[Timer]
OnCalendar=*-*-* *:*:00
AccuracySec=5s

[Install]
WantedBy=timers.target
`

const hostMetricsService = `[Unit]
Description=CaptainCore server metrics sample

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/cc-backups sample
`

const hostBackupsService = `[Unit]
Description=CaptainCore local backups (daily)

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/cc-backups daily
`

var (
	flagBackupWhat = "both"
	flagBackupJSON bool
	reBackupName   = regexp.MustCompile(`^cc-\d{8}T\d{6}-(daily|manual|pre-restore|pre-deploy)$`)
)

type hostTargetInfo struct {
	site      *models.Site
	env       *models.Environment
	container string
	ip        string
	primary   string
	server    string
	sshArgs   []string
}

// hostTarget resolves <site>[-env] to its container and the ssh arguments
// for its node (management key, server user and port from the Manager).
func hostTarget(system *config.SystemConfig, captain *config.CaptainConfig, arg string) (*hostTargetInfo, error) {
	sa := parseSiteArgument(arg)
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return nil, fmt.Errorf("site %s not found", arg)
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return nil, fmt.Errorf("environment %s not found", sa.Environment)
	}
	raw, err := newAPIClient(system, captain).Post("host-config", map[string]interface{}{"site_id": site.SiteID, "environment": env.Environment})
	if err != nil {
		return nil, fmt.Errorf("manager: %v", err)
	}
	var hc struct {
		OwnServer bool   `json:"own_server"`
		Container string `json:"container"`
		IP        string `json:"ip"`
		Primary   string `json:"primary"`
		Server    struct {
			Name    string `json:"name"`
			Address string `json:"address"`
			SSHPort int    `json:"ssh_port"`
			SSHUser string `json:"ssh_user"`
		} `json:"server"`
	}
	if err := json.Unmarshal(raw, &hc); err != nil {
		return nil, fmt.Errorf("manager reply: %v", err)
	}
	if !hc.OwnServer || hc.Container == "" || hc.Server.Address == "" {
		return nil, fmt.Errorf("%s is not a container on one of our servers", arg)
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`).MatchString(hc.Container) {
		return nil, fmt.Errorf("invalid container name %q", hc.Container)
	}
	keyPath, err := siteKeyPath(system, site)
	if err != nil {
		return nil, err
	}
	port, user := hc.Server.SSHPort, hc.Server.SSHUser
	if port == 0 {
		port = 22
	}
	if user == "" {
		user = "root"
	}
	return &hostTargetInfo{site: site, env: env, container: hc.Container, ip: hc.IP, primary: hc.Primary, server: hc.Server.Name,
		sshArgs: []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=30",
			"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath,
			"-p", fmt.Sprint(port), user + "@" + hc.Server.Address}}, nil
}

func (t *hostTargetInfo) ssh(remote string, stdin string) (string, string, error) {
	c := exec.Command("ssh", append(append([]string{}, t.sshArgs...), remote)...)
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	return out.String(), errb.String(), err
}

// ensureBackupTool installs or updates cc-backups and its timer on the node.
func (t *hostTargetInfo) ensureBackupTool() error {
	sum := sha256.Sum256([]byte(hostBackupsScript))
	want := hex.EncodeToString(sum[:])
	have, _, _ := t.ssh("sha256sum /usr/local/sbin/cc-backups 2>/dev/null | cut -d' ' -f1", "")
	if strings.TrimSpace(have) == want {
		return nil
	}
	install := "sudo tee /usr/local/sbin/cc-backups >/dev/null && sudo chmod 0755 /usr/local/sbin/cc-backups && " +
		"printf %s " + shellSingleQuote(hostBackupsService) + " | sudo tee /etc/systemd/system/cc-backups-daily.service >/dev/null && " +
		"printf %s " + shellSingleQuote(hostBackupsTimer) + " | sudo tee /etc/systemd/system/cc-backups-daily.timer >/dev/null && " +
		"printf %s " + shellSingleQuote(hostMetricsService) + " | sudo tee /etc/systemd/system/cc-metrics.service >/dev/null && " +
		"printf %s " + shellSingleQuote(hostMetricsTimer) + " | sudo tee /etc/systemd/system/cc-metrics.timer >/dev/null && " +
		"sudo systemctl daemon-reload && sudo systemctl enable --now cc-backups-daily.timer cc-metrics.timer >/dev/null 2>&1"
	if _, errOut, err := t.ssh(install, hostBackupsScript); err != nil {
		return fmt.Errorf("installing cc-backups: %v %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

func runHostBackup(action string, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
		os.Exit(1)
	}
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := t.ensureBackupTool(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	remote := "sudo /usr/local/sbin/cc-backups " + action + " " + t.container
	switch action {
	case "create":
		remote += " manual"
	case "restore", "delete":
		if len(args) < 2 || !reBackupName.MatchString(args[1]) {
			fmt.Fprintln(os.Stderr, "Error: name a backup (cc-<date>-<type>, from `host backup list`).")
			os.Exit(1)
		}
		remote += " " + args[1]
		if action == "restore" {
			if flagBackupWhat != "both" && flagBackupWhat != "files" && flagBackupWhat != "database" {
				fmt.Fprintln(os.Stderr, "Error: --what must be both, files or database.")
				os.Exit(1)
			}
			remote += " " + flagBackupWhat
		}
	}
	stdout, stderr, _ := t.ssh(remote, "")
	line := strings.TrimSpace(stdout)
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var res map[string]interface{}
	if json.Unmarshal([]byte(line), &res) != nil {
		fmt.Fprintf(os.Stderr, "Error: no reply from cc-backups: %s %s\n", strings.TrimSpace(stdout), strings.TrimSpace(stderr))
		os.Exit(1)
	}
	if flagBackupJSON {
		fmt.Println(line)
		if res["status"] != "ok" {
			os.Exit(1)
		}
		return
	}
	if res["status"] != "ok" {
		fmt.Fprintf(os.Stderr, "Error: %v\n", res["error"])
		os.Exit(1)
	}
	switch action {
	case "list":
		list, _ := res["backups"].([]interface{})
		fmt.Printf("%d local backup(s) of %s on %s\n", len(list), t.site.Site, t.server)
		for _, b := range list {
			m := b.(map[string]interface{})
			fmt.Printf("  %-34s %-12v %7.1f MB unique  %v\n", m["name"], m["type"], toFloat(m["used_bytes"])/1e6, m["created"])
		}
	case "create":
		fmt.Printf("Local backup %v created for %s on %s\n", res["backup"], t.site.Site, t.server)
	case "restore":
		fmt.Printf("Restored %v of %s from %v (safety backup %v taken first). Site answers %v.\n", res["restored"], t.site.Site, res["from"], res["safety_backup"], res["health"])
	case "delete":
		fmt.Printf("Deleted local backup %v of %s\n", res["deleted"], t.site.Site)
	}
}

func toFloat(v interface{}) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

var hostBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Local backups (ZFS snapshots) of sites on our own servers",
}

func hostBackupSub(use, short, action string, nargs int) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(nargs),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { runHostBackup(action, a) })
		},
	}
}

func init() {
	hostCmd.AddCommand(hostBackupCmd)
	list := hostBackupSub("list <site>", "List local backups", "list", 1)
	create := hostBackupSub("create <site>", "Take a local backup now", "create", 1)
	restore := hostBackupSub("restore <site> <backup>", "Restore files, database or both from a local backup", "restore", 2)
	del := hostBackupSub("delete <site> <backup>", "Delete a local backup", "delete", 2)
	restore.Flags().StringVar(&flagBackupWhat, "what", "both", "both, files or database")
	for _, c := range []*cobra.Command{list, create, restore, del} {
		c.Flags().BoolVar(&flagBackupJSON, "json", false, "Print the raw JSON reply")
		hostBackupCmd.AddCommand(c)
	}
}
