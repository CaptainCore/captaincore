package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CaptainCore/captaincore/models"
	"github.com/CaptainCore/captaincore/providers"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Provider commands",
}

var providerAddCmd = &cobra.Command{
	Use:   "add <name> <slug>",
	Short: "Add a hosting provider connection",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <name> <slug> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerAddNative)
	},
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List providers",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerListNative)
	},
}

var providerUpdateCmd = &cobra.Command{
	Use:   "update <provider-id>",
	Short: "Update a provider",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <provider-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerUpdateNative)
	},
}

var providerDeleteCmd = &cobra.Command{
	Use:   "delete <provider-id>",
	Short: "Delete a provider",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <provider-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerDeleteNative)
	},
}

var providerSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync providers from CaptainCore Manager",
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerSyncNative)
	},
}

var providerRemoteSitesCmd = &cobra.Command{
	Use:   "remote-sites <provider-id>",
	Short: "Fetch and display sites from provider API",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <provider-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerRemoteSitesNative)
	},
}

var providerImportCmd = &cobra.Command{
	Use:   "import <provider-id>",
	Short: "Bulk import sites from a provider",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <provider-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, providerImportNative)
	},
}

// providerAddNative implements `captaincore provider add <name> <slug>`.
func providerAddNative(cmd *cobra.Command, args []string) {
	name := args[0]
	slug := args[1]

	// Validate provider slug
	if _, err := providers.Get(slug); err != nil {
		fmt.Printf("Error: Unknown provider '%s'. Available providers: %s\n", slug, strings.Join(providers.All(), ", "))
		return
	}

	// Validate credentials JSON
	if flagCredentials == "" {
		fmt.Println("Error: --credentials flag is required")
		return
	}
	var creds []map[string]string
	if err := json.Unmarshal([]byte(flagCredentials), &creds); err != nil {
		fmt.Printf("Error: Invalid credentials JSON: %v\n", err)
		return
	}

	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	p := models.Provider{
		Name:        name,
		Provider:    slug,
		Status:      "active",
		Credentials: flagCredentials,
		CreatedAt:   timeNow,
		UpdatedAt:   timeNow,
	}

	if err := models.DB.Create(&p).Error; err != nil {
		fmt.Printf("Error creating provider: %v\n", err)
		return
	}
	fmt.Printf("Provider '%s' (%s) added with ID %d\n", name, slug, p.ProviderID)
}

// providerListNative implements `captaincore provider list`.
func providerListNative(cmd *cobra.Command, args []string) {
	providersList, err := models.GetAllProviders()
	if err != nil {
		fmt.Println("Error fetching providers:", err)
		return
	}

	if providersList == nil {
		providersList = []models.Provider{}
	}

	type providerOutput struct {
		ProviderID uint   `json:"provider_id"`
		Name       string `json:"name"`
		Provider   string `json:"provider"`
		Status     string `json:"status"`
		UserID     uint   `json:"user_id"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}

	var output []providerOutput
	for _, p := range providersList {
		output = append(output, providerOutput{
			ProviderID: p.ProviderID,
			Name:       p.Name,
			Provider:   p.Provider,
			Status:     p.Status,
			UserID:     p.UserID,
			CreatedAt:  p.CreatedAt,
			UpdatedAt:  p.UpdatedAt,
		})
	}

	if output == nil {
		output = []providerOutput{}
	}

	result, _ := json.MarshalIndent(output, "", "    ")
	fmt.Println(string(result))
}

// providerUpdateNative implements `captaincore provider update <provider-id>`.
func providerUpdateNative(cmd *cobra.Command, args []string) {
	providerID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid provider-id '%s'\n", args[0])
		return
	}

	p, err := models.GetProviderByID(uint(providerID))
	if err != nil {
		fmt.Printf("Error: Provider #%d not found\n", providerID)
		return
	}

	if flagCredentials != "" {
		var creds []map[string]string
		if err := json.Unmarshal([]byte(flagCredentials), &creds); err != nil {
			fmt.Printf("Error: Invalid credentials JSON: %v\n", err)
			return
		}
		p.Credentials = flagCredentials
	}

	if flagStatus != "" {
		p.Status = flagStatus
	}

	p.UpdatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")

	if err := models.DB.Save(p).Error; err != nil {
		fmt.Printf("Error updating provider: %v\n", err)
		return
	}
	fmt.Printf("Provider #%d updated\n", providerID)
}

// providerDeleteNative implements `captaincore provider delete <provider-id>`.
func providerDeleteNative(cmd *cobra.Command, args []string) {
	providerID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid provider-id '%s'\n", args[0])
		return
	}

	if _, err := models.GetProviderByID(uint(providerID)); err != nil {
		fmt.Printf("Error: Provider #%d not found\n", providerID)
		return
	}

	if err := models.DeleteProviderByID(uint(providerID)); err != nil {
		fmt.Printf("Error deleting provider: %v\n", err)
		return
	}
	fmt.Printf("Provider #%d deleted\n", providerID)
}

// providerSyncNative implements `captaincore provider sync`.
// It fetches all providers from the CaptainCore Manager API and syncs them to the local SQLite DB.
func providerSyncNative(cmd *cobra.Command, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)
	resp, err := client.PostProvidersListRaw()
	if err != nil {
		fmt.Printf("Error fetching providers from API: %v\n", err)
		return
	}

	if flagDebug {
		var pretty interface{}
		if json.Unmarshal(resp, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "    ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(resp))
		}
		return
	}

	// Parse the wrapped API response: {"response": "...", "providers": [...]}
	var apiResp struct {
		Providers []models.Provider `json:"providers"`
	}
	if err := json.Unmarshal(resp, &apiResp); err != nil {
		fmt.Printf("Error parsing providers response: %v\n", err)
		return
	}
	remoteProviders := apiResp.Providers

	// Track which provider IDs came from the API
	remoteIDs := make(map[uint]bool)
	addedCount := 0
	updatedCount := 0

	for _, rp := range remoteProviders {
		remoteIDs[rp.ProviderID] = true
		existing, _ := models.GetProviderByID(rp.ProviderID)
		if existing == nil {
			if err := models.DB.Create(&rp).Error; err != nil {
				fmt.Printf("  Warning: Failed to add provider #%d (%s): %v\n", rp.ProviderID, rp.Name, err)
				continue
			}
			fmt.Printf("  Added provider #%d: %s (%s)\n", rp.ProviderID, rp.Name, rp.Provider)
			addedCount++
		} else {
			if err := models.DB.Model(existing).Updates(rp).Error; err != nil {
				fmt.Printf("  Warning: Failed to update provider #%d (%s): %v\n", rp.ProviderID, rp.Name, err)
				continue
			}
			fmt.Printf("  Updated provider #%d: %s (%s)\n", rp.ProviderID, rp.Name, rp.Provider)
			updatedCount++
		}
	}

	// Remove providers that no longer exist on the API side
	localProviders, _ := models.GetAllProviders()
	removedCount := 0
	for _, lp := range localProviders {
		if !remoteIDs[lp.ProviderID] {
			models.DeleteProviderByID(lp.ProviderID)
			fmt.Printf("  Removed provider #%d: %s (%s)\n", lp.ProviderID, lp.Name, lp.Provider)
			removedCount++
		}
	}

	fmt.Printf("Provider sync complete: %d added, %d updated, %d removed\n", addedCount, updatedCount, removedCount)
}

// providerRemoteSitesNative implements `captaincore provider remote-sites <provider-id>`.
func providerRemoteSitesNative(cmd *cobra.Command, args []string) {
	providerID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid provider-id '%s'\n", args[0])
		return
	}

	p, err := models.GetProviderByID(uint(providerID))
	if err != nil {
		fmt.Printf("Error: Provider #%d not found\n", providerID)
		return
	}

	hp, err := providers.Get(p.Provider)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	creds := p.GetCredentialsMap()
	sites, err := hp.FetchRemoteSites(creds)
	if err != nil {
		fmt.Printf("Error fetching remote sites: %v\n", err)
		return
	}

	if sites == nil {
		sites = []providers.RemoteSite{}
	}

	result, _ := json.MarshalIndent(sites, "", "    ")
	fmt.Println(string(result))
}

// providerImportNative implements `captaincore provider import <provider-id>`.
func providerImportNative(cmd *cobra.Command, args []string) {
	providerID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid provider-id '%s'\n", args[0])
		return
	}

	p, err := models.GetProviderByID(uint(providerID))
	if err != nil {
		fmt.Printf("Error: Provider #%d not found\n", providerID)
		return
	}

	hp, err := providers.Get(p.Provider)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if flagSiteIDs == "" {
		fmt.Println("Error: --site-ids flag is required (comma-separated remote site IDs)")
		return
	}

	siteIDs := strings.Split(flagSiteIDs, ",")
	creds := p.GetCredentialsMap()

	// Fetch all remote sites to filter by selected IDs
	remoteSites, err := hp.FetchRemoteSites(creds)
	if err != nil {
		fmt.Printf("Error fetching remote sites: %v\n", err)
		return
	}

	// Build lookup of selected remote site IDs
	selectedIDs := make(map[string]bool)
	for _, id := range siteIDs {
		selectedIDs[strings.TrimSpace(id)] = true
	}

	// Filter to selected sites
	var selectedSites []providers.RemoteSite
	for _, rs := range remoteSites {
		if selectedIDs[rs.RemoteID] {
			selectedSites = append(selectedSites, rs)
		}
	}

	if len(selectedSites) == 0 {
		fmt.Println("No matching remote sites found for the provided --site-ids")
		return
	}

	var importedSiteIDs []string
	timeNow := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, rs := range selectedSites {
		// Step 1: Duplicate check
		existing, _ := models.GetSiteByProviderSiteID(rs.RemoteID, p.Provider)
		if existing != nil {
			fmt.Printf("Skipping '%s' (remote_id=%s) — already exists as site #%d\n", rs.Name, rs.RemoteID, existing.SiteID)
			continue
		}

		// Wrap creation in a transaction
		txErr := models.DB.Transaction(func(tx *gorm.DB) error {
			// Step 2: Create Site record
			siteName := strings.ToLower(strings.ReplaceAll(rs.Domain, ".", "-"))
			defaultDetails, _ := json.Marshal(map[string]interface{}{
				"backup_settings": map[string]interface{}{
					"mode":     "direct",
					"interval": "daily",
					"active":   true,
				},
			})

			site := models.Site{
				Name:           rs.Name,
				Site:           siteName,
				Provider:       p.Provider,
				ProviderID:     fmt.Sprintf("%d", p.ProviderID),
				ProviderSiteID: rs.RemoteID,
				Status:         "active",
				Details:        string(defaultDetails),
				CreatedAt:      timeNow,
				UpdatedAt:      timeNow,
			}
			if err := tx.Create(&site).Error; err != nil {
				return fmt.Errorf("create site: %w", err)
			}
			fmt.Printf("Created site #%d: %s (%s)\n", site.SiteID, rs.Name, rs.Domain)

			// Step 3: Create AccountSite if --account-id provided
			if flagAccountID != "" {
				accountID, err := strconv.ParseUint(flagAccountID, 10, 64)
				if err == nil {
					as := models.AccountSite{
						AccountID: uint(accountID),
						SiteID:    site.SiteID,
						CreatedAt: timeNow,
						UpdatedAt: timeNow,
					}
					if err := tx.Create(&as).Error; err != nil {
						return fmt.Errorf("create account_site: %w", err)
					}
				}
			}

			// Step 4: Create Production Environment
			env := models.Environment{
				SiteID:      site.SiteID,
				Environment: "Production",
				CreatedAt:   timeNow,
				UpdatedAt:   timeNow,
			}
			if err := tx.Create(&env).Error; err != nil {
				return fmt.Errorf("create environment: %w", err)
			}

			// Step 5: Enrich with provider API data
			enriched, enrichErr := hp.EnrichSite(creds, rs)
			if enrichErr != nil {
				log.Printf("Warning: enrichment failed for '%s': %v (continuing)\n", rs.Name, enrichErr)
				importedSiteIDs = append(importedSiteIDs, fmt.Sprintf("%d", site.SiteID))
				return nil
			}

			// Update environment with enriched data
			env.Address = enriched.SSHAddress
			env.Username = enriched.SSHUsername
			env.Password = enriched.SSHPassword
			env.Port = enriched.SSHPort
			env.Protocol = "sftp"
			env.HomeDirectory = enriched.HomeDirectory
			env.HomeURL = enriched.HomeURL
			env.Core = enriched.WPVersion
			env.Visits = enriched.MonthlyVisits
			if err := tx.Save(&env).Error; err != nil {
				return fmt.Errorf("update environment: %w", err)
			}

			// Update site domain if enriched
			if enriched.Domain != "" {
				site.Site = strings.ToLower(strings.ReplaceAll(enriched.Domain, ".", "-"))
				tx.Model(&site).Update("site", site.Site)
			}

			importedSiteIDs = append(importedSiteIDs, fmt.Sprintf("%d", site.SiteID))
			return nil
		})

		if txErr != nil {
			fmt.Printf("Error importing '%s': %v\n", rs.Name, txErr)
		}
	}

	if len(importedSiteIDs) == 0 {
		fmt.Println("No new sites imported.")
		return
	}

	fmt.Printf("\nImported %d site(s): %s\n", len(importedSiteIDs), strings.Join(importedSiteIDs, ", "))

	// Post-import: run batch sync
	if flagUpdateExtras {
		batchIDs := strings.Join(importedSiteIDs, " ")
		syncCmd := exec.Command("captaincore", "site", "sync-batch", batchIDs, "--update-extras", "--captain-id="+captainID)
		syncCmd.Stdout = os.Stdout
		syncCmd.Stderr = os.Stderr
		if err := syncCmd.Run(); err != nil {
			log.Printf("Warning: batch sync failed: %v\n", err)
		}
	}
}

func init() {
	rootCmd.AddCommand(providerCmd)
	providerCmd.AddCommand(providerAddCmd)
	providerCmd.AddCommand(providerListCmd)
	providerCmd.AddCommand(providerSyncCmd)
	providerCmd.AddCommand(providerUpdateCmd)
	providerCmd.AddCommand(providerDeleteCmd)
	providerCmd.AddCommand(providerRemoteSitesCmd)
	providerCmd.AddCommand(providerImportCmd)

	providerAddCmd.Flags().StringVar(&flagCredentials, "credentials", "", "JSON array of credentials [{\"name\":\"api_key\",\"value\":\"xxx\"}]")
	providerSyncCmd.Flags().BoolVar(&flagDebug, "debug", false, "Debug response")
	providerUpdateCmd.Flags().StringVar(&flagCredentials, "credentials", "", "JSON array of credentials")
	providerUpdateCmd.Flags().StringVar(&flagStatus, "status", "", "Provider status")
	providerImportCmd.Flags().StringVar(&flagSiteIDs, "site-ids", "", "Comma-separated remote site IDs to import")
	providerImportCmd.Flags().StringVar(&flagAccountID, "account-id", "", "Account ID for imported sites")
	providerImportCmd.Flags().BoolVar(&flagUpdateExtras, "update-extras", false, "Run batch sync after import")
}
