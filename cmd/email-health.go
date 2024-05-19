package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var emailHealthCmd = &cobra.Command{
	Use:   "email-health",
	Short: "Email health commands",
}

var emailHealthSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Sends out test email on a site for a health check",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site|target> and <token> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var emailHealthGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Runs email health check on one or more sites",
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

var emailHealthResponseCmd = &cobra.Command{
	Use:   "response <site|target>",
	Short: "Logs response of email health check",
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
	rootCmd.AddCommand(emailHealthCmd)
	emailHealthCmd.AddCommand(emailHealthSendCmd)
	emailHealthCmd.AddCommand(emailHealthGenerateCmd)
	emailHealthCmd.AddCommand(emailHealthResponseCmd)
}
