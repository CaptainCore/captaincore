package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var defaultSyncCmd = &cobra.Command{
	Use:   "default-sync",
	Short: "Syncs default",
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, defaultSyncNative)
	},
}

func defaultSyncNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)
	resp, err := client.Post("default-get", nil)
	if err != nil {
		errMsg := err.Error()
		if idx := strings.Index(errMsg, ": <!"); idx > 0 {
			errMsg = errMsg[:idx]
		}
		fmt.Printf("Something went wrong: %s\n", errMsg)
		return
	}

	// Parse the response
	var results struct {
		Defaults json.RawMessage `json:"defaults"`
	}
	if json.Unmarshal(resp, &results) != nil {
		fmt.Print(string(resp))
		return
	}

	// Pretty-print the full response
	var prettyResult interface{}
	if json.Unmarshal(resp, &prettyResult) == nil {
		out, _ := json.MarshalIndent(prettyResult, "", "    ")
		fmt.Print(string(out))
	}

	// Store defaults in the database
	if results.Defaults != nil {
		cid, _ := strconv.ParseUint(captainID, 10, 64)
		models.SetConfiguration(uint(cid), "defaults", string(results.Defaults))
	}
}

func init() {
	rootCmd.AddCommand(defaultSyncCmd)
	defaultSyncCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Debug response")
}
