package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/CaptainCore/captaincore/models"
	"github.com/CaptainCore/captaincore/version"
	"github.com/spf13/cobra"
)

var flagInfoJSON bool

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display CaptainCore system information",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, infoNative)
	},
}

// infoNative implements `captaincore info` natively in Go.
func infoNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// CLI version info
	cliVersion := version.Version
	goVersion := runtime.Version()
	platform := runtime.GOOS + "/" + runtime.GOARCH

	// Manager and API URLs
	managerURL := getVarString(captain, "captaincore_gui")
	apiURL := getVarString(captain, "captaincore_api")

	// Site count (active only)
	var siteCount int64
	models.DB.Model(&models.Site{}).Where("status = ?", "active").Count(&siteCount)

	// Account count
	var accountCount int64
	models.DB.Model(&models.Account{}).Count(&accountCount)

	// Environment count (joined to active sites)
	var envCount int64
	models.DB.Model(&models.Environment{}).
		Joins("INNER JOIN captaincore_sites ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active").Count(&envCount)

	// Domain count
	var domainCount int64
	models.DB.Model(&models.Domain{}).Count(&domainCount)

	// Provider count
	var providerCount int64
	models.DB.Model(&models.Provider{}).Count(&providerCount)

	// Storage metrics (reuse manifest-generate aggregation logic)
	var envs []models.Environment
	models.DB.Joins("INNER JOIN captaincore_sites ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active").Find(&envs)

	var totalSiteStorage int64
	var quicksaveCount int64
	var quicksaveStorage int64

	for _, env := range envs {
		if env.Storage != "" {
			if s, err := strconv.ParseInt(env.Storage, 10, 64); err == nil {
				totalSiteStorage += s
			}
		}
		if env.Details != "" {
			var details map[string]interface{}
			if json.Unmarshal([]byte(env.Details), &details) == nil {
				if qsUsage, ok := details["quicksave_usage"]; ok {
					if usage, ok := qsUsage.(map[string]interface{}); ok {
						if countVal, ok := usage["count"]; ok {
							switch v := countVal.(type) {
							case string:
								if n, err := strconv.ParseInt(v, 10, 64); err == nil {
									quicksaveCount += n
								}
							case float64:
								quicksaveCount += int64(v)
							}
						}
						if storageVal, ok := usage["storage"]; ok {
							switch v := storageVal.(type) {
							case string:
								if n, err := strconv.ParseInt(v, 10, 64); err == nil {
									quicksaveStorage += n
								}
							case float64:
								quicksaveStorage += int64(v)
							}
						}
					}
				}
			}
		}
	}

	totalStorage := totalSiteStorage + quicksaveStorage

	// Database file size
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".captaincore", "data", "captaincore.db")
	dbDisplayPath := filepath.Join("~/.captaincore", "data", "captaincore.db")
	var dbSize int64
	if fi, err := os.Stat(dbPath); err == nil {
		dbSize = fi.Size()
	}

	// Data path
	dataPath := system.Path

	if flagInfoJSON {
		data := map[string]interface{}{
			"version":      cliVersion,
			"go":           goVersion,
			"platform":     platform,
			"manager":      managerURL,
			"api":          apiURL,
			"captain_id":   captainID,
			"sites":        siteCount,
			"providers":    providerCount,
			"accounts":     accountCount,
			"environments": envCount,
			"domains":      domainCount,
			"storage": map[string]interface{}{
				"total":      totalStorage,
				"sites":      totalSiteStorage,
				"quicksaves": quicksaveStorage,
			},
			"quicksaves":    quicksaveCount,
			"database_path": dbPath,
			"database_size": dbSize,
			"data_path":     dataPath,
		}
		result, _ := json.MarshalIndent(data, "", "    ")
		fmt.Println(string(result))
		return
	}

	// Formatted text output
	fmt.Printf("CaptainCore CLI v%s (%s %s)\n", cliVersion, goVersion, platform)
	fmt.Println()
	fmt.Printf("Manager:      %s\n", managerURL)
	fmt.Printf("API:          %s\n", apiURL)
	fmt.Printf("Captain ID:   %s\n", captainID)
	fmt.Println()
	fmt.Printf("Sites:        %s (%d providers)\n", formatNumber(int(siteCount)), providerCount)
	fmt.Printf("Accounts:     %s\n", formatNumber(int(accountCount)))
	fmt.Printf("Environments: %s\n", formatNumber(int(envCount)))
	fmt.Printf("Domains:      %s\n", formatNumber(int(domainCount)))
	fmt.Println()
	fmt.Printf("Storage:      %s (sites: %s, quicksaves: %s)\n", formatBytes(strconv.FormatInt(totalStorage, 10)), formatBytes(strconv.FormatInt(totalSiteStorage, 10)), formatBytes(strconv.FormatInt(quicksaveStorage, 10)))
	fmt.Printf("Quicksaves:   %s\n", formatNumber(int(quicksaveCount)))
	fmt.Println()
	fmt.Printf("Database:     %s (%s)\n", dbDisplayPath, formatBytes(strconv.FormatInt(dbSize, 10)))
	fmt.Printf("Data path:    %s\n", dataPath)
}

func init() {
	rootCmd.AddCommand(infoCmd)
	infoCmd.Flags().BoolVar(&flagInfoJSON, "json", false, "Output as JSON")
}
