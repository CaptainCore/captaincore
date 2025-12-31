package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup commands",
}

var backupCheckCmd = &cobra.Command{
	Use:   "check <site>",
	Short: "Checks integrity of backup repo",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if flagInit {
			os.Setenv("FLAG_INIT", "true")
		}
		resolveCommand(cmd, args)
	},
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

var backupShowCmd = &cobra.Command{
	Use:   "show <site> <backup-id> <file-id>",
	Short: "Retrieve individual file from site backup",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires a <site> <backup-id> and <file-id> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var backupRuntimeCmd = &cobra.Command{
	Use:   "runtime <site>",
	Short: "Returns runtimes of previous backups",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site>")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupCheckCmd)
	backupCmd.AddCommand(backupDownloadCmd)
	backupCmd.AddCommand(backupGenerateCmd)
	backupCmd.AddCommand(backupGetCmd)
	backupCmd.AddCommand(backupGetGenerateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupListGenerateCmd)
	backupCmd.AddCommand(backupListMissingCmd)
	backupCmd.AddCommand(backupShowCmd)
	backupCmd.AddCommand(backupRuntimeCmd)
	backupCheckCmd.Flags().BoolVarP(&flagInit, "init", "", false, "Initialize repo if missing")
	backupDownloadCmd.Flags().StringVarP(&flagEmail, "email", "e", "", "Email notify")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipDB, "skip-db", "", false, "Skip database backup")
	backupGenerateCmd.Flags().BoolVarP(&flagSkipRemote, "skip-remote", "", false, "Skip remote backup")
	backupGenerateCmd.Flags().StringVarP(&flagSkipIfRecent, "skip-if-recent", "", "", "Skip if backup generated within timeframe (e.g. 24h)")
}
