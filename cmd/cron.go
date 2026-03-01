package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Trigger cron tasks",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, cronNative)
	},
}

// cronNative implements `captaincore cron` natively in Go.
func cronNative(cmd *cobra.Command, args []string) {
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	configValue, err := models.GetConfiguration(uint(cid), "configurations")
	if err != nil || configValue == "" {
		return
	}

	var configObj map[string]json.RawMessage
	if json.Unmarshal([]byte(configValue), &configObj) != nil {
		return
	}

	scheduledTasks, ok := configObj["scheduled_tasks"]
	if !ok {
		return
	}

	// Pretty-print scheduled tasks as JSON
	var pretty interface{}
	if json.Unmarshal(scheduledTasks, &pretty) == nil {
		out, _ := json.MarshalIndent(pretty, "", "    ")
		fmt.Println(string(out))
	}
}

func init() {
	rootCmd.AddCommand(cronCmd)
}
