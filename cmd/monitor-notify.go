package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var monitorNotifyCmd = &cobra.Command{
	Use:   "monitor-notify <account-portal-id>",
	Short: "Monitor notify an account portal administrator",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires an <account-portal-id> argument  --log=<log-file> --monitor-file=<monitor-file> --urls=<urls>")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Monitoring...")
	},
}

func init() {
	rootCmd.AddCommand(monitorNotifyCmd)
}
