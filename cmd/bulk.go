package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var bulkCmd = &cobra.Command{
	Use:   "bulk <command> <target> [<arguments>]",
	Short: "Run command concurrently on many sites",
	Long: `Run command concurrently on many sites.

Unknown flags are passed through to the sub-command, so you can write:
  captaincore bulk ssh @production --command="wp option get home"

Flags:
  -p, --parallel int    Number of sites to run at same time (default 10)
  -d, --debug           Debug mode
      --captain-id string Captain ID (default "1")
      --fleet             Fleet mode
      --label             Print colored site name headers
      --config string     Config file (default "~/.captaincore/config.json")
  -h, --help             Show this help`,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Manual arg parsing since DisableFlagParsing is true.
		// Separate positional args (command + targets) from flags so that
		// CAPTAINCORE_ARGS only contains targets for the bash bulk script.
		var positionalArgs []string
		var passthroughFlags []string
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
			case strings.HasPrefix(arg, "--captain-id="):
				captainID = strings.SplitN(arg, "=", 2)[1]
			case arg == "--captain-id":
				i++
				if i < len(args) {
					captainID = args[i]
				}
			case strings.HasPrefix(arg, "--parallel=") || strings.HasPrefix(arg, "-p="):
				p, _ := strconv.Atoi(strings.SplitN(arg, "=", 2)[1])
				flagParallel = p
			case arg == "--parallel" || arg == "-p":
				i++
				if i < len(args) {
					p, _ := strconv.Atoi(args[i])
					flagParallel = p
				}
			case strings.HasPrefix(arg, "--config="):
				cfgFile = strings.SplitN(arg, "=", 2)[1]
			case arg == "--config":
				i++
				if i < len(args) {
					cfgFile = args[i]
				}
			case strings.HasPrefix(arg, "-"):
				// Pass-through flag for sub-command (e.g. --command, --script)
				passthroughFlags = append(passthroughFlags, arg)
				// If flag uses separate value (--command "value"), grab next arg too
				if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					passthroughFlags = append(passthroughFlags, args[i])
				}
			default:
				positionalArgs = append(positionalArgs, arg)
			}
			i++
		}

		if len(positionalArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Error: requires <command> and <site> arguments\n")
			cmd.Help()
			return
		}

		// Set CAPTAINCORE_ARGS to only the targets (positional args minus the
		// command name) so the bash bulk script can correctly count targets.
		os.Setenv("CAPTAINCORE_ARGS", strings.Join(positionalArgs[1:], " "))

		// Reassemble: positional args + pass-through flags
		cleanArgs := append(positionalArgs, passthroughFlags...)
		resolveCommand(cmd, cleanArgs)
	},
}

func init() {
	rootCmd.AddCommand(bulkCmd)
}
