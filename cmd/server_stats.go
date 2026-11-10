package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

// `captaincore node stats <server-id> [--range=1h|24h|7d]` returns a server's
// live metrics, system facts, containers and history from the node tool
// (cc-backups stats), installing or updating the tool and its every-minute
// metrics timer first. The Manager's Servers → server page reads this.

var flagNodeRange = "1h"

func defaultKeyPath() (string, error) {
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		return "", fmt.Errorf("configuration file not found")
	}
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	key := ""
	if raw, _ := models.GetConfiguration(uint(cid), "configurations"); raw != "" {
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &obj) == nil {
			if v, ok := obj["default_key"]; ok {
				key = configValueString(v)
			}
		}
	}
	if key == "" || !isSafeShellToken(key) {
		return "", fmt.Errorf("no default SSH key configured")
	}
	return fmt.Sprintf("%s/%s/%s", system.PathKeys, captainID, key), nil
}

var flagNodeLines = "200"

// nodeTarget resolves a server id to an ssh target with the tool installed.
func nodeTarget(id string) *hostTargetInfo {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	raw, err := newAPIClient(system, captain).Post("server-get", map[string]interface{}{"server_id": id})
	if err != nil {
		fail("manager: %v", err)
	}
	var sv struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		SSHPort int    `json:"ssh_port"`
		SSHUser string `json:"ssh_user"`
	}
	if json.Unmarshal(raw, &sv) != nil || sv.Address == "" {
		fail("server %s not found: %s", id, strings.TrimSpace(string(raw)))
	}
	keyPath, err := defaultKeyPath()
	if err != nil {
		fail("%v", err)
	}
	if sv.SSHPort == 0 {
		sv.SSHPort = 22
	}
	if sv.SSHUser == "" {
		sv.SSHUser = "root"
	}
	t := &hostTargetInfo{server: sv.Name, sshArgs: []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=30",
		"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath,
		"-p", fmt.Sprint(sv.SSHPort), sv.SSHUser + "@" + sv.Address}}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	return t
}

func nodeLogs(args []string) {
	t := nodeTarget(args[0])
	res, err := ccTool(t, "logs", args[1], flagNodeLines)
	if err != nil {
		fail("logs: %v", err)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
}

var reContainerName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// nodeContainer prints one container's page data (cc-backups container).
func nodeContainer(args []string) {
	if !reContainerName.MatchString(args[1]) {
		fail("invalid container name")
	}
	if flagNodeRange != "1h" && flagNodeRange != "24h" && flagNodeRange != "7d" {
		fail("--range must be 1h, 24h or 7d.")
	}
	t := nodeTarget(args[0])
	res, err := ccTool(t, "container", args[1], flagNodeRange)
	if err != nil {
		fail("container: %v", err)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
}

// nodeRestart restarts one program in a container, or the container.
func nodeRestart(args []string) {
	if !reContainerName.MatchString(args[1]) {
		fail("invalid container name")
	}
	if !regexp.MustCompile(`^(php|database|cron|container|app-[a-z][a-z0-9-]{0,19})$`).MatchString(args[2]) {
		fail("restart php, database, cron, app-<name> or container")
	}
	t := nodeTarget(args[0])
	res, err := ccTool(t, "restart", args[1], args[2])
	if err != nil {
		fail("restart: %v", err)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
}

func nodeStats(args []string) {
	if flagNodeRange != "1h" && flagNodeRange != "24h" && flagNodeRange != "7d" {
		fail("--range must be 1h, 24h or 7d.")
	}
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	raw, err := newAPIClient(system, captain).Post("server-get", map[string]interface{}{"server_id": args[0]})
	if err != nil {
		fail("manager: %v", err)
	}
	var sv struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		SSHPort int    `json:"ssh_port"`
		SSHUser string `json:"ssh_user"`
	}
	if json.Unmarshal(raw, &sv) != nil || sv.Address == "" {
		fail("server %s not found: %s", args[0], strings.TrimSpace(string(raw)))
	}
	keyPath, err := defaultKeyPath()
	if err != nil {
		fail("%v", err)
	}
	if sv.SSHPort == 0 {
		sv.SSHPort = 22
	}
	if sv.SSHUser == "" {
		sv.SSHUser = "root"
	}
	t := &hostTargetInfo{server: sv.Name, sshArgs: []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=30",
		"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath,
		"-p", fmt.Sprint(sv.SSHPort), sv.SSHUser + "@" + sv.Address}}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	res, err := ccTool(t, "stats", flagNodeRange)
	if err != nil {
		fail("stats: %v", err)
	}
	b, _ := json.Marshal(res)
	fmt.Println(string(b))
	_ = os.Stdout.Sync()
}

func init() {
	nodeCmd := &cobra.Command{Use: "node", Short: "Servers CaptainCore runs itself"}
	stats := &cobra.Command{
		Use: "stats <server-id>", Short: "Live metrics, containers and history for a server", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeStats(a) })
		},
	}
	stats.Flags().StringVar(&flagNodeRange, "range", "1h", "1h, 24h or 7d")
	nodeCmd.AddCommand(stats)
	logsCmd := &cobra.Command{
		Use: "logs <server-id> <source>", Short: "Recent lines from a server log (edge, ssh, backups, kernel, incus, updates, ct:<container>:<php|database|ssh|cron>)", Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeLogs(a) })
		},
	}
	logsCmd.Flags().StringVar(&flagNodeLines, "lines", "200", "How many lines (10-2000)")
	nodeCmd.AddCommand(logsCmd)
	containerCmd := &cobra.Command{
		Use: "container <server-id> <container>", Short: "Programs, traffic, web server rules and metrics for one container", Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeContainer(a) })
		},
	}
	containerCmd.Flags().StringVar(&flagNodeRange, "range", "1h", "1h, 24h or 7d")
	nodeCmd.AddCommand(containerCmd)
	nodeCmd.AddCommand(&cobra.Command{
		Use: "restart <server-id> <container> <php|database|cron|app-<name>|container>", Short: "Restart a program in a container, or the container", Args: cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeRestart(a) })
		},
	})
	registerNodeImage(nodeCmd)
	rootCmd.AddCommand(nodeCmd)
}
