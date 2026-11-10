package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// Staging for sites on our own servers.
//   host staging create <site>   build a Staging environment from production:
//                                Manager record + temporary domain, a cloned
//                                container, edge config, own URLs, noindex.
//   host deploy <site> --to=staging|production --what=both|files|database
//                                copy one environment onto the other through
//                                cc-backups sync (a backup of the target first).

var (
	flagDeployTo   string
	flagDeployWhat = "both"
	reHostname     = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

func selfRun(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "captaincore"
	}
	c := exec.Command(exe, append(args, "--captain-id="+captainID)...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func ccTool(t *hostTargetInfo, args ...string) (map[string]interface{}, error) {
	remote := "sudo /usr/local/sbin/cc-backups"
	for _, a := range args {
		remote += " " + shellSingleQuote(a)
	}
	stdout, stderr, _ := t.ssh(remote, "")
	line := strings.TrimSpace(stdout)
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var res map[string]interface{}
	if json.Unmarshal([]byte(line), &res) != nil {
		return nil, fmt.Errorf("no reply from cc-backups: %s %s", strings.TrimSpace(stdout), strings.TrimSpace(stderr))
	}
	if res["status"] != "ok" {
		return res, fmt.Errorf("%v", res["error"])
	}
	return res, nil
}

func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
	os.Exit(1)
}

func hostStagingCreate(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	prod, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("Preparing staging for %s in the Manager…\n", prod.site.Site)
	raw, err := newAPIClient(system, captain).Post("host-staging-prepare", map[string]interface{}{"site_id": prod.site.SiteID})
	if err != nil {
		fail("manager: %v", err)
	}
	var prep struct {
		Container, IP, Primary string
		SSHPort                int    `json:"ssh_port"`
		ProdContainer          string `json:"production_container"`
		ProdPrimary            string `json:"production_primary"`
	}
	if err := json.Unmarshal(raw, &prep); err != nil || prep.Container == "" || prep.IP == "" || prep.SSHPort == 0 {
		fail("manager reply: %s", strings.TrimSpace(string(raw)))
	}
	if !reHostname.MatchString(prep.Primary) || !reHostname.MatchString(prep.ProdPrimary) {
		fail("invalid domains from the manager")
	}
	fmt.Printf("Staging will be %s at https://%s (container %s, %s, SSH port %d).\n", prep.Container, prep.Primary, prep.Container, prep.IP, prep.SSHPort)
	if err := selfRun("site", "sync", fmt.Sprint(prod.site.SiteID)); err != nil {
		fail("site sync: %v", err)
	}
	if err := prod.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	if out, _, _ := prod.ssh("sudo incus info "+prep.Container+" >/dev/null 2>&1 && echo exists", ""); strings.TrimSpace(out) == "exists" {
		fmt.Printf("Container %s already exists; leaving it as is (use host deploy to refresh it).\n", prep.Container)
	} else {
		fmt.Printf("Cloning production (%s) into %s…\n", prod.container, prep.Container)
		res, err := ccTool(prod, "clone", prod.container, prep.Container, prep.IP, fmt.Sprint(prep.SSHPort))
		if err != nil {
			fail("clone: %v", err)
		}
		fmt.Printf("Container up; it answers %v.\n", res["health"])
		fmt.Printf("Pointing the copy at https://%s and hiding it from search engines…\n", prep.Primary)
		if _, err := ccTool(prod, "rehost", prep.Container, prep.ProdPrimary, prep.Primary, "0"); err != nil {
			fail("rehost: %v", err)
		}
	}
	fmt.Println("Applying staging's hosting configuration (edge, PHP)…")
	if err := selfRun("host", "apply", prod.site.Site+"-staging"); err != nil {
		fail("host apply: %v", err)
	}
	fmt.Println("Deploying team SSH keys to staging…")
	_ = selfRun("ssh-keys", "deploy", prod.site.Site+"-staging")
	fmt.Println("Syncing staging into CaptainCore…")
	_ = selfRun("sync-data", prod.site.Site+"-staging")
	fmt.Printf("Staging is ready: https://%s (SSH port %d). Its certificate may take a few minutes on a brand-new domain.\n", prep.Primary, prep.SSHPort)
}

func hostDeploy(args []string) {
	if flagDeployTo != "staging" && flagDeployTo != "production" {
		fail("--to must be staging or production.")
	}
	if flagDeployWhat != "both" && flagDeployWhat != "files" && flagDeployWhat != "database" {
		fail("--what must be both, files or database.")
	}
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	slug := strings.SplitN(args[0], "-", 2)[0]
	prod, err := hostTarget(system, captain, slug)
	if err != nil {
		fail("%v", err)
	}
	stag, err := hostTarget(system, captain, slug+"-staging")
	if err != nil {
		fail("staging: %v", err)
	}
	if prod.primary == "" || stag.primary == "" || !reHostname.MatchString(prod.primary) || !reHostname.MatchString(stag.primary) {
		fail("both environments need a primary domain in their hosting configuration")
	}
	src, dst := prod, stag
	if flagDeployTo == "production" {
		src, dst = stag, prod
	}
	if err := prod.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	fmt.Printf("Deploying %s of %s (%s) onto %s (%s); a backup of the target is taken first…\n", flagDeployWhat, src.env.Environment, src.primary, dst.env.Environment, dst.primary)
	res, err := ccTool(prod, "sync", src.container, dst.container, flagDeployWhat, src.primary, dst.primary)
	if err != nil {
		fail("deploy: %v", err)
	}
	fmt.Printf("Deployed %v. Backup of %s before the deploy: %v. It answers %v.\n", res["deployed"], strings.ToLower(dst.env.Environment), res["safety_backup"], res["health"])
	target := slug
	if dst == stag {
		target += "-staging"
	}
	fmt.Println("Syncing into CaptainCore…")
	_ = selfRun("sync-data", target)
}

var hostStagingCmd = &cobra.Command{Use: "staging", Short: "Staging environments on our own servers"}

func init() {
	hostCmd.AddCommand(hostStagingCmd)
	hostStagingCmd.AddCommand(&cobra.Command{
		Use: "create <site>", Short: "Create a staging environment from production", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostStagingCreate(a) })
		},
	})
	deploy := &cobra.Command{
		Use: "deploy <site>", Short: "Deploy production to staging or staging to production", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostDeploy(a) })
		},
	}
	deploy.Flags().StringVar(&flagDeployTo, "to", "", "staging or production")
	deploy.Flags().StringVar(&flagDeployWhat, "what", "both", "both, files or database")
	hostCmd.AddCommand(deploy)
}
