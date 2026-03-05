package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagScriptPassthrough []string

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
      --captain-id string Captain ID (default "1")
      --fleet             Fleet mode
      --config string     Config file (default "~/.captaincore/config.json")
  -h, --help             Show this help`,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Reset passthrough flags for this invocation
		flagScriptPassthrough = nil

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
			return
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
		return
	}

	// Load config
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Look up site
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Site '%s' not found.\n", colorRed, colorNormal, sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment %s not found for '%s'.\n", colorRed, colorNormal, sa.Environment, site.Name)
		return
	}

	if env.Address == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment %s not found for '%s'.\n", colorRed, colorNormal, env.Environment, site.Name)
		return
	}

	if env.Protocol != "sftp" {
		fmt.Fprintf(os.Stderr, "%sError:%s SSH not supported (Protocol is %s).", colorRed, colorNormal, env.Protocol)
		return
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
				environmentVars = fmt.Sprintf("export %s='%s' && %s", item.Key, item.Value, environmentVars)
			}
		}
	}

	// Determine SSH key
	remoteOptions := "-q -oStrictHostKeyChecking=no"
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
					var defaultKey string
					json.Unmarshal(defaultKeyRaw, &defaultKey)
					key = defaultKey
				}
			}
		}
	}

	if key != "use_password" {
		remoteOptions = fmt.Sprintf("%s -oPreferredAuthentications=publickey -i %s/%s/%s", remoteOptions, system.PathKeys, captainID, key)
	} else {
		beforeSSH = fmt.Sprintf("sshpass -p '%s'", env.Password)
	}

	// Build command prep and remote server based on provider
	var commandPrep, remoteServer string
	switch site.Provider {
	case "kinsta":
		commandPrep = fmt.Sprintf("%s cd public/ &&", environmentVars)
		remoteServer = fmt.Sprintf("%s %s@%s -p %s", remoteOptions, env.Username, env.Address, env.Port)
	case "wpengine":
		commandPrep = fmt.Sprintf("%s rm ~/.wp-cli/config.yml; cd sites/* &&", environmentVars)
		remoteServer = fmt.Sprintf("%s %s@%s.ssh.wpengine.net", remoteOptions, site.Site, site.Site)
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

	sshFailSuffix := fmt.Sprintf(" || captaincore site ssh-fail %s --captain-id=%s", site.Site, captainID)

	// When FLAG_LABEL is set, wrap remote commands with markers so the
	// labeled_run shell function can strip the SSH MOTD/banner and extract
	// only the actual command output.
	labelMode := os.Getenv("FLAG_LABEL") == "true"
	markerStart := "____CC_OUTPUT_START____"
	markerEnd := "____CC_OUTPUT_END____"

	var sshCommand string

	if flagCommand != "" {
		command := strings.Trim(flagCommand, "\"")
		if labelMode {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s echo %s && %s && echo %s\"%s", beforeSSH, remoteServer, commandPrep, markerStart, command, markerEnd, sshFailSuffix)
		} else {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s %s\"%s", beforeSSH, remoteServer, commandPrep, command, sshFailSuffix)
		}
	} else if flagScript != "" {
		scriptFile := flagScript
		// Check if it's an absolute/relative path that exists
		if _, err := os.Stat(scriptFile); os.IsNotExist(err) {
			// Try built-in remote-scripts location
			home, _ := os.UserHomeDir()
			builtinPath := fmt.Sprintf("%s/.captaincore/lib/remote-scripts/%s", home, flagScript)
			if _, err := os.Stat(builtinPath); os.IsNotExist(err) {
				fmt.Printf("Error: Can't locate script %s", flagScript)
				return
			}
			scriptFile = builtinPath
		}
		if labelMode {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s echo %s && bash -s -- --site=%s %s && echo %s\" < %s%s", beforeSSH, remoteServer, commandPrep, markerStart, site.Site, additionalArgsStr, markerEnd, scriptFile, sshFailSuffix)
		} else {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s bash -s -- --site=%s %s\" < %s%s", beforeSSH, remoteServer, commandPrep, site.Site, additionalArgsStr, scriptFile, sshFailSuffix)
		}
	} else if flagRecipe != "" {
		recipeFile := flagRecipe
		if _, err := os.Stat(recipeFile); os.IsNotExist(err) {
			// Try recipes path
			builtinPath := fmt.Sprintf("%s/%s-%s.sh", system.PathRecipes, captainID, flagRecipe)
			if _, err := os.Stat(builtinPath); os.IsNotExist(err) {
				fmt.Printf("Error: Can't locate recipe %s", flagRecipe)
				return
			}
			recipeFile = builtinPath
		}
		if labelMode {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s echo %s && bash -s -- --site=%s %s && echo %s\" < %s%s", beforeSSH, remoteServer, commandPrep, markerStart, site.Site, additionalArgsStr, markerEnd, recipeFile, sshFailSuffix)
		} else {
			sshCommand = fmt.Sprintf("%s ssh %s \"%s bash -s -- --site=%s %s\" < %s%s", beforeSSH, remoteServer, commandPrep, site.Site, additionalArgsStr, recipeFile, sshFailSuffix)
		}
	} else {
		// Interactive SSH
		sshCommand = fmt.Sprintf("%s ssh %s", beforeSSH, remoteServer)
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
	shellCmd.Run()
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
