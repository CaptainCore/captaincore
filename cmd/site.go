package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Site commands",
}

var siteDeployKeysCmd = &cobra.Command{
	Use:   "deploy-keys <site>",
	Short: "Deploy keys to a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var fetchTokenCmd = &cobra.Command{
	Use:   "fetch-token <site>",
	Short: " Fetch token for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <site>",
	Short: "Delete a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var getCmd = &cobra.Command{
	Use:   "get <site>",
	Short: "Get details about a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var listCmd = &cobra.Command{
	Use:     "list [<@target> [--provider=<provider>] [--filter=<theme|plugin|core>] [--filter-name=<name>] [--filter-version=<version>] [--filter-status=<active|inactive|dropin|must-use>] [--field=<field>]",
	Example: "captaincore site list production.updates-off --provider=kinsta",
	Short:   "List sites",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var keyGenerateCmd = &cobra.Command{
	Use:   "key-generate <site>",
	Short: "Generates SFTP/SSH Rclone configs",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var siteCopyProductionToStaging = &cobra.Command{
	Use:   "copy-to-staging <site>",
	Short: "Copy production to staging (Kinsta only)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}
var siteCopyStagingToProduction = &cobra.Command{
	Use:   "copy-to-production <site>",
	Short: "Copy staging to production (Kinsta only)",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var siteStatsGenerateCmd = &cobra.Command{
	Use:   "stats-generate <site>",
	Short: "Generates Fathom tracker",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var siteDeployDefaultsCmd = &cobra.Command{
	Use:   "deploy-defaults <site>",
	Short: "Deploy default plugins/themes/settings",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var siteFetchTokenCmd = &cobra.Command{
	Use:   "fetch-token <site>",
	Short: "Fetch token for site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var sitePrepareCmd = &cobra.Command{
	Use:   "prepare <site> [--skip-deployment]",
	Short: "Preps new site configurations",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var sshFailCmd = &cobra.Command{
	Use:   "ssh-fail <site>",
	Short: "Flag site with SSH failure",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommandWP(cmd, args)
	},
}

var syncSiteCmd = &cobra.Command{
	Use:   "sync <site-id>",
	Short: "Sync site details with CaptainCore CLI",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site-id> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(siteCmd)
	siteCmd.AddCommand(deleteCmd)
	siteCmd.AddCommand(getCmd)
	siteCmd.AddCommand(listCmd)
	siteCmd.AddCommand(keyGenerateCmd)
	siteCmd.AddCommand(sshFailCmd)
	siteCmd.AddCommand(siteCopyProductionToStaging)
	siteCmd.AddCommand(siteCopyStagingToProduction)
	siteCmd.AddCommand(siteFetchTokenCmd)
	siteCmd.AddCommand(sitePrepareCmd)
	siteCmd.AddCommand(siteDeployDefaultsCmd)
	siteCmd.AddCommand(siteDeployKeysCmd)
	siteCmd.AddCommand(siteStatsGenerateCmd)
	siteCmd.AddCommand(syncSiteCmd)
	getCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
	getCmd.Flags().BoolVarP(&flagBash, "bash", "", false, "Return bash format")
	siteStatsGenerateCmd.Flags().BoolVarP(&flagSkipAlreadyGenerated, "skip-already-generated", "", false, "Skips if already has tracking")
	siteDeployDefaultsCmd.Flags().BoolVarP(&flagGlobalOnly, "global-only", "", false, "Deploy global only configurations")
	syncSiteCmd.Flags().BoolVarP(&flagDebug, "debug", "", false, "Debug response")
	syncSiteCmd.Flags().BoolVarP(&flagUpdateExtras, "update-extras", "", false, "Runs prepare site, deploy global defaults and capture screenshot")
	siteCopyProductionToStaging.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	siteCopyStagingToProduction.Flags().StringVarP(&flagEmail, "email", "e", "", "Notify email address")
	listCmd.Flags().StringVarP(&flagProvider, "provider", "p", "", "Filter by host provider")
	listCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Filter by <theme|plugin|core>")
	listCmd.Flags().StringVarP(&flagFilterName, "filter-name", "n", "", "Filter name")
	listCmd.Flags().StringVarP(&flagFilterVersion, "filter-version", "v", "", "Filter version")
	listCmd.Flags().StringVarP(&flagFilterStatus, "filter-status", "s", "", "Filter by status <active|inactive|dropin|must-use>")
	listCmd.Flags().StringVarP(&flagField, "field", "", "", "Return certain field")
}
