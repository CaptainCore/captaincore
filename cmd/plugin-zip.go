package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var pluginZipCmd = &cobra.Command{
	Use:   "plugin-zip <site> <plugin>",
	Short: "Generates plugin zips on a site",
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
	rootCmd.AddCommand(pluginZipCmd)
}
