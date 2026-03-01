package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var sshVerifyCmd = &cobra.Command{
	Use:   "ssh-verify <ssh-connection-string>",
	Short: "Verifies a valid SSH connection",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <ssh-connection-string> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(sshVerifyCmd)
}
