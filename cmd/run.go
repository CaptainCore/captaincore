package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <site> --code=<base64-encoded-code>",
	Short: "Runs arbitrary bash or WP-CLI commands on a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVarP(&flagCode, "code", "c", "", "WP-CLI command or script to run directly")
	runCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Debug mode")
	runCmd.MarkFlagRequired("code")
}
