package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var scanErrorsCmd = &cobra.Command{
	Use:   "scan-errors <site>",
	Short: "Scans for Javascript errors on a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, scanErrorsNative)
	},
}

func scanErrorsNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	if env.HomeURL == "" {
		return
	}

	// Run lighthouse scan
	lighthouseCmd := exec.Command("bash", "-c", fmt.Sprintf(
		`lighthouse %s --only-audits=errors-in-console --chrome-flags="--headless" --output=json --quiet`,
		env.HomeURL))
	output, _ := lighthouseCmd.Output()
	responseStr := strings.TrimSpace(string(output))

	if responseStr == "" {
		return
	}

	var results struct {
		Audits struct {
			ErrorsInConsole struct {
				Details struct {
					Items json.RawMessage `json:"items"`
				} `json:"details"`
			} `json:"errors-in-console"`
		} `json:"audits"`
	}

	if json.Unmarshal([]byte(responseStr), &results) != nil {
		fmt.Println("Check not valid format")
		return
	}

	// Check if errors exist
	items := results.Audits.ErrorsInConsole.Details.Items
	var itemsList []interface{}
	if items != nil {
		json.Unmarshal(items, &itemsList)
	}

	if len(itemsList) > 0 {
		fmt.Printf("Detected %d errors on %s\n", len(itemsList), env.HomeURL)
		prettyItems, _ := json.MarshalIndent(itemsList, "", "    ")
		fmt.Println(string(prettyItems))

		// Update environment details
		updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
			"console_errors": itemsList,
		}, system, captain)

		// Update site details
		updateSiteDetails(site.SiteID, map[string]interface{}{
			"console_errors": itemsList,
		}, system, captain)
	} else {
		// No errors — check if we previously had errors and need to clear them
		envDetails := env.ParseDetails()
		if envDetails.ConsoleErrors != nil && string(envDetails.ConsoleErrors) != "null" && string(envDetails.ConsoleErrors) != "" {
			// Remove console_errors
			updateEnvironmentDetails(env.EnvironmentID, site.SiteID, map[string]interface{}{
				"console_errors": nil,
			}, system, captain)
			updateSiteDetails(site.SiteID, map[string]interface{}{
				"console_errors": nil,
			}, system, captain)
		}
	}
}

func init() {
	rootCmd.AddCommand(scanErrorsCmd)
}
