package cmd

import (
	"github.com/spf13/cobra"
)

var configurationCmd = &cobra.Command{
	Use:   "configuration",
	Short: "Configuration commands",
}

var configurationGetCmd = &cobra.Command{
	Use:   "get [--field=<field>] [--bash]",
	Short: "Get global configuration",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var configurationSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Syncs global configuration to CaptainCore CLI",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(configurationCmd)
	configurationCmd.AddCommand(configurationGetCmd)
	configurationCmd.AddCommand(configurationSyncCmd)

	configurationGetCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	configurationGetCmd.Flags().BoolVarP(&flagBash, "bash", "", false, "Return bash format")
}
