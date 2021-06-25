package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version info",
	Run: func(cmd *cobra.Command, args []string) {
		version := "captaincore 0.12.0\n- go version: " + runtime.Version() + "\n- platform: " + runtime.GOOS + "/" + runtime.GOARCH
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
