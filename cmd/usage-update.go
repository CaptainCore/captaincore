package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var usageUpdateCmd = &cobra.Command{
	Use:   "usage-update <site>",
	Short: "Generates usage stats for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, usageUpdateNative)
	},
}

func usageUpdateNative(cmd *cobra.Command, args []string) {
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

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)

	// Fetch folder size
	sizeCmd := exec.Command("captaincore", "ssh", siteEnvArg, "--script=fetch-folder-size", "--captain-id="+captainID)
	sizeOut, _ := sizeCmd.Output()
	storage := strings.TrimSpace(string(sizeOut))

	// Fetch visits
	visitsCmd := exec.Command("captaincore", "stats", siteEnvArg, "--captain-id="+captainID)
	visitsOut, _ := visitsCmd.Output()
	visits := strings.TrimSpace(string(visitsOut))

	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Update environment directly
	models.DB.Model(&models.Environment{}).Where("environment_id = ?", env.EnvironmentID).Updates(map[string]interface{}{
		"storage":    storage,
		"visits":     visits,
		"updated_at": timeNow,
	})

	if system.CaptainCoreStandby == "true" {
		fmt.Printf("Standby mode, local update only: {\"environment_id\":%d,\"storage\":\"%s\",\"visits\":\"%s\",\"updated_at\":\"%s\"}\n",
			env.EnvironmentID, storage, visits, timeNow)
		return
	}

	// Post to CaptainCore API
	client := newAPIClient(system, captain)
	envUpdate := map[string]interface{}{
		"environment_id": env.EnvironmentID,
		"storage":        storage,
		"visits":         visits,
		"updated_at":     timeNow,
	}
	resp, err := client.Post("usage-update", map[string]interface{}{
		"site_id": site.SiteID,
		"data":    envUpdate,
	})
	if err == nil {
		fmt.Print(string(resp))
	}
}

func init() {
	rootCmd.AddCommand(usageUpdateCmd)
}
