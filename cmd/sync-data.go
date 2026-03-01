package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var flagSyncDataJSON bool

var syncDataCmd = &cobra.Command{
	Use:   "sync-data <site|target>",
	Short: "Sync website data for one or more sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Count non-flag targets
		targetCount := 0
		for _, arg := range args {
			if !strings.HasPrefix(arg, "--") {
				targetCount++
			}
		}
		// Multiple targets or bulk targets go through bash
		if targetCount > 1 || (len(args) > 0 && (strings.HasPrefix(args[0], "@production") || strings.HasPrefix(args[0], "@staging") || strings.HasPrefix(args[0], "@all"))) {
			resolveCommand(cmd, args)
			return
		}
		resolveNativeOrWP(cmd, args, syncDataNative)
	},
}

// syncDataNative implements `captaincore sync-data <site>` natively in Go.
func syncDataNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])

	if !flagSyncDataJSON {
		fmt.Printf("Syncing %s...\n", args[0])
	}

	// Fetch site details via `captaincore site get`
	siteGetCmd := exec.Command("captaincore", "site", "get", args[0], "--captain-id="+captainID)
	siteGetOutput, err := siteGetCmd.Output()
	if err != nil || len(siteGetOutput) == 0 {
		fmt.Printf("Error: Site '%s' not found.\n", args[0])
		return
	}

	var siteDetails struct {
		SiteID uint `json:"site_id"`
	}
	if json.Unmarshal(siteGetOutput, &siteDetails) != nil {
		fmt.Printf("Error: Site '%s' not found.\n", args[0])
		return
	}

	// Run fetch-site-data script
	if !flagSyncDataJSON {
		fmt.Print("Fetching site data via SSH... ")
	}
	sshCmd := exec.Command("captaincore", "ssh", args[0], "--script=fetch-site-data", "--captain-id="+captainID)
	sshOutput, err := sshCmd.Output()
	if err != nil || len(sshOutput) == 0 {
		if !flagSyncDataJSON {
			fmt.Println("failed")
		}
		return
	}
	if !flagSyncDataJSON {
		fmt.Println("done")
	}

	responses := strings.Split(string(sshOutput), "\n")

	// Find environment ID
	environments, err := models.FindEnvironmentsBySiteID(siteDetails.SiteID)
	if err != nil {
		return
	}
	var environmentID uint
	for _, env := range environments {
		if strings.EqualFold(env.Environment, sa.Environment) {
			environmentID = env.EnvironmentID
			break
		}
	}
	if environmentID == 0 {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		return
	}
	client := newAPIClient(system, captain)

	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Handle "WordPress not found" case
	if len(responses) > 0 && responses[0] == "WordPress not found" {
		environmentUpdate := map[string]interface{}{
			"environment_id": environmentID,
			"token":          "basic",
			"updated_at":     timeNow,
		}
		resp, err := client.Post("sync-data", map[string]interface{}{
			"site_id": siteDetails.SiteID,
			"data":    environmentUpdate,
		})
		if err == nil {
			if flagSyncDataJSON {
				fmt.Print(string(resp))
			} else {
				fmt.Println("WordPress not found")
			}
		}
		return
	}

	// Validate plugins and themes JSON
	if len(responses) < 14 {
		fmt.Println("Response not valid")
		return
	}

	var testJSON interface{}
	if json.Unmarshal([]byte(responses[0]), &testJSON) != nil {
		fmt.Println("Response not valid")
		return
	}
	if json.Unmarshal([]byte(responses[1]), &testJSON) != nil {
		fmt.Println("Response not valid")
		return
	}

	environmentUpdate := map[string]interface{}{
		"environment_id":        environmentID,
		"plugins":               responses[0],
		"themes":                responses[1],
		"core":                  responses[2],
		"home_url":              responses[3],
		"users":                 responses[4],
		"database_name":         responses[5],
		"database_username":     responses[6],
		"database_password":     responses[7],
		"core_verify_checksums": responses[8],
		"subsite_count":         responses[9],
		"php_memory":            responses[10],
		"token":                 responses[13],
		"updated_at":            timeNow,
	}

	// Load existing environment details and add extra fields
	envRecord, err := models.GetEnvironmentByID(environmentID)
	if err != nil {
		return
	}

	var details map[string]interface{}
	if envRecord.Details != "" {
		json.Unmarshal([]byte(envRecord.Details), &details)
	}
	if details == nil {
		details = make(map[string]interface{})
	}

	if len(responses) > 11 && responses[11] != "" {
		details["default_role"] = responses[11]
	}
	if len(responses) > 12 && responses[12] != "" {
		details["registration"] = responses[12]
	}
	if len(responses) > 14 && responses[14] != "" {
		var checksumDetails interface{}
		if json.Unmarshal([]byte(responses[14]), &checksumDetails) == nil {
			details["core_checksum_details"] = checksumDetails
		}
	}

	detailsJSON, _ := json.Marshal(details)
	environmentUpdate["details"] = string(detailsJSON)

	// Update environment in DB
	if !flagSyncDataJSON {
		fmt.Print("Updating local database... ")
	}
	updates := map[string]interface{}{
		"plugins":               responses[0],
		"themes":                responses[1],
		"core":                  responses[2],
		"home_url":              responses[3],
		"users":                 responses[4],
		"database_name":         responses[5],
		"database_username":     responses[6],
		"database_password":     responses[7],
		"core_verify_checksums": responses[8],
		"subsite_count":         responses[9],
		"php_memory":            responses[10],
		"token":                 responses[13],
		"details":               string(detailsJSON),
		"updated_at":            timeNow,
	}
	models.DB.Model(&models.Environment{}).Where("environment_id = ?", environmentID).Updates(updates)
	if !flagSyncDataJSON {
		fmt.Println("done")
	}

	// Post to API
	if !flagSyncDataJSON {
		fmt.Print("Syncing to API... ")
	}
	resp, err := client.Post("sync-data", map[string]interface{}{
		"site_id": siteDetails.SiteID,
		"data":    environmentUpdate,
	})
	if err == nil {
		if flagSyncDataJSON {
			fmt.Print(string(resp))
		} else {
			fmt.Println("done")
			syncDataPrintSummary(args[0], responses)
		}
	} else if !flagSyncDataJSON {
		fmt.Println("failed")
	}
}

// syncDataPrintSummary prints a human-readable summary of the synced data.
func syncDataPrintSummary(site string, responses []string) {
	homeURL := responses[3]
	core := responses[2]
	checksums := responses[8]
	phpMemory := responses[10]

	// Count plugins
	var plugins []interface{}
	json.Unmarshal([]byte(responses[0]), &plugins)

	// Count themes
	var themes []interface{}
	json.Unmarshal([]byte(responses[1]), &themes)

	// Count users
	var users []interface{}
	json.Unmarshal([]byte(responses[4]), &users)

	checksumStatus := "pass"
	if checksums == "0" {
		checksumStatus = "fail"
	}

	fmt.Printf("\n%s (%s)\n", site, homeURL)
	fmt.Printf("  WordPress %-8s Checksums %s\n", core, checksumStatus)
	fmt.Printf("  %s plugins, %s themes, %s users\n",
		formatNumber(len(plugins)),
		formatNumber(len(themes)),
		formatNumber(len(users)),
	)
	fmt.Printf("  PHP memory %s\n", phpMemory)
}

// formatNumber adds comma separators to a number (e.g. 3641 -> "3,641").
func formatNumber(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func init() {
	rootCmd.AddCommand(syncDataCmd)
	syncDataCmd.Flags().BoolVarP(&flagSkipScreenshot, "skip-screenshot", "c", false, "Skip screenshot")
	syncDataCmd.Flags().BoolVar(&flagSyncDataJSON, "json", false, "Output raw JSON response")
}
