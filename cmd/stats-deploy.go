package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var statsDeployCmd = &cobra.Command{
	Use:   "stats-deploy <site>",
	Short: "Deploys Fathom tracker to a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, statsDeployNative)
	},
}

// statsDeployNative implements `captaincore stats-deploy <site>` natively in Go.
func statsDeployNative(cmd *cobra.Command, args []string) {
	colorRed := "\033[31m"
	colorNormal := "\033[39m"

	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Site '%s' not found.\n", colorRed, colorNormal, sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment not found.\n", colorRed, colorNormal)
		return
	}

	if env.Address == "" {
		fmt.Fprintf(os.Stderr, "%sError:%s Environment not found for '%s'.\n", colorRed, colorNormal, site.Name)
		return
	}

	if env.Protocol != "sftp" {
		fmt.Fprintf(os.Stderr, "%sError:%s SSH not supported (Protocol is %s).", colorRed, colorNormal, env.Protocol)
		return
	}

	_, _, captain, err := loadCaptainConfig()
	if err != nil || captain == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Determine wp_content path
	siteDetails := site.ParseDetails()
	wpContent := "wp-content"
	if siteDetails.EnvironmentVars != nil && string(siteDetails.EnvironmentVars) != "" && string(siteDetails.EnvironmentVars) != "null" {
		var envVars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal(siteDetails.EnvironmentVars, &envVars) == nil {
			for _, item := range envVars {
				if item.Key == "STACKED_ID" || item.Key == "STACKED_SITE_ID" {
					wpContent = "content/" + item.Value
				}
			}
		}
	}

	// Build fathom arguments
	envDetails := env.ParseDetails()
	fathomAnalytics := "[]"
	if envDetails.Fathom != nil && string(envDetails.Fathom) != "null" && string(envDetails.Fathom) != "" {
		fathomAnalytics = string(envDetails.Fathom)
	}

	trackerURL := getVarString(captain, "captaincore_tracker_url")
	brandingAuthor := getVarString(captain, "captaincore_branding_author")
	brandingAuthorURI := getVarString(captain, "captaincore_branding_author_uri")
	brandingSlug := getVarString(captain, "captaincore_branding_slug")

	fathomArguments := fmt.Sprintf("id=%s\ntracker_url=%s\nbranding_author=%s\nbranding_author_uri=%s\nbranding_slug=%s",
		fathomAnalytics, trackerURL, brandingAuthor, brandingAuthorURI, brandingSlug)
	fathomArgumentsB64 := base64.StdEncoding.EncodeToString([]byte(fathomArguments))

	siteEnvArg := fmt.Sprintf("%s-%s", site.Site, sa.Environment)
	sshCmd := exec.Command("captaincore", "ssh", siteEnvArg,
		"--script=deploy-fathom",
		"--",
		"--wp_content="+wpContent,
		"--fathom_arguments="+fathomArgumentsB64)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	sshCmd.Run()
}

func init() {
	rootCmd.AddCommand(statsDeployCmd)
}
