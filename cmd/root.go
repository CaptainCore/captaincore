package cmd

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var flagDebug, flagSkipDB, flagSkipScreenshot, flagForce, flagBash, flagUpdateExtras, flagSkipRemote, flagFleet, flagInit, flagLabel, flagDryRun bool
var flagAll, flagHtml, flagPublic, flagSkipAlreadyGenerated, flagGlobalOnly, flagDeleteAfterSnapshot, flagCached, flagRepackUncompressed bool
var flagCode, flagCommand, flagFilter, flagFilterName, flagFilterVersion, flagFilterStatus, flagField, flagPage, flagRecipe, flagScript, flagProvider string
var captainID, cfgFile, flagTheme, flagPlugin, flagFile, flagLimit, flagEmail, flagName, flagLink, flagNotes, flagUserId, flagFormat, flagVersion, flagSkipIfRecent, flagSubject, flagStatus, flagAction string
var flagSearchField string
var flagCredentials, flagAccountID, flagSiteIDs string
var flagParallel, flagRetry int

var colorYellow = "\x1b[33;1m"
var colorGreen = "\x1b[32;1m"
var colorNormal = "\x1b[0m"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "captaincore",
	Run: func(cmd *cobra.Command, args []string) {
		if !ensureDB() || !dbHasData() {
			fmt.Println(colorYellow + "Getting Started:" + colorNormal + " Run " + colorGreen + "captaincore connect" + colorNormal + " to set up your CaptainCore CLI.")
			fmt.Println()
		}
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.SilenceErrors = true
	rootCmd.SetUsageTemplate(colorYellow + `Usage:` + colorNormal + `{{if .Runnable}}
` + colorGreen + `{{.UseLine}}` + colorNormal + `{{end}}{{if .HasAvailableSubCommands}}
  ` + colorGreen + `{{.CommandPath}}` + colorNormal + ` [command]{{end}}{{if gt (len .Aliases) 0}}

` + colorYellow + `Aliases:` + colorNormal + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + colorYellow + `Examples:` + colorNormal + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

` + colorYellow + `Available Commands:` + colorNormal + `{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  ` + colorGreen + `{{rpad .Name .NamePadding }}` + colorNormal + ` {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + colorYellow + `Flags:` + colorNormal + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + colorYellow + `Global Flags:` + colorNormal + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)
	rootCmd.PersistentFlags().StringVar(&captainID, "captain-id", "1", "Captain ID")
	rootCmd.PersistentFlags().BoolVar(&flagFleet, "fleet", false, "Fleet mode")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "~/.captaincore/config.json", "config file")
	rootCmd.PersistentFlags().BoolVar(&flagLabel, "label", false, "Print colored site name headers in bulk mode")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cli" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".cli")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

var dbOnce sync.Once
var dbInitErr error

// ensureDB lazily initializes the SQLite database on first use.
func ensureDB() bool {
	dbOnce.Do(func() {
		dbInitErr = models.InitDB()
	})
	return dbInitErr == nil && models.DB != nil
}

// resolveNativeOrWP routes to a native Go handler if the database is available
// AND populated, otherwise returns an error.
func resolveNativeOrWP(c *cobra.Command, args []string, native func(*cobra.Command, []string)) {
	if ensureDB() && dbHasData() {
		native(c, args)
		return
	}
	fmt.Println("Error: Database not available. Run 'captaincore connect' to set up your CaptainCore CLI.")
	os.Exit(1)
}

// dbHasData checks whether the SQLite database has been populated with site data.
func dbHasData() bool {
	var count int64
	models.DB.Table("captaincore_sites").Count(&count)
	return count > 0
}

// fetchCaptainIDsNative returns captain IDs using Go config instead of PHP.
func fetchCaptainIDsNative() ([]string, error) {
	configs, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	ids := config.FetchCaptainIDs(configs)
	return strings.Split(ids, " "), nil
}

func runCommand(c *cobra.Command, args []string) {
	command := c.CommandPath()
	command = strings.Replace(command, "captaincore ", "", -1)
	command = strings.Replace(command, " ", "/", -1)
	print(command)
	//data, _ := scriptFiles.ReadFile("scripts/" + command)
	//print(string(data))
	//fmt.Printf(data)
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func resolveCommand(c *cobra.Command, args []string) {
	var bulk bool
	dirname, err := os.UserHomeDir()
	env := os.Environ()
	target_count := 0

	for _, arg := range args {
		if strings.Contains(arg, "--") == false {
			target_count = target_count + 1
		}
	}

	_, args_check := os.LookupEnv("CAPTAINCORE_ARGS")
	if !args_check {
		env = append([]string{"CAPTAINCORE_ARGS=" + strings.Join(args, " ")}, env...)
	}

	if err != nil {
		log.Fatal(err)
	}

	if target_count > 0 && c.CommandPath() != "captaincore monitor" &&
		c.CommandPath() != "captaincore bulk" && c.CommandPath() != "captaincore site sync" && c.CommandPath() != "captaincore site sync-batch" && c.CommandPath() != "captaincore ssh-detect" && c.CommandPath() != "captaincore plugin-zip" && c.CommandPath() != "captaincore upload" &&
		c.CommandPath() != "captaincore backup get" && c.CommandPath() != "captaincore backup get-generate" && c.CommandPath() != "captaincore backup download" && c.CommandPath() != "captaincore backup show" && c.CommandPath() != "captaincore email-health send" && c.CommandPath() != "captaincore email-health response" && c.CommandPath() != "captaincore email-health generate" &&
		c.CommandPath() != "captaincore quicksave show-changes" && c.CommandPath() != "captaincore quicksave file-diff" && c.CommandPath() != "captaincore quicksave rollback" && c.CommandPath() != "captaincore quicksave get-generate" && c.CommandPath() != "captaincore quicksave get" &&
		c.CommandPath() != "captaincore update-log generate" && c.CommandPath() != "captaincore update-log list-generate" && c.CommandPath() != "captaincore update-log get" && c.CommandPath() != "captaincore capture generate" && c.CommandPath() != "captaincore capture scan" {
		if strings.HasPrefix(args[0], "@production") || strings.HasPrefix(args[0], "@staging") || strings.HasPrefix(args[0], "@all") || target_count > 1 {
			bulk = true
		}
	}

	if (c.CommandPath() == "captaincore backup generate" || c.CommandPath() == "captaincore backup check") && target_count > 1 {
		bulk = true
	}

	command := c.CommandPath()
	command = strings.Replace(command, "captaincore ", "", -1)
	command = strings.Replace(command, " ", "/", -1)

	// Bulk mode: use the native Go bulk runner
	if bulk {
		// Separate targets from any stray flags in args
		var targets []string
		for _, arg := range args {
			if !strings.HasPrefix(arg, "--") {
				targets = append(targets, arg)
			}
		}

		cfg := BulkConfig{
			Command:   command,
			Targets:   targets,
			Flags:     collectBulkFlags(),
			CaptainID: captainID,
			Parallel:  flagParallel,
			Label:     flagLabel,
			Debug:     flagDebug,
		}

		if flagFleet {
			captainIds, nativeErr := fetchCaptainIDsNative()
			if nativeErr != nil {
				log.Fatalf("Error fetching captain IDs: %s\n", nativeErr)
			}
			for _, fleetCaptainID := range captainIds {
				cfg.CaptainID = fleetCaptainID
				if err := runBulk(cfg); err != nil {
					log.Printf("Fleet bulk error (captain %s): %s\n", fleetCaptainID, err)
				}
			}
			return
		}

		if err := runBulk(cfg); err != nil {
			os.Exit(1)
		}
		return
	}

	// Non-bulk: delegate to bash script via syscall.Exec
	path := dirname + "/.captaincore/app/"

	args = append([]string{c.Name()}, args...)

	if flagCommand != "" {
		args = append(args, "--command="+flagCommand)
	}
	if flagRecipe != "" {
		args = append(args, "--recipe="+flagRecipe)
	}
	if flagScript != "" {
		args = append(args, "--script="+flagScript)
	}
	for _, passArg := range flagScriptPassthrough {
		args = append(args, passArg)
	}

	env = append([]string{"COLOR_RED=\033[31m"}, env...)
	env = append([]string{"COLOR_GREEN=\033[32;1m"}, env...)
	env = append([]string{"COLOR_NORMAL=\033[39m"}, env...)
	env = append([]string{"CAPTAINCORE_PATH=" + dirname + "/.captaincore"}, env...)
	if flagSkipIfRecent != "" {
		env = append([]string{"SKIP_IF_RECENT=" + flagSkipIfRecent}, env...)
	}
	if flagSkipDB == true {
		env = append([]string{"SKIP_DB=true"}, env...)
	}
	if flagInit == true {
		env = append([]string{"FLAG_INIT=true"}, env...)
	}
	if flagField != "" {
		env = append([]string{"FIELD=" + flagField}, env...)
	}
	if flagSkipRemote == true {
		env = append([]string{"SKIP_REMOTE=true"}, env...)
	}
	if flagUpdateExtras == true {
		env = append([]string{"CAPTAINCORE_UPDATE_EXTRAS=true"}, env...)
	}
	if flagDeleteAfterSnapshot == true {
		env = append([]string{"DELETE_AFTER_SNAPSHOT=true"}, env...)
	}
	if flagNotes != "" {
		env = append([]string{"FLAG_NOTES=" + flagNotes}, env...)
	}
	if flagVersion != "" {
		env = append([]string{"FLAG_VERSION=" + flagVersion}, env...)
	}
	if flagParallel != 0 {
		env = append([]string{"FLAG_PARALLEL=" + strconv.Itoa(flagParallel)}, env...)
	}
	if flagAll == true {
		env = append([]string{"FLAG_ALL=true"}, env...)
	}
	if flagForce == true {
		env = append([]string{"FLAG_FORCE=true"}, env...)
	}
	if flagHtml == true {
		env = append([]string{"FLAG_HTML=true"}, env...)
	}
	if flagTheme != "" {
		env = append([]string{"FLAG_THEME=" + flagTheme}, env...)
	}
	if flagPlugin != "" {
		env = append([]string{"FLAG_PLUGIN=" + flagPlugin}, env...)
	}
	if flagFile != "" {
		env = append([]string{"FLAG_FILE=" + flagFile}, env...)
	}
	if flagLimit != "" {
		env = append([]string{"FLAG_LIMIT=" + flagLimit}, env...)
	}
	if flagName != "" {
		env = append([]string{"FLAG_NAME=" + flagName}, env...)
	}
	if flagLink != "" {
		env = append([]string{"FLAG_LINK=" + flagLink}, env...)
	}
	if flagSubject != "" {
		env = append([]string{"FLAG_SUBJECT=" + flagSubject}, env...)
	}
	if flagStatus != "" {
		env = append([]string{"FLAG_STATUS=" + flagStatus}, env...)
	}
	if flagAction != "" {
		env = append([]string{"FLAG_ACTION=" + flagAction}, env...)
	}
	if flagEmail != "" {
		env = append([]string{"FLAG_EMAIL=" + flagEmail}, env...)
	}
	if flagUserId != "" {
		env = append([]string{"FLAG_USER_ID=" + flagUserId}, env...)
	}
	if flagFilter != "" {
		env = append([]string{"FLAG_FILTER=" + flagFilter}, env...)
	}
	if flagRetry != 0 {
		env = append([]string{"RETRY=" + strconv.Itoa(flagRetry)}, env...)
	}
	if flagPublic == true {
		env = append([]string{"FLAG_PUBLIC=true"}, env...)
	}
	if flagCode != "" {
		env = append([]string{"CAPTAINCORE_CODE=" + flagCode}, env...)
	}
	if flagDebug == true {
		env = append([]string{"CAPTAINCORE_DEBUG=true"}, env...)
	}
	if flagLabel {
		env = append([]string{"FLAG_LABEL=true"}, env...)
	}
	if flagSkipAlreadyGenerated == true {
		env = append([]string{"SKIP_ALREADY_GENERATED=true"}, env...)
	}
	if flagFleet == true {
		// Fetch CaptainIDs using native Go config
		captainIds, nativeErr := fetchCaptainIDsNative()
		if nativeErr != nil {
			log.Fatalf("Error fetching captain IDs: %s\n", nativeErr)
		}
		// Loop through CaptainIDs
		for _, fleetCaptainID := range captainIds {
			cmdRun(path+command, args, env, fleetCaptainID)
		}
		return
	}
	for i, a := range args {
		hasSpace := strings.Contains(a, " ")
		if hasSpace {
			a = strings.Replace(a, "=", "=\"", 1)
			a = a + "\""
			args[i] = a
		}
	}
	env = append([]string{"CAPTAIN_ID=" + captainID}, env...)
	err = syscall.Exec(path+command, args, env)
	log.Println(err)
}

func cmdRun(command string, args []string, env []string, fleetCaptainID string) {
	l := len(args)
	runArgs := args[1:l]
	cmd := exec.Command(command, runArgs...)
	env = append([]string{"CAPTAIN_ID=" + fleetCaptainID}, env...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
}
