package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var captureCmd = &cobra.Command{
	Use:   "capture <site|target>",
	Short: "Captures website's pages visually over time based on quicksaves and html changes",
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
	rootCmd.AddCommand(captureCmd)
	captureCmd.Flags().StringVarP(&flagPage, "pages", "", "", "Overrides pages to check. Defaults to site's capture_pages configuration.")
}
