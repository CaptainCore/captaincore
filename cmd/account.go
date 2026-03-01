package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account <site>",
	Short: "Account commands",
}

var accountSync = &cobra.Command{
	Use:   "sync <account>",
	Short: "Syncs account",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <account> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, accountSyncNative)
	},
}

var accountDeleteCmd = &cobra.Command{
	Use:   "delete <account>",
	Short: "Deletes an account",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <account> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, accountDeleteNative)
	},
}

func accountSyncNative(cmd *cobra.Command, args []string) {
	accountID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid account_id '%s'\n", args[0])
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	client := newAPIClient(system, captain)
	resp, err := client.PostAccountGetRaw(uint(accountID))
	if err != nil {
		fmt.Printf("Something went wrong: %s\n", err)
		return
	}

	if flagDebug {
		var pretty interface{}
		if json.Unmarshal(resp, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "    ")
			fmt.Println(string(out))
		}
		return
	}

	var result struct {
		Account json.RawMessage `json:"account"`
		Domains []json.RawMessage `json:"domains"`
		Sites   []json.RawMessage `json:"sites"`
		Users   []json.RawMessage `json:"users"`
	}
	if json.Unmarshal(resp, &result) != nil {
		fmt.Println("Error: Invalid API response")
		return
	}

	// Upsert account
	var accountData models.Account
	if json.Unmarshal(result.Account, &accountData) != nil {
		fmt.Println("Error: Could not parse account data")
		return
	}

	existing, _ := models.GetAccountByID(accountData.AccountID)
	if existing == nil {
		fmt.Printf("Inserting account #%d\n", accountData.AccountID)
	} else {
		fmt.Printf("Updating account #%d\n", accountData.AccountID)
	}
	models.UpsertAccount(accountData)

	// Upsert domains
	for _, domainRaw := range result.Domains {
		var domainData models.AccountDomain
		if json.Unmarshal(domainRaw, &domainData) != nil {
			continue
		}
		var existingDomain models.AccountDomain
		dbResult := models.DB.Where("account_domain_id = ?", domainData.AccountDomainID).First(&existingDomain)
		if dbResult.Error != nil {
			fmt.Printf("Inserting account_domain #%d\n", domainData.AccountDomainID)
		} else {
			fmt.Printf("Updating account_domain #%d\n", domainData.AccountDomainID)
		}
		models.UpsertAccountDomain(domainData)
	}

	// Upsert sites (account_site records)
	for _, siteRaw := range result.Sites {
		var siteData models.AccountSite
		if json.Unmarshal(siteRaw, &siteData) != nil {
			continue
		}
		var existingSite models.AccountSite
		dbResult := models.DB.Where("account_site_id = ?", siteData.AccountSiteID).First(&existingSite)
		if dbResult.Error != nil {
			fmt.Printf("Inserting account_site #%d\n", siteData.AccountSiteID)
		} else {
			fmt.Printf("Updating account_site #%d\n", siteData.AccountSiteID)
		}
		models.UpsertAccountSite(siteData)
	}

	// Upsert users
	for _, userRaw := range result.Users {
		var userData models.AccountUser
		if json.Unmarshal(userRaw, &userData) != nil {
			continue
		}
		var existingUser models.AccountUser
		dbResult := models.DB.Where("account_user_id = ?", userData.AccountUserID).First(&existingUser)
		if dbResult.Error != nil {
			fmt.Printf("Inserting account_user #%d\n", userData.AccountUserID)
		} else {
			fmt.Printf("Updating account_user #%d\n", userData.AccountUserID)
		}
		models.UpsertAccountUser(userData)
	}
}

func accountDeleteNative(cmd *cobra.Command, args []string) {
	accountID, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		fmt.Printf("Error: Invalid account_id '%s'\n", args[0])
		return
	}

	account, _ := models.GetAccountByID(uint(accountID))
	if account == nil {
		return
	}

	models.DeleteAccountByID(uint(accountID))
}

func init() {
	rootCmd.AddCommand(accountCmd)
	accountCmd.AddCommand(accountSync)
	accountCmd.AddCommand(accountDeleteCmd)
	accountSync.Flags().BoolVarP(&flagDebug, "debug", "d", false, "Debug response")
}
