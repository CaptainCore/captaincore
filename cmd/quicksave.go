package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var quicksaveCmd = &cobra.Command{
	Use:   "quicksave",
	Short: "Quicksave commands",
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

var quicksaveShowChangesCmd = &cobra.Command{
	Use:   "show-changes <site> <commit-hash>",
	Short: "Shows file changes between Quicksaves",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires a <site> and <commit-has> argument")
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
	quicksaveCmd.AddCommand(quicksaveGenerateCmd)
	quicksaveCmd.AddCommand(quicksaveFileDiffCmd)
	quicksaveCmd.AddCommand(quicksaveRollbackCmd)
	quicksaveCmd.AddCommand(quicksaveShowChangesCmd)
	quicksaveCmd.AddCommand(quicksaveSyncCmd)
	quicksaveCmd.AddCommand(quicksaveUpdateUsageCmd)
	quicksaveFileDiffCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme slug")
	quicksaveFileDiffCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin slug")
	quicksaveRollbackCmd.Flags().StringVar(&flagTheme, "theme", "", "Theme to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagPlugin, "plugin", "", "Plugin to rollback")
	quicksaveRollbackCmd.Flags().StringVar(&flagFile, "file", "", "File to rollback")
	quicksaveFileDiffCmd.Flags().BoolVar(&flagHtml, "html", false, "Returns HTML format")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force a new Quicksave")
	quicksaveGenerateCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
}
