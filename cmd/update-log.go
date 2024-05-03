package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var updateLogCmd = &cobra.Command{
	Use:   "update-log",
	Short: "Update log commands",
}

var updateLogGetCmd = &cobra.Command{
	Use:   "get <site>",
	Short: "Get update log for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <quicksave-hash-before> <quicksave-hash-after> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var updateLogGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "generates new update log",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <quicksave-hash-before> <quicksave-hash-after> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var updateLogListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List of update logs",
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

var updateLogListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "generates new update log list",
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

func init() {
	rootCmd.AddCommand(updateLogCmd)
	updateLogCmd.AddCommand(updateLogGetCmd)
	updateLogCmd.AddCommand(updateLogGenerateCmd)
	updateLogCmd.AddCommand(updateLogListCmd)
	updateLogCmd.AddCommand(updateLogListGenerateCmd)
}
