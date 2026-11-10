package cmd

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// `captaincore host apply <site>` makes one environment on our own servers
// match its hosting configuration in the Manager (Hosting tab): the edge
// Caddy site block (domains, www, HTTPS, redirect to primary), PHP settings
// and workers inside the container, and the container's memory limit. The
// work happens in one Python script run with sudo on the node; it writes only
// what changed, validates before reloading, checks the site afterwards and
// rolls back what it changed when a check fails. The outcome is recorded on
// the revision in the Manager (`host-config-applied`).

//go:embed host_apply.py
var hostApplyScript string

// The page cache's PHP side, installed in the container by host_apply.py
// (it travels in the same JSON the script reads on stdin).
//
//go:embed host_page_cache.php
var hostPageCachePHP string

var flagHostDryRun bool

var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "Hosting configuration for sites on our own servers",
}

var hostApplyCmd = &cobra.Command{
	Use:   "apply <site> | --all [--server=<id>]",
	Short: "Apply a site's hosting configuration to its server (--all: every site on our servers)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if flagApplyAll {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, _ []string) { hostApplyAll() })
			return
		}
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "Error: name a site, or pass --all.")
			os.Exit(1)
		}
		resolveNativeOrWP(cmd, args, hostApplyNative)
	},
}

func hostApplyNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
		os.Exit(1)
	}
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "Error: site %s not found.\n", args[0])
		os.Exit(1)
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintf(os.Stderr, "Error: environment %s not found.\n", sa.Environment)
		os.Exit(1)
	}

	client := newAPIClient(system, captain)
	raw, err := client.Post("host-config", map[string]interface{}{"site_id": site.SiteID, "environment": env.Environment})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: manager: %v\n", err)
		os.Exit(1)
	}
	var hc struct {
		OwnServer bool   `json:"own_server"`
		Revision  int    `json:"revision"`
		Container string `json:"container"`
		IP        string `json:"ip"`
		Image     string `json:"image"`
		Server    struct {
			Name    string `json:"name"`
			Address string `json:"address"`
			SSHPort int    `json:"ssh_port"`
			SSHUser string `json:"ssh_user"`
		} `json:"server"`
	}
	if err := json.Unmarshal(raw, &hc); err != nil {
		fmt.Fprintf(os.Stderr, "Error: manager reply: %v\n", err)
		os.Exit(1)
	}
	if !hc.OwnServer {
		fmt.Printf("%s is not on one of our servers, nothing to apply.\n", args[0])
		return
	}
	if hc.Container == "" || hc.IP == "" || hc.Server.Address == "" {
		fmt.Fprintln(os.Stderr, "Error: the environment has no container, container IP or server address recorded.")
		os.Exit(1)
	}

	keyPath, err := siteKeyPath(system, site)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	port := hc.Server.SSHPort
	if port == 0 {
		port = 22
	}
	user := hc.Server.SSHUser
	if user == "" {
		user = "root"
	}
	// The PHP version is the container's image. A container on another image
	// is rebuilt on the right one first (same volumes, address and limits);
	// the script below then writes the site's settings into the new one.
	var rebuilt map[string]interface{}
	if hc.Image != "" {
		rebuilt = hostApplyImage(system, captain, client, args[0], hc.Container, hc.Image, hc.Revision, site.SiteID, env.Environment)
	}
	remoteCmd := "sudo python3 -c " + shellSingleQuote(hostApplyScript)
	if flagHostDryRun {
		remoteCmd += " --dry-run"
	}
	c := exec.Command("ssh", "-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=30",
		"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath,
		"-p", fmt.Sprint(port), user+"@"+hc.Server.Address, remoteCmd)
	payload := raw
	var full map[string]interface{}
	if json.Unmarshal(raw, &full) == nil {
		full["page_cache_php"] = hostPageCachePHP
		if withPHP, err := json.Marshal(full); err == nil {
			payload = withPHP
		}
	}
	c.Stdin = bytes.NewReader(payload)
	var out, errBuf bytes.Buffer
	c.Stdout, c.Stderr = &out, &errBuf
	runErr := c.Run()

	// The script prints one JSON object as its last line.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var result map[string]interface{}
	if len(lines) > 0 {
		_ = json.Unmarshal([]byte(lines[len(lines)-1]), &result)
	}
	if result == nil {
		result = map[string]interface{}{"status": "failed", "error": strings.TrimSpace(errBuf.String() + " " + fmt.Sprint(runErr))}
	}
	status, _ := result["status"].(string)
	if rebuilt != nil {
		result["rebuild"] = rebuilt
	}

	if flagHostDryRun {
		pretty, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(pretty))
		return
	}
	if _, err := client.Post("host-config-applied", map[string]interface{}{
		"site_id": site.SiteID, "environment": env.Environment,
		"data": map[string]interface{}{"revision": hc.Revision, "status": status, "result": result},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record the result in the manager: %v\n", err)
	}

	changes, _ := result["changes"].([]interface{})
	names := []string{}
	for _, ch := range changes {
		names = append(names, fmt.Sprint(ch))
	}
	if status != "applied" {
		fmt.Fprintf(os.Stderr, "%s revision %d on %s: FAILED: %v\n", args[0], hc.Revision, hc.Server.Name, result["error"])
		os.Exit(1)
	}
	summary := "no changes"
	if len(names) > 0 {
		summary = "changed " + strings.Join(names, ", ")
	}
	fmt.Printf("%s revision %d applied on %s: %s\n", args[0], hc.Revision, hc.Server.Name, summary)
	if h, ok := result["health"].(map[string]interface{}); ok {
		fmt.Printf("  health: container %v, edge %v\n", h["container"], h["edge"])
	}
	if warns, ok := result["warnings"].([]interface{}); ok {
		for _, w := range warns {
			fmt.Printf("  WARNING: %v\n", w)
		}
	}
	if pc, ok := result["page_cache"].(map[string]interface{}); ok && pc["enabled"] == true {
		// HIT: the home page is answered from its saved copy. Anything else
		// means the page itself opts out (it sets a cookie, say).
		fmt.Printf("  page cache: on, pages kept %v minutes, home page %v\n", pc["ttl"], pc["home"])
	}
}

func init() {
	rootCmd.AddCommand(hostCmd)
	hostCmd.AddCommand(hostApplyCmd)
	hostApplyCmd.Flags().BoolVar(&flagHostDryRun, "dry-run", false, "Show what would change without writing anything")
	hostApplyCmd.Flags().BoolVar(&flagApplyAll, "all", false, "Apply every site on our own servers, one after another")
	hostApplyCmd.Flags().StringVar(&flagApplyServer, "server", "", "With --all: only the sites on this server (id)")
}
