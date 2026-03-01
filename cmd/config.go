package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/CaptainCore/captaincore/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration commands",
}

var configFetchCmd = &cobra.Command{
	Use:   "fetch [<section>] [<key>]",
	Short: "Fetch configuration values",
	Long:  "Fetch configuration KEY=VALUE pairs for the current captain. Replaces configs.php fetch.",
	Run: func(cmd *cobra.Command, args []string) {
		configs, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		// If section and key provided: fetch specific value
		if len(args) >= 2 {
			val, err := config.FetchSpecificKey(configs, captainID, args[0], args[1])
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
			}
			fmt.Print(val)
			return
		}

		// If only section provided: fetch entire section as JSON
		if len(args) == 1 {
			val, err := config.FetchSection(configs, captainID, args[0])
			if err != nil {
				fmt.Fprint(os.Stderr, err.Error())
				os.Exit(1)
			}
			fmt.Print(val)
			return
		}

		// Default: output all KEY=VALUE pairs
		output, err := config.FetchKeyValues(configs, captainID)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		fmt.Print(output)
	},
}

var configFetchCaptainIDsCmd = &cobra.Command{
	Use:   "fetch-captain-ids",
	Short: "Fetch all captain IDs",
	Long:  "Returns space-separated captain IDs. Replaces configs.php fetch-captain-ids.",
	Run: func(cmd *cobra.Command, args []string) {
		configs, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(config.FetchCaptainIDs(configs))
	},
}

var configFromApiCmd = &cobra.Command{
	Use:   "from-api",
	Short: "Fetch config field from API",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, configFromApiNative)
	},
}

// configFromApiNative implements `captaincore config from-api --field=<field>` natively in Go.
func configFromApiNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	baseURL := getVarString(captain, "captaincore_gui")
	if baseURL == "" {
		fmt.Println("Error: captaincore_gui not configured.")
		return
	}

	url := baseURL + "/wp-json/captaincore/v1/client"

	// Build HTTP client with optional TLS skip
	client := &http.Client{}
	if system.CaptainCoreDev != "" && system.CaptainCoreDev != "false" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("Error fetching from API: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	var response map[string]interface{}
	if json.Unmarshal(body, &response) != nil {
		fmt.Print(strings.TrimSpace(string(body)))
		return
	}

	if flagField != "" {
		if val, ok := response[flagField]; ok {
			fmt.Print(strings.TrimSpace(fmt.Sprint(val)))
		}
		return
	}

	result, _ := json.MarshalIndent(response, "", "    ")
	fmt.Print(string(result))
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configFetchCmd)
	configCmd.AddCommand(configFetchCaptainIDsCmd)
	configCmd.AddCommand(configFromApiCmd)
	configFromApiCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
}
