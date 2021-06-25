package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var storeSnapshotCmd = &cobra.Command{
	Use:   "store-snapshot <url|file>",
	Short: "Moves zip to Rclone remote for cold storage",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <url|file> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(storeSnapshotCmd)
}
