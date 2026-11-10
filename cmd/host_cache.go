package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// host cache: the page cache of a site on one of our servers. Pages are saved
// by the container's PHP (the prepend file `host apply` installs, see
// host_page_cache.php) and served by its web server; these commands only
// look at the saved files or delete them.

var flagCacheJSON bool

var hostCacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Page cache of a site on one of our servers",
}

func runHostCache(action string, args []string) {
	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fail("Configuration file not found.")
	}
	t, err := hostTarget(system, captain, args[0])
	if err != nil {
		fail("%v", err)
	}
	if err := t.ensureBackupTool(); err != nil {
		fail("%v", err)
	}
	res, err := ccTool(t, "cache-"+action, t.container)
	if flagCacheJSON {
		if res == nil {
			res = map[string]interface{}{"status": "error", "error": fmt.Sprint(err)}
		}
		line, _ := json.Marshal(res)
		fmt.Println(string(line))
		if err != nil {
			os.Exit(1)
		}
		return
	}
	if err != nil {
		fail("%v", err)
	}
	state := "off"
	if on, _ := res["enabled"].(bool); on {
		state = "on"
	}
	if action == "clear" {
		fmt.Printf("Cleared %.0f saved page(s) of %s on %s. Page cache is %s.\n", toFloat(res["cleared"]), t.site.Site, t.server, state)
		return
	}
	pages := toFloat(res["pages"])
	fmt.Printf("Page cache of %s on %s is %s: %.0f page(s), %.1f MB\n", t.site.Site, t.server, state, pages, toFloat(res["bytes"])/1e6)
	if pages > 0 {
		now := time.Unix(int64(toFloat(res["now"])), 0)
		fmt.Printf("  oldest saved %s ago, newest %s ago\n",
			now.Sub(time.Unix(int64(toFloat(res["oldest"])), 0)).Round(time.Second), now.Sub(time.Unix(int64(toFloat(res["newest"])), 0)).Round(time.Second))
	}
}

func init() {
	hostCmd.AddCommand(hostCacheCmd)
	for _, sub := range [][3]string{
		{"status <site>[-env]", "How many pages are saved, how big and how old", "status"},
		{"clear <site>[-env]", "Delete every saved page (the next visit saves it again)", "clear"},
	} {
		action := sub[2]
		c := &cobra.Command{
			Use: sub[0], Short: sub[1], Args: cobra.ExactArgs(1),
			Run: func(cmd *cobra.Command, args []string) {
				resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { runHostCache(action, a) })
			},
		}
		c.Flags().BoolVar(&flagCacheJSON, "json", false, "Print the raw JSON reply")
		hostCacheCmd.AddCommand(c)
	}
}
