package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/CaptainCore/captaincore/apiclient"
	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migration commands",
}

var migrateWPToSQLiteCmd = &cobra.Command{
	Use:   "wp-to-sqlite",
	Short: "Migrate data from WordPress API to local SQLite database",
	Long: `One-time migration command that:
1. Reads site/environment/account data from the WordPress API
2. Inserts into the new SQLite database via GORM
3. Verifies row counts`,
	Run: func(cmd *cobra.Command, args []string) {
		if !ensureDB() {
			fmt.Fprintln(os.Stderr, "Error: Database not initialized")
			os.Exit(1)
		}

		configs, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		system := configs.GetSystem()
		captain := configs.GetCaptain(captainID)
		if captain == nil {
			fmt.Fprintf(os.Stderr, "Error: Captain %s not found in config\n", captainID)
			os.Exit(1)
		}

		apiURL := ""
		if captain.Vars != nil {
			if raw, ok := captain.Vars["captaincore_api"]; ok {
				json.Unmarshal(raw, &apiURL)
			}
		}
		if apiURL == "" {
			fmt.Fprintln(os.Stderr, "Error: captaincore_api not found in config vars")
			os.Exit(1)
		}

		token := captain.Keys["token"]
		skipSSL := system != nil && system.CaptainCoreDev == "true"
		client := apiclient.NewClient(apiURL, token, skipSSL)

		fmt.Println("Starting migration from WordPress API to SQLite...")

		// Fetch all site IDs from config vars
		websitesRaw, hasWebsites := captain.Vars["websites"]
		if !hasWebsites {
			fmt.Fprintln(os.Stderr, "Error: No websites found in config vars")
			os.Exit(1)
		}

		var websitesStr string
		json.Unmarshal(websitesRaw, &websitesStr)

		// The websites var contains space-separated site slugs, but we need site IDs.
		// We'll need to sync sites one at a time using site-get-raw.
		// For migration, we'll ask the API for site list first.
		fmt.Println("Fetching site data from API...")

		// Try fetching raw site data for each known site via API
		migrateSiteCount := 0
		migrateEnvCount := 0
		migrateAccountSiteCount := 0

		// First, get existing sites from the API by iterating through site IDs
		// We'll use a generous range or fetch from the sites list
		resp, err := client.Post("sites-list-raw", map[string]interface{}{})
		if err != nil {
			fmt.Printf("Note: sites-list-raw not available (%v), falling back to individual sync\n", err)
			fmt.Println("To migrate, run 'captaincore site sync <site-id>' for each site first,")
			fmt.Println("then re-run this migration command.")
			fmt.Println("")
			fmt.Println("Alternatively, provide site IDs as arguments:")
			fmt.Println("  captaincore migrate wp-to-sqlite 1 2 3 4 5")
			if len(args) == 0 {
				return
			}
		}

		type SiteListRaw struct {
			SiteID uint `json:"site_id"`
		}

		var siteIDs []uint

		if resp != nil {
			// Try to parse as array of site IDs
			var siteList []SiteListRaw
			if err := json.Unmarshal(resp, &siteList); err == nil {
				for _, s := range siteList {
					siteIDs = append(siteIDs, s.SiteID)
				}
			}
		}

		// Also accept site IDs from command args
		for _, arg := range args {
			var id uint
			if _, err := fmt.Sscanf(arg, "%d", &id); err == nil && id > 0 {
				siteIDs = append(siteIDs, id)
			}
		}

		// Deduplicate
		seen := make(map[uint]bool)
		var uniqueIDs []uint
		for _, id := range siteIDs {
			if !seen[id] {
				seen[id] = true
				uniqueIDs = append(uniqueIDs, id)
			}
		}
		siteIDs = uniqueIDs

		if len(siteIDs) == 0 {
			fmt.Println("No site IDs found to migrate.")
			return
		}

		fmt.Printf("Found %d sites to migrate\n", len(siteIDs))

		for _, siteID := range siteIDs {
			resp, err := client.PostSiteGetRaw(siteID)
			if err != nil {
				fmt.Printf("  Warning: Failed to fetch site %d: %v\n", siteID, err)
				continue
			}

			var wrapper struct {
				Site json.RawMessage `json:"site"`
			}
			// Try parsing as wrapped response first
			if err := json.Unmarshal(resp, &wrapper); err != nil || wrapper.Site == nil {
				// Try as direct site object
				wrapper.Site = resp
			}

			// Parse site
			var site models.Site
			if err := json.Unmarshal(wrapper.Site, &site); err != nil {
				fmt.Printf("  Warning: Failed to parse site %d: %v\n", siteID, err)
				continue
			}

			// Parse environments and shared_with from within site data
			var siteNested struct {
				Environments []models.Environment `json:"environments"`
				SharedWith   []models.AccountSite `json:"shared_with"`
			}
			json.Unmarshal(wrapper.Site, &siteNested)

			// Upsert site
			var existing models.Site
			if err := models.DB.Where("site_id = ?", site.SiteID).First(&existing).Error; err != nil {
				models.DB.Create(&site)
				fmt.Printf("  Added site #%d (%s)\n", site.SiteID, site.Name)
			} else {
				models.DB.Where("site_id = ?", site.SiteID).Updates(&site)
				fmt.Printf("  Updated site #%d (%s)\n", site.SiteID, site.Name)
			}
			migrateSiteCount++

			// Upsert environments
			for _, env := range siteNested.Environments {
				var existingEnv models.Environment
				if err := models.DB.Where("environment_id = ?", env.EnvironmentID).First(&existingEnv).Error; err != nil {
					models.DB.Create(&env)
				} else {
					models.DB.Where("environment_id = ?", env.EnvironmentID).Updates(&env)
				}
				migrateEnvCount++
			}

			// Upsert shared_with (account_site records)
			for _, as := range siteNested.SharedWith {
				var existingAS models.AccountSite
				if err := models.DB.Where("account_site_id = ?", as.AccountSiteID).First(&existingAS).Error; err != nil {
					models.DB.Create(&as)
				} else {
					models.DB.Where("account_site_id = ?", as.AccountSiteID).Updates(&as)
				}
				migrateAccountSiteCount++
			}
		}

		// Migrate providers
		fmt.Println("")
		fmt.Println("Syncing providers...")
		migrateProviderCount := 0
		provResp, provErr := client.PostProvidersListRaw()
		if provErr != nil {
			fmt.Printf("  Note: providers-list-raw not available (%v), skipping provider migration\n", provErr)
		} else {
			var apiResp struct {
				Providers []models.Provider `json:"providers"`
			}
			if err := json.Unmarshal(provResp, &apiResp); err != nil {
				fmt.Printf("  Warning: Failed to parse providers response: %v\n", err)
			} else {
				for _, rp := range apiResp.Providers {
					var existing models.Provider
					if err := models.DB.Where("provider_id = ?", rp.ProviderID).First(&existing).Error; err != nil {
						models.DB.Create(&rp)
						fmt.Printf("  Added provider #%d (%s)\n", rp.ProviderID, rp.Name)
					} else {
						models.DB.Where("provider_id = ?", rp.ProviderID).Updates(&rp)
						fmt.Printf("  Updated provider #%d (%s)\n", rp.ProviderID, rp.Name)
					}
					migrateProviderCount++
				}
			}
		}

		// Print summary
		fmt.Println("")
		fmt.Println("Migration complete!")
		fmt.Printf("  Sites:         %d migrated\n", migrateSiteCount)
		fmt.Printf("  Environments:  %d migrated\n", migrateEnvCount)
		fmt.Printf("  Account-Sites: %d migrated\n", migrateAccountSiteCount)
		fmt.Printf("  Providers:     %d migrated\n", migrateProviderCount)

		// Verify row counts
		var siteCount, envCount, asCount, provCount int64
		models.DB.Model(&models.Site{}).Count(&siteCount)
		models.DB.Model(&models.Environment{}).Count(&envCount)
		models.DB.Model(&models.AccountSite{}).Count(&asCount)
		models.DB.Model(&models.Provider{}).Count(&provCount)

		fmt.Println("")
		fmt.Println("Database totals:")
		fmt.Printf("  Sites:         %d\n", siteCount)
		fmt.Printf("  Environments:  %d\n", envCount)
		fmt.Printf("  Account-Sites: %d\n", asCount)
		fmt.Printf("  Providers:     %d\n", provCount)
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateWPToSQLiteCmd)
}
