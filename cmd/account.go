package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account <site>",
	Short: "Account commands",
}

var accountSync = &cobra.Command{
	Use:   "sync <account>",
	Short: "Syncs account",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <account> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(accountCmd)
	accountCmd.AddCommand(accountSync)
}
