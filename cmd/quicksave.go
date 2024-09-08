package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var quicksaveCmd = &cobra.Command{
	Use:   "quicksave",
	Short: "Quicksave commands",
}

var quicksaveListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List of quicksaves",
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

var quicksaveListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "Generate list of quicksaves",
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

var quicksaveGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "Generate new quicksave",
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

var quicksaveRestoreGitCmd = &cobra.Command{
	Use:   "restore-git <site>",
	Short: "Restores latest quicksave repo from restic repo",
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

var quicksaveGetCmd = &cobra.Command{
	Use:   "get <site> <hash>",
	Short: "Get quicksave for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveGetGenerateCmd = &cobra.Command{
	Use:   "get-generate <site> <hash>",
	Short: "Generate quicksave response",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveListMissingCmd = &cobra.Command{
	Use:   "list-missing <site>",
	Short: "Generates list of quicksaves for a site that haven't been generated",
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

var quicksaveFileDiffCmd = &cobra.Command{
	Use:   "file-diff <site> <commit> <file>",
	Short: "Shows file diff between Quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <commit> <file> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveRollbackCmd = &cobra.Command{
	Use:   "rollback <site> <commit> [--plugin=<plugin>] [--theme=<theme>] [--all]",
	Short: "Rollback theme, plugin or file from a Quicksave on a site.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <site> <commit> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveLatestCmd = &cobra.Command{
	Use:   "latest <site>",
	Short: "Show most recent quicksave",
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

var quicksaveSearchCmd = &cobra.Command{
	Use:   "search <site> <theme|plugin:title|name:search>",
	Short: "Searches Quicksaves for theme/plugin changes",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <theme|plugin:title|name:search> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var quicksaveShowChangesCmd = &cobra.Command{
	Use:   "show-changes <site> <commit-hash> [<match>]",
	Short: "Shows file changes between Quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <commit-hash> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var quicksaveSyncCmd = &cobra.Command{
	Use:   "sync <site>",
	Short: "Sync quicksaves to CaptainCore API",
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

var quicksaveUpdateUsageCmd = &cobra.Command{
	Use:   "update-usage <site>",
	Short: "Updates Quicksave usage stats",
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
	rootCmd.AddCommand(quicksaveCmd)
	quicksaveCmd.AddCommand(quicksaveGetCmd)
	quicksaveCmd.AddCommand(quicksaveGetGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveLatestCmd)
	quicksaveCmd.AddCommand(quicksaveListCmd)
	quicksaveCmd.AddCommand(quicksaveListGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveListMissingCmd)
	quicksaveCmd.AddCommand(quicksaveFileDiffCmd)
	quicksaveCmd.AddCommand(quicksaveRestoreGitCmd)
	quicksaveCmd.AddCommand(quicksaveRollbackCmd)
	quicksaveCmd.AddCommand(quicksaveSearchCmd)
	quicksaveCmd.AddCommand(quicksaveShowChangesCmd)
	quicksaveCmd.AddCommand(quicksaveSyncCmd)
	quicksaveCmd.AddCommand(quicksaveUpdateUsageCmd)
	quicksaveFileDiffCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme slug")
	quicksaveFileDiffCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin slug")
	quicksaveLatestCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveListCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	quicksaveRollbackCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagVersion, "version", "this", "Rollback to 'this' or 'previous' version")
	quicksaveRollbackCmd.Flags().StringVar(&flagFile, "file", "", "File to rollback")
	quicksaveRollbackCmd.Flags().BoolVar(&flagAll, "all", false, "All themes and plugins")
	quicksaveFileDiffCmd.Flags().BoolVar(&flagHtml, "html", false, "Returns HTML format")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force a new Quicksave")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
}
