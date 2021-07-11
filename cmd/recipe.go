package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var recipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "Recipe commands",
}

var recipeAddCmd = &cobra.Command{
	Use:   "add <recipe> [--format=<format>]",
	Short: "Add or update recipe",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <recipe> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(recipeCmd)
	recipeCmd.AddCommand(recipeAddCmd)
	recipeAddCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Format of input. Supports base64.")
}
