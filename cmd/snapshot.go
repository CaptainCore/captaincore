package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Snapshot commands",
}

var snapshotGenerateCmd = &cobra.Command{
	Use:   "generate <site> [--email=<email>] [--notes=<notes>] [--filter=<filter-options>] [--skip-remote] [--delete-after-snapshot]",
	Short: "Generates new snapshot for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var snapshotFetchLinkCmd = &cobra.Command{
	Use:   "fetch-link <snapshot-id>",
	Short: "Fetches download link for a snapshot",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <snapshot-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotGenerateCmd)
	snapshotCmd.AddCommand(snapshotFetchLinkCmd)
	snapshotGenerateCmd.Flags().BoolVarP(&flagSkipRemote, "skip-remote", "", false, "Skip sending snapshot to remote storage provider")
	snapshotGenerateCmd.Flags().BoolVarP(&flagDeleteAfterSnapshot, "delete-after-snapshot", "", false, "Delete site after making final snapshot")
	snapshotGenerateCmd.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	snapshotGenerateCmd.Flags().StringVarP(&flagNotes, "notes", "n", "", "Adds a note about the snapshot")
	snapshotGenerateCmd.Flags().StringVarP(&flagUserId, "user-id", "u", "", "User ID")
	snapshotGenerateCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Filter options include one or more of the following: database, themes, plugins, uploads, everything-else. Example --filter=database,themes,plugins will generate a zip with only the database, themes and plugins. Without filter a snapshot will include everything")
}
