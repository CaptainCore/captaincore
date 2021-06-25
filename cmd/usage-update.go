package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var usageUpdateCmd = &cobra.Command{
	Use:   "usage-update <site>",
	Short: "Generates usage stats for a site",
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
	rootCmd.AddCommand(usageUpdateCmd)
}
