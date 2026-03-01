package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var accountPortalCmd = &cobra.Command{
	Use:   "account-portal <command>",
	Short: "Account Portal commands",
}

var accountPortalSync = &cobra.Command{
	Use:   "sync <account-portal-id>",
	Short: "Sync an account portal",
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

var accountPortalDelete = &cobra.Command{
	Use:   "delete <account-portal-id>",
	Short: "Delete an account portal",
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
	rootCmd.AddCommand(accountPortalCmd)
	accountPortalCmd.AddCommand(accountPortalSync)
	accountPortalCmd.AddCommand(accountPortalDelete)
}
