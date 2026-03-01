package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CaptainCore/captaincore/models"
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
		resolveNativeOrWP(cmd, args, recipeAddNative)
	},
}

// recipeAddNative implements `captaincore recipe add <recipe>` natively in Go.
func recipeAddNative(cmd *cobra.Command, args []string) {
	recipeArg := args[0]

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	if flagFormat == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(recipeArg)
		if err != nil {
			return
		}

		var recipe models.Recipe
		if json.Unmarshal(decoded, &recipe) != nil {
			return
		}

		if err := models.UpsertRecipe(&recipe); err != nil {
			return
		}

		// Write .sh file
		if system.PathRecipes != "" {
			recipeFile := filepath.Join(system.PathRecipes, fmt.Sprintf("%s-%d.sh", captainID, recipe.RecipeID))
			fmt.Printf("Generating %s\n", recipeFile)
			os.WriteFile(recipeFile, []byte(recipe.Content), 0644)
		}
	}
}

func init() {
	rootCmd.AddCommand(recipeCmd)
	recipeCmd.AddCommand(recipeAddCmd)
	recipeAddCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Format of input. Supports base64.")
}
