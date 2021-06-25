package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats <site>",
	Short: "Fetches stats from WordPress.com stats, Fathom Analytics or Fathom Lite",
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
	rootCmd.AddCommand(statsCmd)
}
