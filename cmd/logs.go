package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Logs commands",
}

var logsListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "Retrieve list of server logs for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var logsGetCmd = &cobra.Command{
	Use:   "get <site> --file=<filename>",
	Short: "Retrieves server log file for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsListCmd)
	logsCmd.AddCommand(logsGetCmd)
	logsGetCmd.Flags().StringVar(&flagFile, "file", "", "File to retrieve")
	logsGetCmd.Flags().StringVar(&flagLimit, "limit", "", "Limit number of lines")
}
