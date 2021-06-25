package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var deactivateCmd = &cobra.Command{
	Use:   "deactivate <site> [--name=<business-name>] [--link=<business-link>]",
	Short: "Deploys custom deactivate mu-plugin to a site",
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
	rootCmd.AddCommand(deactivateCmd)
	deactivateCmd.Flags().StringVarP(&flagName, "name", "", "", "Business name to show on deactivate page")
	deactivateCmd.Flags().StringVarP(&flagLink, "link", "", "", "Business link to show on deactivate page")
}
