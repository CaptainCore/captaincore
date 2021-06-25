package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var activateCmd = &cobra.Command{
	Use:   "activate <site>",
	Short: "Removes custom deactivate mu-plugin on a site",
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
	rootCmd.AddCommand(activateCmd)
}
