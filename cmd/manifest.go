package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var manifestGenerateCmd = &cobra.Command{
	Use:   "manifest-generate",
	Short: "Generate manifest.json with site metrics",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, manifestGenerateNative)
	},
}

// manifestGenerateNative implements `captaincore manifest-generate` natively in Go.
func manifestGenerateNative(cmd *cobra.Command, args []string) {
	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Query all environments for active sites
	var envs []models.Environment
	models.DB.Joins("INNER JOIN captaincore_sites ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active").Find(&envs)

	// Track unique site IDs for count
	siteIDs := make(map[uint]bool)
	var totalSiteStorage int64
	var quicksaveCount int64
	var quicksaveStorage int64

	for _, env := range envs {
		siteIDs[env.SiteID] = true

		// Parse storage as int64
		if env.Storage != "" {
			if s, err := strconv.ParseInt(env.Storage, 10, 64); err == nil {
				totalSiteStorage += s
			}
		}

		// Parse details JSON to extract quicksave_usage
		if env.Details != "" {
			var details map[string]interface{}
			if json.Unmarshal([]byte(env.Details), &details) == nil {
				if qsUsage, ok := details["quicksave_usage"]; ok {
					if usage, ok := qsUsage.(map[string]interface{}); ok {
						// Count can be string or number
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
						// Storage can be string or number
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

	manifest := map[string]interface{}{
		"sites": map[string]interface{}{
			"count":   len(siteIDs),
			"storage": totalSiteStorage,
		},
		"quicksaves": map[string]interface{}{
			"count":   quicksaveCount,
			"storage": quicksaveStorage,
		},
		"storage": totalStorage,
	}

	result, _ := json.MarshalIndent(manifest, "", "    ")
	fmt.Println(string(result))

	manifestPath := filepath.Join(system.Path, "manifest.json")
	os.WriteFile(manifestPath, result, 0644)
}

func init() {
	rootCmd.AddCommand(manifestGenerateCmd)
}
