package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

// `captaincore ssh-keys deploy` writes the operator SSH keys the Manager
// holds (Servers → Team SSH keys) into a managed block of
// ~/.ssh/authorized_keys on sites that run on our own servers. Lines outside
// the block are kept; the block itself is replaced whole, so a key removed in
// the Manager disappears on the next deploy. The CLI always adds the public
// half of the key it connects with, so a deploy cannot lock CaptainCore out.

const (
	sshKeysBegin = "# BEGIN CaptainCore managed keys (do not edit)"
	sshKeysEnd   = "# END CaptainCore managed keys"
)

var (
	flagSSHKeysAll    bool
	flagSSHKeysDryRun bool
	reAuthorizedKey   = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com) [A-Za-z0-9+/]+={0,3}( [A-Za-z0-9._@:-]{1,120})?$`)
)

var sshKeysCmd = &cobra.Command{
	Use:   "ssh-keys",
	Short: "Operator SSH keys on sites on our own servers",
}

var sshKeysDeployCmd = &cobra.Command{
	Use:   "deploy [<site>...]",
	Short: "Write the Manager's operator SSH keys into authorized_keys",
	Long: `Writes the operator SSH keys from the Manager into a managed block of
~/.ssh/authorized_keys on each site, keeping any lines outside the block.
Only sites whose environment runs on one of our own servers are touched.

  captaincore ssh-keys deploy austingindercc
  captaincore ssh-keys deploy austingindercc-staging
  captaincore ssh-keys deploy --all`,
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, sshKeysDeployNative)
	},
}

func sshKeysDeployNative(cmd *cobra.Command, args []string) {
	{
		if !flagSSHKeysAll && len(args) == 0 {
			fmt.Fprintln(os.Stderr, "Error: name a <site> or pass --all.")
			os.Exit(1)
		}
		_, system, captain, err := loadCaptainConfig()
		if err != nil || system == nil {
			fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
			os.Exit(1)
		}
		targets := args
		if flagSSHKeysAll {
			targets = ownServerTargets()
			if len(targets) == 0 {
				fmt.Println("No sites on our own servers.")
				return
			}
		}
		failed := 0
		for _, t := range targets {
			if err := deploySSHKeys(system, captain, t); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", t, err)
				failed++
			}
		}
		if failed > 0 {
			os.Exit(1)
		}
	}
}

// ownServerTargets lists active sites with the provider used for our own
// servers. The Manager makes the final call per environment (own_server).
func ownServerTargets() []string {
	sites, err := models.GetAllActiveSites()
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range sites {
		if s.Provider != "captaincore" {
			continue
		}
		envs, _ := models.FindEnvironmentsBySiteID(s.SiteID)
		for _, e := range envs {
			if e.Address == "" {
				continue
			}
			name := s.Site
			if !strings.EqualFold(e.Environment, "production") {
				name += "-" + strings.ToLower(e.Environment)
			}
			out = append(out, name)
		}
	}
	return out
}

// siteKeyPath resolves the private key the CLI connects to a site with,
// the same way `captaincore ssh` does (site key, else the default key).
func siteKeyPath(system *config.SystemConfig, site *models.Site) (string, error) {
	key := site.ParseDetails().Key
	if key == "" {
		cid, _ := strconv.ParseUint(captainID, 10, 64)
		if raw, _ := models.GetConfiguration(uint(cid), "configurations"); raw != "" {
			var obj map[string]json.RawMessage
			if json.Unmarshal([]byte(raw), &obj) == nil {
				if v, ok := obj["default_key"]; ok {
					key = configValueString(v)
				}
			}
		}
	}
	if key == "" || key == "use_password" || !isSafeShellToken(key) {
		return "", fmt.Errorf("no usable SSH key for %s", site.Site)
	}
	return fmt.Sprintf("%s/%s/%s", system.PathKeys, captainID, key), nil
}

func deploySSHKeys(system *config.SystemConfig, captain *config.CaptainConfig, target string) error {
	sa := parseSiteArgument(target)
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return fmt.Errorf("site not found")
	}
	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil || env.Address == "" {
		return fmt.Errorf("environment %s not found", sa.Environment)
	}
	if !isSSHProtocol(env.Protocol) {
		return fmt.Errorf("protocol %q is not ssh/sftp", env.Protocol)
	}

	client := newAPIClient(system, captain)
	raw, err := client.Post("authorized-keys", map[string]interface{}{
		"site_id": site.SiteID, "environment": env.Environment,
	})
	if err != nil {
		return fmt.Errorf("manager: %v", err)
	}
	var want struct {
		OwnServer bool     `json:"own_server"`
		Keys      []string `json:"keys"`
		Hash      string   `json:"hash"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		return fmt.Errorf("manager reply: %v", err)
	}
	if !want.OwnServer {
		fmt.Printf("%s: skipped, not on one of our servers\n", target)
		return nil
	}

	keyPath, err := siteKeyPath(system, site)
	if err != nil {
		return err
	}
	pub, err := exec.Command("ssh-keygen", "-y", "-f", keyPath).Output()
	if err != nil {
		return fmt.Errorf("reading management key: %v", err)
	}
	mgmt := strings.Fields(strings.TrimSpace(string(pub)))
	if len(mgmt) < 2 {
		return fmt.Errorf("management key has no public half")
	}

	lines := []string{sshKeysBegin, mgmt[0] + " " + mgmt[1] + " captaincore-management"}
	for _, k := range want.Keys {
		// Defence in depth: the Manager validates keys, but these lines land
		// in a file sshd trusts, so anything else is dropped here too.
		if reAuthorizedKey.MatchString(k) {
			lines = append(lines, k)
		}
	}
	lines = append(lines, sshKeysEnd)
	block := strings.Join(lines, "\n")

	// Hash of the Manager's lines only, so the Manager can compare it with
	// its own (the management key line is the CLI's business).
	h := sha256.Sum256([]byte(strings.Join(want.Keys, "\n")))
	hash := hex.EncodeToString(h[:])
	if hash != want.Hash {
		return fmt.Errorf("hash mismatch with the manager (%s vs %s)", hash[:12], want.Hash)
	}

	if flagSSHKeysDryRun {
		fmt.Printf("%s (%s@%s:%s):\n%s\n", target, env.Username, env.Address, env.Port, block)
		return nil
	}

	// Rewrites authorized_keys atomically: keeps lines outside the block
	// (minus any that duplicate a managed key), then appends the block.
	script := `set -e
umask 077
mkdir -p "$HOME/.ssh"
f="$HOME/.ssh/authorized_keys"
touch "$f"
block=$(cat)
tmp=$(mktemp "$HOME/.ssh/.authorized_keys.XXXXXX")
mk=$(mktemp "$HOME/.ssh/.managed.XXXXXX")
printf '%s\n' "$block" | awk '!/^#/ {print $2}' > "$mk"
awk '
  FNR == NR { m[$1] = 1; next }
  /^# BEGIN CaptainCore managed keys/ { skip = 1; next }
  /^# END CaptainCore managed keys/ { skip = 0; next }
  skip { next }
  ($2 in m) { next }
  { print }
' "$mk" "$f" > "$tmp"
rm -f "$mk"
printf '%s\n' "$block" >> "$tmp"
chmod 600 "$tmp"
mv "$tmp" "$f"
echo "keys=$(grep -c '^[a-z]' "$f")"
`
	remote := fmt.Sprintf("%s@%s", env.Username, env.Address)
	c := exec.Command("ssh", "-oBatchMode=yes", "-oStrictHostKeyChecking=no", "-oConnectTimeout=30",
		"-oPreferredAuthentications=publickey", "-oIdentitiesOnly=yes", "-i", keyPath, "-p", env.Port, remote,
		"bash -c "+shellSingleQuote(script))
	c.Stdin = strings.NewReader(block + "\n")
	var out, errBuf bytes.Buffer
	c.Stdout, c.Stderr = &out, &errBuf
	if err := c.Run(); err != nil {
		return fmt.Errorf("ssh: %v %s", err, strings.TrimSpace(errBuf.String()))
	}

	if _, err := client.Post("authorized-keys-deployed", map[string]interface{}{
		"site_id": site.SiteID, "environment": env.Environment,
		"data": map[string]interface{}{"hash": hash, "count": len(want.Keys)},
	}); err != nil {
		return fmt.Errorf("deployed, but reporting back failed: %v", err)
	}
	fmt.Printf("%s: %d operator key(s) + management key deployed (%s)\n", target, len(want.Keys), strings.TrimSpace(out.String()))
	return nil
}

func init() {
	rootCmd.AddCommand(sshKeysCmd)
	sshKeysCmd.AddCommand(sshKeysDeployCmd)
	sshKeysDeployCmd.Flags().BoolVar(&flagSSHKeysAll, "all", false, "Every site on our own servers")
	sshKeysDeployCmd.Flags().BoolVar(&flagSSHKeysDryRun, "dry-run", false, "Print the block instead of writing it")
}
