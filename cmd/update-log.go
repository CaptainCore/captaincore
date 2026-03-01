package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var updateLogCmd = &cobra.Command{
	Use:   "update-log",
	Short: "Update log commands",
}

var updateLogGetCmd = &cobra.Command{
	Use:   "get <site>",
	Short: "Get update log for a site",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <quicksave-hash-before> <quicksave-hash-after> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var updateLogGenerateCmd = &cobra.Command{
	Use:   "generate <site>",
	Short: "generates new update log",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("requires <site> <quicksave-hash-before> <quicksave-hash-after> arguments")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, updateLogGenerateNative)
	},
}

var updateLogListCmd = &cobra.Command{
	Use:   "list <site>",
	Short: "List of update logs",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var updateLogListGenerateCmd = &cobra.Command{
	Use:   "list-generate <site>",
	Short: "generates new update log list",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, updateLogListGenerateNative)
	},
}

func init() {
	rootCmd.AddCommand(updateLogCmd)
	updateLogCmd.AddCommand(updateLogGetCmd)
	updateLogCmd.AddCommand(updateLogGenerateCmd)
	updateLogCmd.AddCommand(updateLogListCmd)
	updateLogCmd.AddCommand(updateLogListGenerateCmd)
}

// updateLogGenerateNative implements `captaincore update-log generate <site> <hash-before> <hash-after>` natively in Go.
func updateLogGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	hashBefore := args[1]
	hashAfter := args[2]

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	quicksaveDir := filepath.Join(system.Path, siteDir, envName, "quicksave")
	updateLogsDir := filepath.Join(system.Path, siteDir, envName, "update-logs")
	os.MkdirAll(updateLogsDir, 0755)

	fmt.Printf("Generating %s/%s/update-logs/log-%s-%s.json\n", siteDir, envName, hashBefore, hashAfter)

	// Helper to run git commands in quicksave dir
	gitShow := func(gitArgs ...string) string {
		c := exec.Command("git", gitArgs...)
		c.Dir = quicksaveDir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	// Get current (after) commit data
	currentCore := gitShow("show", hashAfter+":versions/core.json")
	currentThemesRaw := gitShow("show", hashAfter+":versions/themes.json")
	currentPluginsRaw := gitShow("show", hashAfter+":versions/plugins.json")
	createdAt := gitShow("show", "-s", "--pretty=format:%ct", hashAfter)

	// Get previous (before) commit data
	previousCore := gitShow("show", hashBefore+":versions/core.json")
	previousThemesRaw := gitShow("show", hashBefore+":versions/themes.json")
	previousPluginsRaw := gitShow("show", hashBefore+":versions/plugins.json")
	startedAt := gitShow("show", "-s", "--pretty=format:%ct", hashBefore)

	// Git diff status (note: after first, before second — matches PHP)
	status := gitShow("diff", hashAfter, hashBefore, "--shortstat", "--format=")

	var currentThemes []map[string]interface{}
	var currentPlugins []map[string]interface{}
	var previousThemes []map[string]interface{}
	var previousPlugins []map[string]interface{}

	json.Unmarshal([]byte(currentThemesRaw), &currentThemes)
	json.Unmarshal([]byte(currentPluginsRaw), &currentPlugins)
	json.Unmarshal([]byte(previousThemesRaw), &previousThemes)
	json.Unmarshal([]byte(previousPluginsRaw), &previousPlugins)

	// Build lookup maps for previous items
	prevThemeMap := make(map[string]map[string]interface{})
	for _, t := range previousThemes {
		if name, ok := t["name"].(string); ok {
			prevThemeMap[name] = t
		}
	}
	prevPluginMap := make(map[string]map[string]interface{})
	for _, p := range previousPlugins {
		if name, ok := p["name"].(string); ok {
			prevPluginMap[name] = p
		}
	}

	// Track current names for deleted detection
	currentThemeNames := make(map[string]bool)
	currentPluginNames := make(map[string]bool)

	// Compare themes
	for i, theme := range currentThemes {
		name, _ := theme["name"].(string)
		currentThemeNames[name] = true

		prev, existed := prevThemeMap[name]
		if !existed {
			// New theme — no changed field (matching PHP behavior)
			continue
		}

		changed := false
		if fmt.Sprint(theme["version"]) != fmt.Sprint(prev["version"]) {
			currentThemes[i]["changed_version"] = prev["version"]
			changed = true
		}
		if fmt.Sprint(theme["status"]) != fmt.Sprint(prev["status"]) {
			currentThemes[i]["changed_status"] = prev["status"]
			changed = true
		}
		if fmt.Sprint(theme["title"]) != fmt.Sprint(prev["title"]) {
			currentThemes[i]["changed_title"] = prev["title"]
			changed = true
		}
		currentThemes[i]["changed"] = changed
	}

	// Compare plugins
	for i, plugin := range currentPlugins {
		name, _ := plugin["name"].(string)
		currentPluginNames[name] = true

		prev, existed := prevPluginMap[name]
		if !existed {
			// New plugin — no changed field (matching PHP behavior)
			continue
		}

		changed := false
		if fmt.Sprint(plugin["version"]) != fmt.Sprint(prev["version"]) {
			currentPlugins[i]["changed_version"] = prev["version"]
			changed = true
		}
		if fmt.Sprint(plugin["status"]) != fmt.Sprint(prev["status"]) {
			currentPlugins[i]["changed_status"] = prev["status"]
			changed = true
		}
		if fmt.Sprint(plugin["title"]) != fmt.Sprint(prev["title"]) {
			currentPlugins[i]["changed_title"] = prev["title"]
			changed = true
		}
		currentPlugins[i]["changed"] = changed
	}

	// Find deleted themes
	var themesDeleted []map[string]interface{}
	for _, pt := range previousThemes {
		if name, ok := pt["name"].(string); ok && !currentThemeNames[name] {
			themesDeleted = append(themesDeleted, pt)
		}
	}

	// Find deleted plugins
	var pluginsDeleted []map[string]interface{}
	for _, pp := range previousPlugins {
		if name, ok := pp["name"].(string); ok && !currentPluginNames[name] {
			pluginsDeleted = append(pluginsDeleted, pp)
		}
	}

	// Sort themes: changed first (true before false), then alpha by name
	sort.Slice(currentThemes, func(i, j int) bool {
		iChanged, _ := currentThemes[i]["changed"].(bool)
		jChanged, _ := currentThemes[j]["changed"].(bool)
		if iChanged != jChanged {
			if iChanged {
				return true
			}
			return false
		}
		iName, _ := currentThemes[i]["name"].(string)
		jName, _ := currentThemes[j]["name"].(string)
		return iName < jName
	})

	// Sort plugins: must-use status last, then changed first, then alpha by name
	sort.Slice(currentPlugins, func(i, j int) bool {
		iStatus, _ := currentPlugins[i]["status"].(string)
		jStatus, _ := currentPlugins[j]["status"].(string)
		if iStatus == "must-use" || jStatus == "must-use" {
			return iStatus < jStatus
		}
		iChanged, _ := currentPlugins[i]["changed"].(bool)
		jChanged, _ := currentPlugins[j]["changed"].(bool)
		if iChanged != jChanged {
			if iChanged {
				return true
			}
			return false
		}
		iName, _ := currentPlugins[i]["name"].(string)
		jName, _ := currentPlugins[j]["name"].(string)
		return iName < jName
	})

	// Ensure deleted slices are empty arrays not null in JSON
	if themesDeleted == nil {
		themesDeleted = []map[string]interface{}{}
	}
	if pluginsDeleted == nil {
		pluginsDeleted = []map[string]interface{}{}
	}

	// Build output
	output := map[string]interface{}{
		"created_at":      createdAt,
		"started_at":      startedAt,
		"hash_before":     hashBefore,
		"hash_after":      hashAfter,
		"core":            currentCore,
		"core_previous":   previousCore,
		"theme_count":     len(currentThemes),
		"themes":          currentThemes,
		"themes_deleted":  themesDeleted,
		"plugin_count":    len(currentPlugins),
		"plugins":         currentPlugins,
		"plugins_deleted": pluginsDeleted,
		"status":          status,
	}

	result, _ := json.MarshalIndent(output, "", "    ")
	fmt.Print(string(result))

	// Write to log file
	logFile := filepath.Join(updateLogsDir, fmt.Sprintf("log-%s-%s.json", hashBefore, hashAfter))
	os.WriteFile(logFile, result, 0644)
}

// updateLogListGenerateNative implements `captaincore update-log list-generate <site>` natively in Go.
func updateLogListGenerateNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])

	site, err := sa.LookupSite()
	if err != nil || site == nil {
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
	envName := strings.ToLower(env.Environment)
	updateLogsDir := filepath.Join(system.Path, siteDir, envName, "update-logs")
	os.MkdirAll(updateLogsDir, 0755)

	// Find all log-*.json files
	matches, _ := filepath.Glob(filepath.Join(updateLogsDir, "log-*.json"))
	if len(matches) == 0 {
		fmt.Printf("Skipping generation of %s/%s/update-logs/list.json as no update logs found.\n", siteDir, envName)
		return
	}

	fmt.Printf("Generating %s/%s/update-logs/list.json\n", siteDir, envName)

	var updateLogs []map[string]interface{}

	for _, match := range matches {
		data, err := os.ReadFile(match)
		if err != nil {
			continue
		}

		var logData map[string]interface{}
		if json.Unmarshal(data, &logData) != nil {
			continue
		}

		item := map[string]interface{}{
			"hash_before": logData["hash_before"],
			"hash_after":  logData["hash_after"],
			"created_at":  logData["created_at"],
			"started_at":  logData["started_at"],
			"status":      logData["status"],
		}

		// Conditionally include core fields
		if core, ok := logData["core"].(string); ok && core != "" {
			item["core"] = core
		}
		if corePrevious, ok := logData["core_previous"].(string); ok && corePrevious != "" {
			item["core_previous"] = corePrevious
		}

		// Conditionally include counts (only if non-zero)
		if themeCount, ok := logData["theme_count"].(float64); ok && themeCount > 0 {
			item["theme_count"] = int(themeCount)
		}
		if pluginCount, ok := logData["plugin_count"].(float64); ok && pluginCount > 0 {
			item["plugin_count"] = int(pluginCount)
		}

		// Count changed themes and plugins
		themesChanged := 0
		if themes, ok := logData["themes"].([]interface{}); ok {
			for _, t := range themes {
				if theme, ok := t.(map[string]interface{}); ok {
					changedVersion, _ := theme["changed_version"].(string)
					changedStatus, _ := theme["changed_status"].(string)
					if changedVersion != "" || changedStatus != "" {
						themesChanged++
					}
				}
			}
		}

		pluginsChanged := 0
		if plugins, ok := logData["plugins"].([]interface{}); ok {
			for _, p := range plugins {
				if plugin, ok := p.(map[string]interface{}); ok {
					changedVersion, _ := plugin["changed_version"].(string)
					changedStatus, _ := plugin["changed_status"].(string)
					if changedVersion != "" || changedStatus != "" {
						pluginsChanged++
					}
				}
			}
		}

		item["themes_changed"] = themesChanged
		item["plugins_changed"] = pluginsChanged

		updateLogs = append(updateLogs, item)
	}

	// Sort descending by created_at
	sort.Slice(updateLogs, func(i, j int) bool {
		iCreated, _ := updateLogs[i]["created_at"].(string)
		jCreated, _ := updateLogs[j]["created_at"].(string)
		return iCreated > jCreated
	})

	result, _ := json.MarshalIndent(updateLogs, "", "    ")
	fmt.Println(string(result))

	// Write to list.json
	listFile := filepath.Join(updateLogsDir, "list.json")
	os.WriteFile(listFile, result, 0644)
}
