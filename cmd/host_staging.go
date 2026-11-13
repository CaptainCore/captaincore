package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
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
	return ccToolIn(t, "", args...)
}

// ccToolIn runs a cc-backups command on the node with something on its stdin.
func ccToolIn(t *hostTargetInfo, stdin string, args ...string) (map[string]interface{}, error) {
	remote := "sudo /usr/local/sbin/cc-backups"
	for _, a := range args {
		remote += " " + shellSingleQuote(a)
	}
	stdout, stderr, _ := t.ssh(remote, stdin)
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

// host site create <site>: build the container for a site the Manager just
// created on one of our servers (Servers → Add site…).
func hostSiteCreate(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	client := newAPIClient(system, captain)
	// The Manager passes the site id: a site created a moment ago is not in
	// the CLI's local copy until it is synced.
	if !regexp.MustCompile(`^\d+$`).MatchString(args[0]) {
		fail("pass the site id")
	}
	fmt.Println("Pulling the new site into the CLI…")
	if err := selfRun("site", "sync", args[0]); err != nil {
		fail("site sync: %v", err)
	}
	id, _ := strconv.ParseUint(args[0], 10, 64)
	site, err := models.GetSiteByID(uint(id))
	if err != nil || site == nil {
		fail("site %s not found after sync", args[0])
	}
	args = []string{site.Site}
	raw, err := client.Post("host-site-prepare", map[string]interface{}{"site_id": site.SiteID})
	if err != nil {
		fail("manager: %v", err)
	}
	var info struct {
		Container, IP, Image, Primary, Start, Title string
		SSHPort                                     int    `json:"ssh_port"`
		MemoryMiB                                   int    `json:"memory_mib"`
		DiskGiB                                     int    `json:"disk_gib"`
		AdminUser                                   string `json:"admin_user"`
		AdminEmail                                  string `json:"admin_email"`
	}
	if json.Unmarshal(raw, &info) != nil || info.Container == "" || info.Image == "" || !reHostname.MatchString(info.Primary) {
		fail("manager reply: %s", strings.TrimSpace(string(raw)))
	}
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	if out, _, _ := t.ssh("sudo incus info "+info.Container+" >/dev/null 2>&1 && echo exists", ""); strings.TrimSpace(out) == "exists" {
		fmt.Printf("Container %s already exists; carrying on from it.\n", info.Container)
	} else {
		fmt.Printf("Building container %s on %s (%s, SSH port %d, %d GiB, %d MiB)…\n", info.Container, t.server, info.IP, info.SSHPort, info.DiskGiB, info.MemoryMiB)
		stack := "wordpress"
		if info.Start == "plain" {
			stack = "plain"
		}
		if _, err := ccTool(t, "site-create", info.Container, info.Image, info.IP, fmt.Sprint(info.SSHPort), fmt.Sprint(info.DiskGiB), fmt.Sprint(info.MemoryMiB), stack); err != nil {
			fail("create: %v", err)
		}
	}
	// CaptainCore's own key first, so the CLI can log in as the site user.
	keyPath, err := siteKeyPath(system, site)
	if err != nil {
		fail("%v", err)
	}
	pub, err := exec.Command("ssh-keygen", "-y", "-f", keyPath).Output()
	if err != nil {
		fail("reading management key: %v", err)
	}
	f := strings.Fields(strings.TrimSpace(string(pub)))
	if len(f) < 2 {
		fail("management key has no public half")
	}
	if _, err := ccTool(t, "authorize", info.Container, f[0]+" "+f[1]); err != nil {
		fail("authorize: %v", err)
	}
	// "empty" (a placeholder for a copy) and "plain" (HTML files) get no WordPress.
	wordpress := info.Start != "empty" && info.Start != "plain"
	if out, _, _ := t.ssh("sudo incus exec "+info.Container+" -- test -f /srv/site/public/wp-config.php && echo yes", ""); wordpress && strings.TrimSpace(out) != "yes" {
		fmt.Println("Installing WordPress…")
		if _, err := ccTool(t, "wp-install", info.Container, info.Primary, info.Title, info.AdminUser, info.AdminEmail); err != nil {
			fail("install: %v", err)
		}
	}
	fmt.Println("Deploying SSH keys, then the hosting configuration…")
	if err := selfRun("ssh-keys", "deploy", args[0]); err != nil {
		fail("ssh keys: %v", err)
	}
	if err := selfRun("host", "apply", args[0]); err != nil {
		fail("host apply: %v", err)
	}
	if wordpress {
		fmt.Println("Syncing into CaptainCore…")
		_ = selfRun("sync-data", args[0])
	}
	fmt.Printf("Site ready: https://%s (SSH port %d). The certificate can take a few minutes on a brand-new domain.\n", info.Primary, info.SSHPort)
}

// hostSiteRemove takes a site environment off its server: the node parks the
// container (stopped, renamed <ct>-removed-<date>, unpublished, purged after
// seven days) and the Manager drops the temporary DNS records and marks the
// environment removed.
func hostSiteRemove(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	fmt.Printf("Taking a last local backup of %s…\n", t.container)
	if _, err := ccTool(t, "create", t.container, "manual"); err != nil {
		fmt.Printf("  (no backup: %v)\n", err)
	}
	fmt.Printf("Parking container %s on %s…\n", t.container, t.server)
	res, err := ccTool(t, "site-park", t.container)
	if err != nil {
		fail("site-park: %v", err)
	}
	raw, err := newAPIClient(system, captain).Post("host-site-removed", map[string]interface{}{
		"site_id": t.site.SiteID, "environment": t.env.Environment, "parked": res["parked"]})
	if err != nil {
		fail("manager: %v", err)
	}
	fmt.Printf("%s is off %s. The container is kept as %v for %v days, then purged.\n%s\n", args[0], t.server, res["parked"], res["purge_after_days"], strings.TrimSpace(string(raw)))
}

// hostWordPressInstall turns a plain site into a WordPress site in place
// (container page → Install WordPress): the node tool takes a local backup,
// turns MariaDB on with a fresh database and moves the current files out of
// ~/public, WordPress is installed on the primary domain with the operator
// who created the site as its administrator, the Manager records the site as
// WordPress (page cache on, WordPress-sized PHP and memory), then the hosting
// configuration is applied and the site synced.
func hostWordPressInstall(args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	client := newAPIClient(system, captain)
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	if !strings.EqualFold(t.env.Environment, "Production") {
		fail("WordPress can only be installed on a plain site's production environment")
	}
	raw, err := client.Post("host-site-prepare", map[string]interface{}{"site_id": t.site.SiteID})
	if err != nil {
		fail("manager: %v", err)
	}
	var info struct {
		Container, Primary, Start, Title string
		AdminUser                        string `json:"admin_user"`
		AdminEmail                       string `json:"admin_email"`
	}
	if json.Unmarshal(raw, &info) != nil || info.Container == "" || !reHostname.MatchString(info.Primary) {
		fail("manager reply: %s", strings.TrimSpace(string(raw)))
	}
	if info.Start != "plain" {
		fail("%s is not a plain site", args[0])
	}
	if info.Container != t.container {
		fail("container mismatch: manager says %s, CLI resolved %s", info.Container, t.container)
	}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	fmt.Printf("Turning on MariaDB in %s (a local backup first)…\n", t.container)
	res, err := ccTool(t, "wp-enable", t.container)
	if err != nil {
		fail("wp-enable: %v", err)
	}
	fmt.Printf("  backup %v", res["backup"])
	if m, _ := res["moved_to"].(string); m != "" {
		fmt.Printf("; the files that were in ~/public are now in %s", m)
	}
	fmt.Println()
	fmt.Printf("Installing WordPress at https://%s…\n", info.Primary)
	if _, err := ccTool(t, "wp-install", t.container, info.Primary, info.Title, info.AdminUser, info.AdminEmail); err != nil {
		fail("install: %v", err)
	}
	if out, err := client.Post("host-wordpress-installed", map[string]interface{}{"site_id": t.site.SiteID, "environment": t.env.Environment}); err != nil {
		fail("manager: %v", err)
	} else {
		fmt.Println(strings.TrimSpace(string(out)))
	}
	// The CLI's cached copy of the site still says plain; refresh it first.
	_ = selfRun("site", "sync", fmt.Sprint(t.site.SiteID))
	if err := selfRun("host", "apply", args[0]); err != nil {
		fail("host apply: %v", err)
	}
	fmt.Println("Syncing into CaptainCore…")
	_ = selfRun("sync-data", args[0])
	fmt.Printf("WordPress is installed at https://%s (administrator %s). Use Login to WordPress to sign in.\n", info.Primary, info.AdminUser)
}

func init() {
	siteCmd := &cobra.Command{Use: "site", Short: "Sites on our own servers"}
	siteCmd.AddCommand(&cobra.Command{
		Use: "remove <site>[-env]", Short: "Take a site environment off its server (kept 7 days, then purged)", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostSiteRemove(a) })
		},
	})
	siteCmd.AddCommand(&cobra.Command{
		Use: "create <site-id>", Short: "Build the container for a site created on one of our servers", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostSiteCreate(a) })
		},
	})
	hostCmd.AddCommand(siteCmd)

	wpCmd := &cobra.Command{Use: "wordpress", Short: "WordPress on a site on our own servers"}
	wpCmd.AddCommand(&cobra.Command{
		Use: "install <site>", Short: "Install WordPress on a plain site (MariaDB on, current files moved aside)", Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { hostWordPressInstall(a) })
		},
	})
	hostCmd.AddCommand(wpCmd)
}
