package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <site>... [--command=<commands>] [--script=<name|file>] [--<script-args>=<value>]",
	Short: "SSH connection to a site",
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
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().StringVarP(&flagCommand, "command", "c", "", "WP-CLI command or script to run directly")
	sshCmd.Flags().StringVarP(&flagRecipe, "recipe", "r", "", "Run a built-in or custom defined recipe")
	sshCmd.Flags().StringVarP(&flagScript, "script", "s", "", "Run a built-in script file")
	sshCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
}
