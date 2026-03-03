package cmd

import (
	"fmt"
	"runtime"

	"github.com/CaptainCore/captaincore/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version info",
	Run: func(cmd *cobra.Command, args []string) {
		v := "captaincore " + version.Version + "\n- go version: " + runtime.Version() + "\n- platform: " + runtime.GOOS + "/" + runtime.GOARCH
		fmt.Println(v)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
