package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats <site>",
	Short: "Fetches stats from WordPress.com stats, Fathom Analytics or Fathom Lite",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, statsNative)
	},
}

// statMonth is one month of pageviews ("2026-01"), from the same request
// that gives the total.
type statMonth struct {
	M string `json:"m"`
	V int    `json:"v"`
}

// printStats prints the yearly pageview total, or with --monthly a JSON
// object {"total": N, "months": [{"m": "2026-01", "v": N}, …]} so
// usage-update can keep the months without another API request.
func printStats(cmd *cobra.Command, total int, months []statMonth) {
	if monthly, _ := cmd.Flags().GetBool("monthly"); monthly {
		if months == nil {
			months = []statMonth{}
		}
		out, _ := json.Marshal(map[string]interface{}{"total": total, "months": months})
		fmt.Print(string(out))
		return
	}
	fmt.Print(total)
}

// statsNative implements `captaincore stats <site>` natively in Go.
func statsNative(cmd *cobra.Command, args []string) {
	sa := parseSiteArgument(args[0])
	site, err := sa.LookupSite()
	if err != nil || site == nil {
		fmt.Printf("Error: Site '%s' not found.", sa.SiteName)
		return
	}

	env, err := sa.LookupEnvironment(site.SiteID)
	if err != nil || env == nil {
		return
	}

	_, system, captain, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	// Parse fathom from environment details
	envDetails := env.ParseDetails()
	var fathomAnalytics []struct {
		Domain string `json:"domain"`
		Code   string `json:"code"`
	}
	if envDetails.Fathom != nil && string(envDetails.Fathom) != "null" && string(envDetails.Fathom) != "" {
		json.Unmarshal(envDetails.Fathom, &fathomAnalytics)
	}

	var fathomIDs []string
	for _, fa := range fathomAnalytics {
		if fa.Code != "" {
			fathomIDs = append(fathomIDs, fa.Code)
		}
	}

	var fathomID string

	// If no fathom analytics or not exactly 1 ID, hunt by site name
	if len(fathomIDs) != 1 {
		siteName := env.HomeURL
		siteName = strings.TrimPrefix(siteName, "http://www.")
		siteName = strings.TrimPrefix(siteName, "https://www.")
		siteName = strings.TrimPrefix(siteName, "http://")
		siteName = strings.TrimPrefix(siteName, "https://")

		if system.FathomAPIKey != "" {
			// Search Fathom API for matching site
			fathomID = searchFathomSiteByName(system.FathomAPIKey, siteName)
		}
	}

	if fathomID == "" && len(fathomIDs) > 0 {
		fathomID = fathomIDs[0]
	}
	if fathomID == "" {
		printStats(cmd, 0, nil)
		return
	}

	// Fetch stats from Fathom API
	now := time.Now()
	yearAgo := now.AddDate(-1, 0, 0)
	after := now.Format("2006-01-02 15:04:05")
	before := yearAgo.Format("2006-01-02 15:04:05")

	url := fmt.Sprintf("https://api.usefathom.com/v1/aggregations?entity=pageview&entity_id=%s&aggregates=visits,pageviews,avg_duration,bounce_rate&date_from=%s&date_to=%s&date_grouping=month&sort_by=timestamp:asc",
		fathomID, before, after)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+system.FathomAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		printStats(cmd, 0, nil)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var stats []struct {
		Pageviews string `json:"pageviews"`
		Date      string `json:"date"`
	}
	if json.Unmarshal(body, &stats) != nil {
		printStats(cmd, 0, nil)
		return
	}

	totalPageviews := 0
	var months []statMonth
	for _, s := range stats {
		if v, err := strconv.Atoi(s.Pageviews); err == nil {
			totalPageviews += v
			if len(s.Date) >= 7 {
				months = append(months, statMonth{M: s.Date[:7], V: v})
			}
		}
	}

	if totalPageviews > 0 {
		printStats(cmd, totalPageviews, months)
		return
	}

	// Fallback: WordPress.com API
	if captain != nil {
		accessKey := ""
		if v, ok := captain.Keys["access_key"]; ok {
			accessKey = v
		}
		if accessKey != "" && site.Name != "" {
			wpPageviews, wpMonths := fetchWordPressComStats(accessKey, site.Name)
			printStats(cmd, wpPageviews, wpMonths)
			return
		}
	}

	printStats(cmd, 0, nil)
}

// searchFathomSiteByName searches the Fathom API for a site by name.
func searchFathomSiteByName(apiKey, siteName string) string {
	url := "https://api.usefathom.com/v1/sites?limit=100"
	for {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		}
		if json.Unmarshal(body, &result) != nil {
			return ""
		}

		for _, s := range result.Data {
			if s.Name == siteName {
				return s.ID
			}
		}

		if !result.HasMore || result.NextCursor == "" {
			break
		}
		url = fmt.Sprintf("https://api.usefathom.com/v1/sites?limit=100&starting_after=%s", result.NextCursor)
	}
	return ""
}

// fetchWordPressComStats fetches yearly pageview estimate from WordPress.com
// API, and the months it was estimated from.
func fetchWordPressComStats(accessKey, siteName string) (int, []statMonth) {
	url := fmt.Sprintf("https://public-api.wordpress.com/rest/v1/sites/%s/stats/visits?unit=month&quantity=12", siteName)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+accessKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var stats struct {
		Error string          `json:"error"`
		Data  [][]json.Number `json:"data"`
	}
	if json.Unmarshal(body, &stats) != nil {
		return 0, nil
	}

	// If error is unknown_blog, try with www prefix
	if stats.Error == "unknown_blog" {
		url = fmt.Sprintf("https://public-api.wordpress.com/rest/v1/sites/www.%s/stats/visits?unit=month&quantity=12", siteName)
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+accessKey)
		resp, err = client.Do(req)
		if err != nil {
			return 0, nil
		}
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
		if json.Unmarshal(body, &stats) != nil {
			return 0, nil
		}
	}

	if stats.Data == nil {
		return 0, nil
	}

	count := 0
	total := 0
	var months []statMonth
	for _, stat := range stats.Data {
		if len(stat) >= 2 {
			val, _ := stat[1].Int64()
			if val > 0 || stat[0].String() != "" {
				total++
			}
			count += int(val)
			if p := stat[0].String(); len(p) >= 7 {
				months = append(months, statMonth{M: p[:7], V: int(val)})
			}
		}
	}

	if total == 0 {
		return 0, nil
	}

	monthlyAverage := count / total
	return monthlyAverage * 12, months
}

func init() {
	statsCmd.Flags().Bool("monthly", false, "Print JSON with the total and the monthly pageviews it was summed from")
	rootCmd.AddCommand(statsCmd)
}
