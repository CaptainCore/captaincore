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

		// positionalArgs[0] is the command, the rest are targets
		bulkCommand := positionalArgs[0]
		bulkCommand = strings.Replace(bulkCommand, " ", "/", -1)

		cfg := BulkConfig{
			Command:   bulkCommand,
			Targets:   positionalArgs[1:],
			Flags:     passthroughFlags,
			CaptainID: captainID,
			Parallel:  flagParallel,
			Label:     flagLabel,
			Debug:     flagDebug,
		}

		if flagFleet {
			captainIds, nativeErr := fetchCaptainIDsNative()
			if nativeErr != nil {
				fmt.Fprintf(os.Stderr, "Error fetching captain IDs: %s\n", nativeErr)
				os.Exit(1)
			}
			for _, fleetCaptainID := range captainIds {
				cfg.CaptainID = fleetCaptainID
				if err := runBulk(cfg); err != nil {
					fmt.Fprintf(os.Stderr, "Fleet bulk error (captain %s): %s\n", fleetCaptainID, err)
				}
			}
			return
		}

		if err := runBulk(cfg); err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(bulkCmd)
}
