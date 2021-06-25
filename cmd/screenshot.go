package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot <site|target>",
	Short: "Takes screenshot of one or more sites",
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
	rootCmd.AddCommand(screenshotCmd)
	screenshotCmd.Flags().IntVarP(&flagParallel, "parallel", "p", 5, "Number of screenshots to run at same time")
	screenshotCmd.Flags().StringVarP(&flagPage, "page", "", "", "Check a specific page, example: --page=/wp-admin/")
}
