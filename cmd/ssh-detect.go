package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var sshDetectCmd = &cobra.Command{
	Use:   "ssh-detect <username> <address> <port>",
	Short: "SSH detect if connection valid",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires a <username> <address> <port> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, sshDetectNative)
	},
}

func sshDetectNative(cmd *cobra.Command, args []string) {
	username := args[0]
	address := args[1]
	port := args[2]

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Look up default key from configurations
	cid, _ := strconv.ParseUint(captainID, 10, 64)
	configValue, _ := models.GetConfiguration(uint(cid), "configurations")
	key := ""
	if configValue != "" {
		var configObj map[string]json.RawMessage
		if json.Unmarshal([]byte(configValue), &configObj) == nil {
			if defaultKeyRaw, ok := configObj["default_key"]; ok {
				json.Unmarshal(defaultKeyRaw, &key)
			}
		}
	}

	keyPath := fmt.Sprintf("%s/%s/%s", system.PathKeys, captainID, key)
	sshCommand := fmt.Sprintf("ssh -q -oStrictHostKeyChecking=no -oPreferredAuthentications=publickey -i %s %s@%s -p %s \"cd public; pwd\"", keyPath, username, address, port)

	if flagDebug {
		fmt.Println(sshCommand)
		return
	}

	shellCmd := exec.Command("bash", "-c", sshCommand)
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr
	shellCmd.Run()
}

func init() {
	rootCmd.AddCommand(sshDetectCmd)
	sshDetectCmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Preview ssh command")
}
