package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var configurationCmd = &cobra.Command{
	Use:   "configuration",
	Short: "Configuration commands",
}

var configurationGetCmd = &cobra.Command{
	Use:   "get [--field=<field>] [--bash]",
	Short: "Get global configuration",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, configurationGetNative)
	},
}

var configurationSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Syncs global configuration to CaptainCore CLI",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, configurationSyncNative)
	},
}

func configurationGetNative(cmd *cobra.Command, args []string) {
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	configValue, err := models.GetConfiguration(uint(cid), "configurations")
	if err != nil || configValue == "" {
		return
	}

	if flagField != "" {
		// Parse as JSON object and extract field
		var configObj map[string]json.RawMessage
		if json.Unmarshal([]byte(configValue), &configObj) == nil {
			if val, ok := configObj[flagField]; ok {
				// Try to unquote string values, otherwise output raw
				var s string
				if json.Unmarshal(val, &s) == nil {
					fmt.Print(s)
				} else {
					fmt.Print(string(val))
				}
				return
			}
		}
		return
	}

	// Output full JSON — parse and re-encode for pretty printing
	var configObj interface{}
	if json.Unmarshal([]byte(configValue), &configObj) == nil {
		out, _ := json.Marshal(configObj)
		fmt.Print(string(out))
	} else {
		fmt.Print(configValue)
	}
}

func configurationSyncNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)
	resp, err := client.Post("configuration-get", nil)
	if err != nil {
		// Extract just the status code, not the full HTML body
		errMsg := err.Error()
		if idx := strings.Index(errMsg, ": <!"); idx > 0 {
			errMsg = errMsg[:idx]
		}
		fmt.Printf("Something went wrong: %s\n", errMsg)
		return
	}

	// Parse the response
	var results struct {
		Configurations json.RawMessage `json:"configurations"`
	}
	if json.Unmarshal(resp, &results) != nil {
		// Output raw response
		fmt.Print(string(resp))
		return
	}

	// Pretty-print the full response
	var prettyResult interface{}
	if json.Unmarshal(resp, &prettyResult) == nil {
		out, _ := json.MarshalIndent(prettyResult, "", "    ")
		fmt.Print(string(out))
	}

	// Store configurations in the database
	if results.Configurations != nil {
		cid, _ := strconv.ParseUint(captainID, 10, 64)
		models.SetConfiguration(uint(cid), "configurations", string(results.Configurations))
	}
}

func init() {
	rootCmd.AddCommand(configurationCmd)
	configurationCmd.AddCommand(configurationGetCmd)
	configurationCmd.AddCommand(configurationSyncCmd)

	configurationGetCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	configurationGetCmd.Flags().BoolVarP(&flagBash, "bash", "", false, "Return bash format")
}
