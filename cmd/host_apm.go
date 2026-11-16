package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// host apm: what a site's visitors waited for, from the hourly files the
// node keeps (cc-backups apm, collected every minute by cc-metrics.timer).
// Layer one (requests from the container's access log) is always there;
// layer two (transactions, plugins, queries, outside calls) only for the
// hours the site's APM switch was on. The recorder is installed by
// `host apply` from host_apm.php when the hosting configuration says so.

var flagApmRange string
var flagApmJSON bool

var hostApmCmd = &cobra.Command{
	Use:   "apm <site>[-env] [--range=1h|24h|7d] [--json]",
	Short: "Requests, slow pages, plugins, queries and outside calls of a site on our servers",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, func(_ *cobra.Command, a []string) { runHostApm(a) })
	},
}

func runHostApm(args []string) {
	if flagApmRange != "1h" && flagApmRange != "24h" && flagApmRange != "7d" {
		fail("--range must be 1h, 24h or 7d")
	}
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
	res, err := ccTool(t, "apm", t.container, flagApmRange)
	if flagApmJSON {
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
	edge, _ := res["edge"].(map[string]interface{})
	wp, _ := res["wp"].(map[string]interface{})
	ms := func(v interface{}) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprintf("%.0f ms", toFloat(v))
	}
	fmt.Printf("%s on %s, last %s\n", t.site.Site, t.server, flagApmRange)
	fmt.Printf("  Requests %.0f: PHP %.0f, saved pages %.0f, static files %.0f, 4xx %.0f, 5xx %.0f\n",
		toFloat(edge["requests"]), toFloat(edge["php"]), toFloat(edge["hit"]), toFloat(edge["static"]), toFloat(edge["e4"]), toFloat(edge["e5"]))
	fmt.Printf("  PHP time: median %s, p95 %s, slowest %s\n", ms(edge["php_p50_ms"]), ms(edge["php_p95_ms"]), ms(edge["php_max_ms"]))
	if routes, ok := edge["routes"].([]interface{}); ok && len(routes) > 0 {
		fmt.Println("  Most PHP time:")
		for i, r := range routes {
			if i == 8 {
				break
			}
			m, _ := r.(map[string]interface{})
			fmt.Printf("    %-50.50s %6.0f × avg %s, p95 %s\n", m["route"], toFloat(m["n"]), ms(m["avg_ms"]), ms(m["p95_ms"]))
		}
	}
	if toFloat(wp["requests"]) == 0 {
		fmt.Println("  APM recorded nothing in this range (turn it on in the site's hosting configuration).")
		return
	}
	fmt.Printf("  APM: %.0f pages, avg %s (database %s, outside calls %s)\n", toFloat(wp["requests"]), ms(wp["avg_ms"]), ms(wp["db_ms"]), ms(wp["http_ms"]))
	if list, ok := wp["transactions"].([]interface{}); ok {
		fmt.Println("  Slowest transactions (by total time):")
		for i, r := range list {
			if i == 8 {
				break
			}
			m, _ := r.(map[string]interface{})
			fmt.Printf("    %-50.50s %6.0f × avg %s, p95 %s, db %s\n", m["tx"], toFloat(m["n"]), ms(m["avg_ms"]), ms(m["p95_ms"]), ms(m["db_ms"]))
		}
	}
	if list, ok := wp["plugins"].([]interface{}); ok {
		fmt.Println("  Plugins (load + database + outside calls, per page):")
		for i, r := range list {
			if i == 8 {
				break
			}
			m, _ := r.(map[string]interface{})
			fmt.Printf("    %-40.40s %s (load %s, db %s, http %s)\n", m["component"], ms(m["avg_ms"]), ms(m["load_ms"]), ms(m["db_ms"]), ms(m["http_ms"]))
		}
	}
}

func init() {
	hostCmd.AddCommand(hostApmCmd)
	hostApmCmd.Flags().StringVar(&flagApmRange, "range", "1h", "1h, 24h or 7d")
	hostApmCmd.Flags().BoolVar(&flagApmJSON, "json", false, "Print the whole report as JSON")
}
