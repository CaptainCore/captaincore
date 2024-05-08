package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update <site>",
	Short: "Runs themes, plugins and core updates on a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Debug mode. No updates will run.")
	updateCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 5, "Number of sites to update at same time")
}
