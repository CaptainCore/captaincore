package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// node health: ask each of our servers what needs someone right now
// (`cc-backups health`: the storage pool, the edge, timers, every site
// container's programs, memory kills, full volumes, local backups) and post
// it to the Manager (`hosting-health`), which keeps it as Hosting signals on
// the attention list. A server that cannot be reached is reported as such.
// Meant for cron on the CLI server, every few minutes:
//   captaincore node health --all

var flagNodeHealthAll bool

type nodeServer struct {
	ServerID int    `json:"server_id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	SSHPort  int    `json:"ssh_port"`
	SSHUser  string `json:"ssh_user"`
}

func nodeHealth(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	client := newAPIClient(system, captain)
	var servers []nodeServer
	if flagNodeHealthAll {
		raw, err := client.Post("host-servers", map[string]interface{}{})
		if err != nil || json.Unmarshal(raw, &servers) != nil {
			fail("manager: could not list servers: %v %s", err, strings.TrimSpace(string(raw)))
		}
	} else {
		if len(args) == 0 {
			fail("name a server id, or pass --all")
		}
		for _, id := range args {
			raw, err := client.Post("server-get", map[string]interface{}{"server_id": id})
			var sv nodeServer
			if err != nil || json.Unmarshal(raw, &sv) != nil || sv.Address == "" {
				fail("server %s not found: %s", id, strings.TrimSpace(string(raw)))
			}
			servers = append(servers, sv)
		}
	}
	keyPath, err := defaultKeyPath()
	if err != nil {
		fail("%v", err)
	}
	for _, sv := range servers {
		if sv.SSHPort == 0 {
			sv.SSHPort = 22
		}
		if sv.SSHUser == "" {
			sv.SSHUser = "root"
		}
		t := &hostTargetInfo{server: sv.Name, sshArgs: []string{"-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=20",
			"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath,
			"-p", fmt.Sprint(sv.SSHPort), sv.SSHUser + "@" + sv.Address}}
		payload := map[string]interface{}{"server_id": sv.ServerID}
		var res map[string]interface{}
		if err := t.ensureBackupTool(); err != nil {
			payload["error"] = err.Error()
		} else if res, err = ccTool(t, "health"); err != nil {
			payload["error"] = err.Error()
		} else {
			delete(res, "status")
			payload["report"] = res
		}
		out, err := client.Post("hosting-health", payload)
		if err != nil {
			fmt.Printf("%s: manager: %v\n", sv.Name, err)
			continue
		}
		if e, ok := payload["error"]; ok {
			fmt.Printf("%s: unreachable (%v)\n", sv.Name, e)
			continue
		}
		fmt.Printf("%s: %s\n", sv.Name, strings.TrimSpace(string(out)))
	}
}

func registerNodeHealth(nodeCmd *cobra.Command) {
	c := &cobra.Command{
		Use: "health [<server-id>...] [--all]", Short: "Check our servers and report what needs someone to the Manager (Hosting signals)",
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { nodeHealth(a) })
		},
	}
	c.Flags().BoolVar(&flagNodeHealthAll, "all", false, "every server that is not retired")
	nodeCmd.AddCommand(c)
}
