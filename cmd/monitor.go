package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor <site|target>",
	Short: "Runs up-time check on one or more sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)
	monitorCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 15, "Number of monitor checks to run at same time")
	monitorCmd.Flags().IntVarP(&flagRetry, "retry", "r", 3, "Number of retries for failures")
	monitorCmd.Flags().StringVarP(&flagPage, "page", "", "", "Check a specific page, example: --page=/wp-admin/")
}
