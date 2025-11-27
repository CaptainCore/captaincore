package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var archiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive commands",
}

var archiveListCmd = &cobra.Command{
	Use:   "list",
	Short: "Fetch list of files for defined archive location",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var archiveShareCmd = &cobra.Command{
	Use:   "share <file>",
	Short: "Create public shareable link for a given file which expires in 7 days",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <file> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(archiveCmd)
	archiveCmd.AddCommand(archiveListCmd)
	archiveCmd.AddCommand(archiveShareCmd)
}
