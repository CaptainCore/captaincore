package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
	rootCmd.AddCommand(nodeCmd)
}
