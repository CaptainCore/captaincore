package cmd

import (
	"github.com/spf13/cobra"
)

var defaultSyncCmd = &cobra.Command{
	Use:   "default-sync",
	Short: "Syncs default",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(defaultSyncCmd)
}
