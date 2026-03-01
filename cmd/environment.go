package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
)

var environmentCmd = &cobra.Command{
	Use:   "environment",
	Short: "Environment commands",
}

var listEnvironmentCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List environments",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, environmentListNative)
	},
}

func environmentListNative(cmd *cobra.Command, args []string) {
	site := args[0]

	// Look up site by ID or name
	var siteRecord *models.Site
	var err error
	if id, parseErr := strconv.ParseUint(site, 10, 64); parseErr == nil {
		siteRecord, err = models.GetSiteByID(uint(id))
	} else {
		siteRecord, err = models.GetSiteByName(site)
	}
	if err != nil || siteRecord == nil {
		fmt.Printf("Error: Site '%s' not found.", site)
		return
	}

	environments, err := models.FindEnvironmentsBySiteID(siteRecord.SiteID)
	if err != nil {
		return
	}

	var names []string
	for _, env := range environments {
		names = append(names, strings.ToLower(env.Environment))
	}
	fmt.Print(strings.Join(names, " "))
}

func init() {
	rootCmd.AddCommand(environmentCmd)
	environmentCmd.AddCommand(listEnvironmentCmd)
}
