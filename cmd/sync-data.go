package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var syncDataCmd = &cobra.Command{
	Use:   "sync-data <site|target>",
	Short: "Sync website data for one or more sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(syncDataCmd)
	syncDataCmd.Flags().BoolVarP(&flagSkipScreenshot, "skip-screenshot", "c", false, "Skip screenshot")
}
