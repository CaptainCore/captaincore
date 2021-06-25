package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var environmentCmd = &cobra.Command{
	Use:   "environment",
	Short: "Environment commands",
}

var listEnvironmentCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List environments",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(environmentCmd)
	environmentCmd.AddCommand(listEnvironmentCmd)
}
