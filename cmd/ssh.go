package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagScriptPassthrough []string

// flagSSHTenant runs the command inside one WP Freighter tenant of the target
// site (its host), by exporting STACKED_SITE_ID the way a tenant site's own
// environment vars would. Lets the Manager reach tenants that are not
// CaptainCore sites, e.g. to install the helper there.
var flagSSHTenant string

var sshCmd = &cobra.Command{
	Use:   "ssh <site>... [--command=<commands>] [--script=<name|file>] [flags...]",
	Short: "SSH connection to a site",
	Long: `SSH connection to a site.

Unknown flags are passed through to remote scripts/recipes, so you can write:
  captaincore ssh mysite --script=fetch-error-log-size --human-readable

Flags:
  -c, --command string   WP-CLI command or script to run directly
  -r, --recipe string    Run a built-in or custom defined recipe
  -s, --script string    Run a built-in script file
  -d, --debug            Preview ssh command
      --tenant int       Run inside this WP Freighter tenant of the site (sets STACKED_SITE_ID)
      --captain-id string Captain ID (default "1")
      --fleet             Fleet mode
      --config string     Config file (default "~/.captaincore/config.json")
  -h, --help             Show this help`,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Reset passthrough flags for this invocation
		flagScriptPassthrough = nil
		flagSSHTenant = ""

		// Manual arg parsing since DisableFlagParsing is true
		var targets []string
		i := 0
		for i < len(args) {
			arg := args[i]
			switch {
			case arg == "--help" || arg == "-h":
				cmd.Help()
				return
			case arg == "--debug" || arg == "-d":
				flagDebug = true
			case arg == "--label":
				flagLabel = true
			case arg == "--fleet":
				flagFleet = true
			case strings.HasPrefix(arg, "--script="):
				flagScript = strings.SplitN(arg, "=", 2)[1]
			case strings.HasPrefix(arg, "-s="):
				flagScript = strings.SplitN(arg, "=", 2)[1]
			case arg == "--script" || arg == "-s":
				i++
				if i < len(args) {
					flagScript = args[i]
				}
			case strings.HasPrefix(arg, "--command="):
				flagCommand = strings.SplitN(arg, "=", 2)[1]
			case strings.HasPrefix(arg, "-c="):
				flagCommand = strings.SplitN(arg, "=", 2)[1]
			case arg == "--command" || arg == "-c":
				i++
				if i < len(args) {
					flagCommand = args[i]
				}
			case strings.HasPrefix(arg, "--recipe="):
				flagRecipe = strings.SplitN(arg, "=", 2)[1]
			case strings.HasPrefix(arg, "-r="):
				flagRecipe = strings.SplitN(arg, "=", 2)[1]
			case arg == "--recipe" || arg == "-r":
				i++
				if i < len(args) {
					flagRecipe = args[i]
				}
			case strings.HasPrefix(arg, "--captain-id="):
				captainID = strings.SplitN(arg, "=", 2)[1]
			case arg == "--captain-id":
				i++
				if i < len(args) {
					captainID = args[i]
				}
			case strings.HasPrefix(arg, "--config="):
				cfgFile = strings.SplitN(arg, "=", 2)[1]
			case arg == "--config":
				i++
				if i < len(args) {
					cfgFile = args[i]
				}
			case strings.HasPrefix(arg, "--tenant="):
				flagSSHTenant = strings.SplitN(arg, "=", 2)[1]
			case arg == "--tenant":
				i++
				if i < len(args) {
					flagSSHTenant = args[i]
				}
			case strings.HasPrefix(arg, "--parallel="):
				p, _ := strconv.Atoi(strings.SplitN(arg, "=", 2)[1])
				flagParallel = p
			case arg == "--parallel" || arg == "-p":
				i++
				if i < len(args) {
					p, _ := strconv.Atoi(args[i])
					flagParallel = p
				}
			case arg == "--":
				// Skip bare separator
			case strings.HasPrefix(arg, "-"):
				flagScriptPassthrough = append(flagScriptPassthrough, arg)
			default:
				targets = append(targets, arg)
			}
			i++
		}

		if len(targets) < 1 {
			fmt.Fprintf(os.Stderr, "Error: requires a <site|target> argument\n")
			cmd.Help()
			os.Exit(1)
		}

		if flagSSHTenant != "" && (len(targets) > 1 || strings.HasPrefix(targets[0], "@")) {
			fmt.Fprintf(os.Stderr, "Error: --tenant works on a single site\n")
			os.Exit(1)
		}

		// Check for bulk/target mode — delegate to bash
		if strings.HasPrefix(targets[0], "@production") || strings.HasPrefix(targets[0], "@staging") || strings.HasPrefix(targets[0], "@all") {
			resolveCommand(cmd, targets)
			return
		}
		if len(targets) > 1 {
			resolveCommand(cmd, targets)
			return
		}
		resolveNativeOrWP(cmd, targets, sshNative)
	},
}

// sshNative implements `captaincore ssh <site>` natively in Go.
// It builds the SSH command string and either prints it (--debug) or executes it.
func sshNative(cmd *cobra.Command, args []string) {
	colorRed := "\033[31m"
	colorNormal := "\033[39m"

	// Parse the site argument
	siteArg := args[0]
	sa := parseSiteArgument(siteArg)

	// Collect passthrough flags for remote scripts/recipes
	additionalArgs := append([]string(nil), flagScriptPassthrough...)

	if sa.SiteName == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s Please specify a <site>.\n", colorRed, colorNormal)
		os.Exit(1)
	}

	// Load config
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Fprintln(os.Stderr, "Error: Configuration file not found.")
		os.Exit(1)
	}

	// Look up site
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Site '%s' not found.\n", colorRed, colorNormal, sa.SiteName)
		os.Exit(1)
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment %s not found for '%s'.\n", colorRed, colorNormal, sa.Environment, site.Name)
		os.Exit(1)
	}

	if env.Address == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment %s not found for '%s'.\n", colorRed, colorNormal, env.Environment, site.Name)
		os.Exit(1)
	}

	// Defence in depth: connect refuses these at ingest, but the slug is
	// interpolated into the bash -c command string below either way.
	if !isSafeSiteSlug(site.Site) {
		fmt.Fprintf(os.Stderr, "%sError:%s Refusing unsafe site slug %q.\n", colorRed, colorNormal, site.Site)
		os.Exit(1)
	}

	if env.Protocol != "sftp" {
		fmt.Fprintf(os.Stderr, "%sError:%s SSH not supported (Protocol is %s).\n", colorRed, colorNormal, env.Protocol)
		os.Exit(1)
	}

	// Parse site details
	siteDetails := site.ParseDetails()
	environmentVars := ""
	if siteDetails.EnvironmentVars != nil && string(siteDetails.EnvironmentVars) != "" && string(siteDetails.EnvironmentVars) != "null" {
		var envVarsList []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal(siteDetails.EnvironmentVars, &envVarsList) == nil {
			for _, item := range envVarsList {
				// Key is emitted unquoted (LHS of export) — must be a valid shell
				// identifier. Value is single-quote escaped so it can't break out.
				if !isValidEnvKey(item.Key) {
					continue
				}
				// The value is single-quoted for the remote shell. The whole
				// remote command is single-quoted again below, so the local
				// shell never reads it.
				environmentVars = fmt.Sprintf("export %s=%s && %s", item.Key, shellSingleQuote(item.Value), environmentVars)
			}
		}
	}

	if flagSSHTenant != "" {
		// Digits only: it lands unquoted in the remote export, and a
		// Freighter tenant id is always a positive integer.
		if !reTenantID.MatchString(flagSSHTenant) {
			fmt.Fprintf(os.Stderr, "%sError:%s --tenant must be a numeric WP Freighter tenant id.\n", colorRed, colorNormal)
			os.Exit(1)
		}
		// After the site's own vars, so it wins when the site is itself a
		// tenant whose environment vars already export STACKED_SITE_ID.
		environmentVars = fmt.Sprintf("%sexport STACKED_SITE_ID=%s && ", environmentVars, flagSSHTenant)
	}

	// Determine SSH key
	remoteOptions := "-oStrictHostKeyChecking=no -oConnectTimeout=30 -oServerAliveInterval=60 -oServerAliveCountMax=10"
	beforeSSH := ""

	key := siteDetails.Key
	if key != "use_password" && key == "" {
		// Look up default key from configurations
		cid, _ := strconv.ParseUint(captainID, 10, 64)
		configValue, _ := models.GetConfiguration(uint(cid), "configurations")
		if configValue != "" {
			var configObj map[string]json.RawMessage
			if json.Unmarshal([]byte(configValue), &configObj) == nil {
				if defaultKeyRaw, ok := configObj["default_key"]; ok {
					key = configValueString(defaultKeyRaw)
				}
			}
		}
	}

	if key != "use_password" {
		// The key name is emitted unquoted after `ssh -i` in a bash -c string.
		if !isSafeShellToken(key) {
			fmt.Fprintf(os.Stderr, "%sError:%s Refusing unsafe SSH key name %q for '%s'.\n", colorRed, colorNormal, key, site.Site)
			os.Exit(1)
		}
		remoteOptions = fmt.Sprintf("%s -oPreferredAuthentications=publickey -i %s/%s/%s", remoteOptions, system.PathKeys, captainID, key)
	} else {
		// Single-quote escape the password so a embedded quote can't break out
		// of the sshpass argument into the local shell.
		beforeSSH = "sshpass -p " + shellSingleQuote(env.Password)
	}

	// Build command prep and remote server based on provider
	var commandPrep, remoteServer string
	target := fmt.Sprintf("%s@%s port %s", env.Username, env.Address, env.Port)
	switch site.Provider {
	case "kinsta":
		commandPrep = fmt.Sprintf("%s cd public/ &&", environmentVars)
		remoteServer = fmt.Sprintf("%s %s@%s -p %s", remoteOptions, env.Username, env.Address, env.Port)
	case "wpengine":
		commandPrep = fmt.Sprintf("%s rm ~/.wp-cli/config.yml; cd sites/* &&", environmentVars)
		remoteServer = fmt.Sprintf("%s %s@%s.ssh.wpengine.net", remoteOptions, site.Site, site.Site)
		target = fmt.Sprintf("%s@%s.ssh.wpengine.net", site.Site, site.Site)
	case "rocketdotnet":
		commandPrep = fmt.Sprintf("%s cd %s/ &&", environmentVars, env.HomeDirectory)
		remoteServer = fmt.Sprintf("%s %s@%s -p %s", remoteOptions, env.Username, env.Address, env.Port)
	default:
		commandPrep = fmt.Sprintf("%s cd %s/ &&", environmentVars, env.HomeDirectory)
		remoteServer = fmt.Sprintf("%s %s@%s -p %s", remoteOptions, env.Username, env.Address, env.Port)
	}

	// Format additional args for passing through, shell-quoting each
	// arg so special characters (apostrophes, spaces, etc.) survive
	// the remote shell invocation.
	additionalArgsStr := ""
	if len(additionalArgs) > 0 {
		quoted := make([]string, len(additionalArgs))
		for i, arg := range additionalArgs {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		}
		additionalArgsStr = strings.Join(quoted, " ")
	}

	// -q keeps banners and the 16-line REMOTE HOST IDENTIFICATION HAS CHANGED
	// notice (common across a fleet whose sites move hosts) out of command
	// output, but it also hid why a connection failed. A real run sends ssh's
	// own messages to a temp log (-E) instead and prints them only when the
	// connection fails. --debug keeps printing the -q form with the ssh-fail
	// suffix, since backup/generate evals that string.
	sshVerb := "ssh -q"
	sshFailSuffix := ""
	sshLog := ""
	if flagDebug {
		sshFailSuffix = fmt.Sprintf(" || captaincore site ssh-fail %s --captain-id=%s", site.Site, captainID)
	} else if f, err := os.CreateTemp("", "captaincore-ssh-*.log"); err == nil {
		sshLog = f.Name()
		f.Close()
		sshVerb = "ssh -oLogLevel=ERROR -E " + shellSingleQuote(sshLog)
	}

	// When FLAG_LABEL is set, wrap remote commands with markers so the
	// labeled_run shell function can strip the SSH MOTD/banner and extract
	// only the actual command output.
	labelMode := os.Getenv("FLAG_LABEL") == "true"
	markerStart := "____CC_OUTPUT_START____"
	markerEnd := "____CC_OUTPUT_END____"

	// remote is the command line the site's shell runs. It reaches ssh as ONE
	// single-quoted word, so the local bash -c never expands $VAR, $(...) or
	// backticks in it: a --command runs on the site, as written.
	var remote, inputFile string

	if flagCommand != "" {
		// Strip one pair of double quotes wrapped around the whole command, as
		// before, but keep quotes that belong to it: trimming every leading and
		// trailing quote broke commands such as printf "%s" "$X".
		command := flagCommand
		if len(command) >= 2 && strings.HasPrefix(command, `"`) && strings.HasSuffix(command, `"`) && !strings.Contains(command[1:len(command)-1], `"`) {
			command = command[1 : len(command)-1]
		}
		if labelMode {
			remote = fmt.Sprintf("%s echo %s && %s && echo %s", commandPrep, markerStart, command, markerEnd)
		} else {
			remote = fmt.Sprintf("%s %s", commandPrep, command)
		}
	} else if flagScript != "" || flagRecipe != "" {
		if flagScript != "" {
			inputFile = flagScript
			// Check if it's an absolute/relative path that exists
			if _, err := os.Stat(inputFile); os.IsNotExist(err) {
				// Try built-in remote-scripts location
				home, _ := os.UserHomeDir()
				builtinPath := fmt.Sprintf("%s/.captaincore/lib/remote-scripts/%s", home, flagScript)
				if _, err := os.Stat(builtinPath); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "Error: Can't locate script %s\n", flagScript)
					os.Remove(sshLog)
					os.Exit(1)
				}
				inputFile = builtinPath
			}
		} else {
			inputFile = flagRecipe
			if _, err := os.Stat(inputFile); os.IsNotExist(err) {
				// Try recipes path
				builtinPath := fmt.Sprintf("%s/%s-%s.sh", system.PathRecipes, captainID, flagRecipe)
				if _, err := os.Stat(builtinPath); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "Error: Can't locate recipe %s\n", flagRecipe)
					os.Remove(sshLog)
					os.Exit(1)
				}
				inputFile = builtinPath
			}
		}
		if labelMode {
			remote = fmt.Sprintf("%s echo %s && bash -s -- --site=%s %s && echo %s", commandPrep, markerStart, site.Site, additionalArgsStr, markerEnd)
		} else {
			remote = fmt.Sprintf("%s bash -s -- --site=%s %s", commandPrep, site.Site, additionalArgsStr)
		}
	}

	var sshCommand string
	if remote == "" {
		// Interactive SSH
		sshCommand = fmt.Sprintf("%s %s %s", beforeSSH, sshVerb, remoteServer)
	} else {
		redirect := ""
		if inputFile != "" {
			redirect = " < " + shellSingleQuote(inputFile)
		}
		sshCommand = fmt.Sprintf("%s %s %s %s%s%s", beforeSSH, sshVerb, remoteServer, shellSingleQuote(strings.TrimSpace(remote)), redirect, sshFailSuffix)
	}

	// Clean up extra spaces
	sshCommand = strings.TrimSpace(sshCommand)

	if flagDebug {
		fmt.Println(sshCommand)
		return
	}

	// Execute the SSH command
	shellCmd := exec.Command("bash", "-c", sshCommand)
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr
	code := exitStatus(shellCmd.Run())

	// ssh exits 255 when the connection fails, but so does a remote command
	// that dies of a PHP fatal. ssh's own log tells the two apart: it holds an
	// error only when ssh itself failed. sshpass reports a rejected password
	// as 5. Any other status belongs to the remote command, so a failing wp
	// command no longer marks the site's connection as broken.
	var sshErrors []string
	connectionFailed := false
	if code == 255 {
		if sshLog == "" {
			connectionFailed = true
		} else if data, err := os.ReadFile(sshLog); err == nil {
			sshErrors = sshLogErrors(string(data))
			connectionFailed = len(sshErrors) > 0
		}
	}
	if beforeSSH != "" && code == 5 {
		connectionFailed = true
		sshErrors = append(sshErrors, "sshpass: the password was rejected.")
	}
	if sshLog != "" {
		os.Remove(sshLog)
	}

	if connectionFailed {
		fmt.Fprintf(os.Stderr, "%sError:%s SSH connection to %s failed.\n", colorRed, colorNormal, target)
		for _, line := range sshErrors {
			fmt.Fprintln(os.Stderr, line)
		}
		if flagCommand != "" || flagScript != "" || flagRecipe != "" {
			siteSSHFailNative(cmd, []string{site.Site})
		}
	}

	if code != 0 {
		os.Exit(code)
	}
}

// exitStatus maps the error from exec.Cmd.Run to a process exit code.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code > 0 {
			return code
		}
	}
	return 1
}

// sshLogErrors returns the lines of an ssh -E log (LogLevel=ERROR) that report
// a real failure. It drops the REMOTE HOST IDENTIFICATION HAS CHANGED notice,
// which ssh logs at the same level on connections that then succeed.
func sshLogErrors(log string) []string {
	notice := []string{
		"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY",
		"Someone could be eavesdropping on you",
		"It is also possible that a host key has just been changed",
		"The fingerprint for the ",
		"SHA256:",
		"MD5:",
		"Please contact your system administrator",
		"Add correct host key in ",
		"Offending ",
		"remove with:",
		"ssh-keygen -f ",
		"is disabled to avoid man-in-the-middle attacks",
		"is disabled because the host key is not trusted",
	}
	var lines []string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "@") {
			continue
		}
		isNotice := false
		for _, n := range notice {
			if strings.Contains(line, n) {
				isNotice = true
				break
			}
		}
		if !isNotice {
			lines = append(lines, line)
		}
	}
	return lines
}

var reTenantID = regexp.MustCompile(`^[1-9][0-9]{0,5}$`)

func init() {
	rootCmd.AddCommand(sshCmd)
}
