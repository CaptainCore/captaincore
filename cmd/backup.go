package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup commands",
}

var backupDownloadCmd = &cobra.Command{
	Use:   "download <site> <backup-id> <payload> [--email=<email>]",
	Short: "Download a backup for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <backup-id> <payload> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "Generates new backup for a site",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupGetCmd = &cobra.Command{
	Use:   "get <site> <backup-id>",
	Short: "Fetches backup for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <site> and <backup-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupGetGenerateCmd = &cobra.Command{
	Use:   "get-generate <site> <backup-id>",
	Short: "Generate contents of a backup",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "Fetches list of snapshots for a site",
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

var backupListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "Generates list of snapshots for a site",
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupListMissingCmd = &cobra.Command{
	Use:   "list-missing <site>",
	Short: "Generates list of snapshots for a site that haven't been generated",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupDownloadCmd)
	backupCmd.AddCommand(backupGenerateCmd)
	backupCmd.AddCommand(backupGetCmd)
	backupCmd.AddCommand(backupGetGenerateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupListGenerateCmd)
	backupCmd.AddCommand(backupListMissingCmd)
	backupDownloadCmd.Flags().StringVarP(&flagEmail, "email", "e", "", "Email notify")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipDB, "skip-db", "", false, "Skip database backup")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipRemote, "skip-remote", "", false, "Skip remote backup")
}
