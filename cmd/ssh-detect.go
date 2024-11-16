package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var sshDetectCmd = &cobra.Command{
	Use:   "ssh-detect <username> <address> <port>",
	Short: "SSH detect if connection valid",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires a <username> <address> <port> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(sshDetectCmd)
	sshDetectCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
}
