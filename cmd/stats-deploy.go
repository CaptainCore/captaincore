package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var statsDeployCmd = &cobra.Command{
	Use:   "stats-deploy <site>",
	Short: "Deploys Fathom tracker to a site",
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
	rootCmd.AddCommand(statsDeployCmd)
}
