package cmd

import (
	"github.com/CaptainCore/cli/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start server",
	Run: func(cmd *cobra.Command, args []string) {
		server.HandleRequests(flagDebug)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Enable debug mode")
}
