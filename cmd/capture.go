package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture commands",
}

var captureGenerateCmd = &cobra.Command{
	Use:   "generate <site|target>",
	Short: "Captures website's pages visually over time based on quicksaves and html changes",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveCommand(cmd, args)
	},
}

var captureScanCmd = &cobra.Command{
	Use:   "scan <site|@target>",
	Short: "Scan captured HTML for injected scripts and stylesheets",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <site|@target> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resolveNativeOrWP(cmd, args, captureScanNative)
	},
}

// captureSignaturesFile holds the JSON structure of capture-signatures.json.
type captureSignaturesFile struct {
	KnownSafeDomains []string         `json:"known_safe_domains"`
	Signatures       []captureSigRule `json:"signatures"`
}

type captureSigRule struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Severity string   `json:"severity"`
	Type     string   `json:"type"`     // inline-pattern, src-pattern, src-domain
	Patterns []string `json:"patterns"` // for inline-pattern and src-pattern
	Domains  []string `json:"domains"`  // for src-domain
}

type compiledCaptureSig struct {
	Rule     captureSigRule
	Patterns []*regexp.Regexp
}

// captureElement represents an extracted script or stylesheet from HTML.
type captureElement struct {
	Tag      string `json:"tag"`
	Src      string `json:"src,omitempty"`
	Inline   string `json:"inline,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Severity string `json:"severity"`
	Label    string `json:"label"`
	SigName  string `json:"sig_name,omitempty"`
}

// captureScanResult holds results for a single capture file.
type captureScanResult struct {
	Page        string           `json:"page"`
	File        string           `json:"file"`
	Scripts     []captureElement `json:"scripts"`
	Stylesheets []captureElement `json:"stylesheets"`
}

// captureSiteScanResult holds results for a single site.
type captureSiteScanResult struct {
	Site    string              `json:"site"`
	Results []captureScanResult `json:"results"`
}

func loadCaptureSignatures() (*captureSignaturesFile, error) {
	home, _ := os.UserHomeDir()
	sigPath := filepath.Join(home, ".captaincore", "lib", "capture-signatures.json")
	data, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, fmt.Errorf("could not read capture signatures: %w", err)
	}
	var sigs captureSignaturesFile
	if err := json.Unmarshal(data, &sigs); err != nil {
		return nil, fmt.Errorf("could not parse capture signatures: %w", err)
	}
	return &sigs, nil
}

func compileCaptureSignatures(sigs *captureSignaturesFile) []compiledCaptureSig {
	var compiled []compiledCaptureSig
	for _, sig := range sigs.Signatures {
		var patterns []*regexp.Regexp
		for _, p := range sig.Patterns {
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				fmt.Printf("Warning: invalid regex in capture signature %s: %s\n", sig.ID, p)
				continue
			}
			patterns = append(patterns, re)
		}
		compiled = append(compiled, compiledCaptureSig{Rule: sig, Patterns: patterns})
	}
	return compiled
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func isSameOrigin(srcDomain, homeURL string) bool {
	homeDomain := extractDomain(homeURL)
	if homeDomain == "" || srcDomain == "" {
		return false
	}
	return srcDomain == homeDomain || strings.HasSuffix(srcDomain, "."+homeDomain)
}

func isKnownSafe(domain string, safeDomains []string) bool {
	for _, safe := range safeDomains {
		if domain == safe {
			return true
		}
	}
	return false
}

// classifyElement determines the severity of a capture element.
func classifyElement(elem *captureElement, homeURL string, safeDomains []string, compiled []compiledCaptureSig) {
	if elem.Src != "" {
		// Relative URLs (start with /) are same-origin
		if strings.HasPrefix(elem.Src, "/") && !strings.HasPrefix(elem.Src, "//") {
			elem.Severity = "ok"
			elem.Label = elem.Src + " (same-origin)"
			return
		}

		// Protocol-relative URLs
		if strings.HasPrefix(elem.Src, "//") {
			elem.Src = "https:" + elem.Src
		}

		// External resource
		elem.Domain = extractDomain(elem.Src)

		// Check src-domain signatures
		for _, cs := range compiled {
			if cs.Rule.Type == "src-domain" {
				for _, d := range cs.Rule.Domains {
					if elem.Domain == d || strings.HasSuffix(elem.Domain, "."+d) {
						elem.Severity = cs.Rule.Severity
						elem.SigName = cs.Rule.Name
						elem.Label = fmt.Sprintf("%s (matched: %s)", elem.Src, cs.Rule.Name)
						return
					}
				}
			}
		}

		// Check src-pattern signatures
		for _, cs := range compiled {
			if cs.Rule.Type == "src-pattern" {
				for _, pat := range cs.Patterns {
					if pat.MatchString(elem.Src) {
						elem.Severity = cs.Rule.Severity
						elem.SigName = cs.Rule.Name
						elem.Label = fmt.Sprintf("%s (%s)", elem.Src, cs.Rule.Name)
						return
					}
				}
			}
		}

		// Same-origin check
		if isSameOrigin(elem.Domain, homeURL) {
			elem.Severity = "ok"
			elem.Label = elem.Src + " (same-origin)"
			return
		}

		// Known-safe CDN
		if isKnownSafe(elem.Domain, safeDomains) {
			elem.Severity = "ok"
			elem.Label = elem.Src
			return
		}

		// Unknown external
		elem.Severity = "warning"
		elem.Label = fmt.Sprintf("%s (unknown external)", elem.Src)
		return
	}

	// Inline content
	if elem.Inline != "" {
		for _, cs := range compiled {
			if cs.Rule.Type == "inline-pattern" {
				for _, pat := range cs.Patterns {
					if pat.MatchString(elem.Inline) {
						elem.Severity = cs.Rule.Severity
						elem.SigName = cs.Rule.Name
						elem.Label = fmt.Sprintf("<inline> %s — %s", strings.TrimSpace(elem.Inline), cs.Rule.Name)
						return
					}
				}
			}
		}

		elem.Severity = "ok"
		elem.Label = fmt.Sprintf("<inline> %s", strings.TrimSpace(elem.Inline))
	}
}

// parseCapture extracts script and stylesheet elements from an HTML capture file.
func parseCapture(filePath string, homeURL string, safeDomains []string, compiled []compiledCaptureSig) (scripts []captureElement, stylesheets []captureElement) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	z := html.NewTokenizer(f)
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := z.TagName()
			tagName := string(tn)

			switch tagName {
			case "script":
				elem := captureElement{Tag: "script"}
				if hasAttr {
					for {
						key, val, more := z.TagAttr()
						if string(key) == "src" {
							elem.Src = string(val)
						}
						if !more {
							break
						}
					}
				}
				if elem.Src == "" && tt == html.StartTagToken {
					// Read inline content until </script>
					for {
						inner := z.Next()
						if inner == html.TextToken {
							elem.Inline += string(z.Text())
						} else if inner == html.EndTagToken {
							tn2, _ := z.TagName()
							if string(tn2) == "script" {
								break
							}
						} else if inner == html.ErrorToken {
							goto done
						}
					}
					elem.Inline = strings.TrimSpace(elem.Inline)
					if elem.Inline == "" {
						continue
					}
				}
				classifyElement(&elem, homeURL, safeDomains, compiled)
				scripts = append(scripts, elem)

			case "link":
				isStylesheet := false
				href := ""
				if hasAttr {
					for {
						key, val, more := z.TagAttr()
						k := string(key)
						v := string(val)
						if k == "rel" && strings.Contains(strings.ToLower(v), "stylesheet") {
							isStylesheet = true
						}
						if k == "href" {
							href = v
						}
						if !more {
							break
						}
					}
				}
				if isStylesheet && href != "" {
					elem := captureElement{Tag: "link", Src: href}
					classifyElement(&elem, homeURL, safeDomains, compiled)
					stylesheets = append(stylesheets, elem)
				}

			case "style":
				if tt == html.StartTagToken {
					elem := captureElement{Tag: "style"}
					for {
						inner := z.Next()
						if inner == html.TextToken {
							elem.Inline += string(z.Text())
						} else if inner == html.EndTagToken {
							tn2, _ := z.TagName()
							if string(tn2) == "style" {
								break
							}
						} else if inner == html.ErrorToken {
							goto done
						}
					}
					elem.Inline = strings.TrimSpace(elem.Inline)
					if elem.Inline == "" {
						continue
					}
					classifyElement(&elem, homeURL, safeDomains, compiled)
					stylesheets = append(stylesheets, elem)
				}
			}
		}
	}
done:
	return
}

func severityColor(severity string) string {
	switch severity {
	case "critical":
		return "\033[31m" // red
	case "high":
		return "\033[33m" // yellow
	case "warning":
		return "\033[33m" // yellow
	case "ok":
		return "\033[32m" // green
	default:
		return "\033[0m"
	}
}

func captureScanNative(cmd *cobra.Command, args []string) {
	sigs, err := loadCaptureSignatures()
	if err != nil {
		fmt.Println(err)
		return
	}

	compiled := compileCaptureSignatures(sigs)

	_, system, _, err := loadCaptainConfig()
	if err != nil || system == nil {
		fmt.Println("Error: Configuration file not found.")
		return
	}

	type scanTarget struct {
		Label    string
		ScanPath string
		HomeURL  string
	}
	var targets []scanTarget

	if strings.HasPrefix(args[0], "@") {
		sites, err := models.GetAllActiveSites()
		if err != nil {
			fmt.Printf("Error fetching sites: %v\n", err)
			return
		}

		environment, _ := models.ParseTargetString(args[0])

		for _, site := range sites {
			envs, err := models.FindEnvironmentsBySiteID(site.SiteID)
			if err != nil {
				continue
			}
			for _, env := range envs {
				if environment != "" && environment != "all" && !strings.EqualFold(env.Environment, environment) {
					continue
				}
				envName := strings.ToLower(env.Environment)
				siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
				scanPath := filepath.Join(system.Path, siteDir, envName, "captures")
				if _, err := os.Stat(scanPath); os.IsNotExist(err) {
					continue
				}
				targets = append(targets, scanTarget{
					Label:    fmt.Sprintf("%s-%s", site.Site, envName),
					ScanPath: scanPath,
					HomeURL:  env.HomeURL,
				})
			}
		}
		if flagFormat != "json" {
			fmt.Printf("Scanning %d environments...\n", len(targets))
		}
	} else {
		sa := parseSiteArgument(args[0])
		site, err := sa.LookupSite()
		if err != nil || site == nil {
			fmt.Printf("Error: Site '%s' not found.\n", sa.SiteName)
			return
		}
		env, err := sa.LookupEnvironment(site.SiteID)
		if err != nil || env == nil {
			fmt.Printf("Error: Environment not found.\n")
			return
		}
		envName := strings.ToLower(env.Environment)
		siteDir := fmt.Sprintf("%s_%d", site.Site, site.SiteID)
		scanPath := filepath.Join(system.Path, siteDir, envName, "captures")
		if _, err := os.Stat(scanPath); os.IsNotExist(err) {
			fmt.Printf("No captures found at %s\n", scanPath)
			return
		}
		targets = append(targets, scanTarget{
			Label:    fmt.Sprintf("%s-%s", site.Site, envName),
			ScanPath: scanPath,
			HomeURL:  env.HomeURL,
		})
	}

	filterFlag, _ := cmd.Flags().GetString("filter")
	malwareMode, _ := cmd.Flags().GetBool("malware")
	if malwareMode && filterFlag == "" {
		filterFlag = "warning"
	}
	isFleet := len(targets) > 1
	totalClean := 0
	totalWithFindings := 0
	totalCritical := 0
	totalWarning := 0

	var jsonResults []captureSiteScanResult

	for i, target := range targets {
		if isFleet && flagFormat != "json" {
			fmt.Printf("\r\033[K\033[90mScanning [%d/%d] %s...\033[0m", i+1, len(targets), target.Label)
		}

		// Find all .capture files
		entries, err := os.ReadDir(target.ScanPath)
		if err != nil {
			continue
		}

		var siteResults []captureScanResult
		siteCritical := 0
		siteWarning := 0

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".capture") {
				continue
			}

			filePath := filepath.Join(target.ScanPath, entry.Name())
			pageName := strings.TrimSuffix(entry.Name(), ".capture")
			pageName = strings.ReplaceAll(pageName, "#", "/")

			scripts, stylesheets := parseCapture(filePath, target.HomeURL, sigs.KnownSafeDomains, compiled)

			// In malware mode, skip stylesheets
			if malwareMode {
				stylesheets = nil
			}

			// Count findings
			pageCritical := 0
			pageWarning := 0
			for _, s := range scripts {
				switch s.Severity {
				case "critical":
					pageCritical++
				case "high":
					pageCritical++
				case "warning":
					pageWarning++
				}
			}
			for _, s := range stylesheets {
				switch s.Severity {
				case "critical":
					pageCritical++
				case "high":
					pageCritical++
				case "warning":
					pageWarning++
				}
			}

			siteCritical += pageCritical
			siteWarning += pageWarning

			// Apply filter
			if filterFlag != "" {
				switch filterFlag {
				case "critical":
					if pageCritical == 0 {
						continue
					}
				case "warning":
					if pageWarning == 0 && pageCritical == 0 {
						continue
					}
				case "external":
					// Keep pages that have any non-ok element
					hasExternal := false
					for _, s := range scripts {
						if s.Severity != "ok" {
							hasExternal = true
							break
						}
					}
					if !hasExternal {
						for _, s := range stylesheets {
							if s.Severity != "ok" {
								hasExternal = true
								break
							}
						}
					}
					if !hasExternal {
						continue
					}
				}
			}

			siteResults = append(siteResults, captureScanResult{
				Page:        pageName,
				File:        entry.Name(),
				Scripts:     scripts,
				Stylesheets: stylesheets,
			})
		}

		if siteCritical > 0 || siteWarning > 0 {
			totalWithFindings++
			totalCritical += siteCritical
			totalWarning += siteWarning
		} else {
			totalClean++
		}

		if flagFormat == "json" {
			if len(siteResults) > 0 {
				jsonResults = append(jsonResults, captureSiteScanResult{
					Site:    target.Label,
					Results: siteResults,
				})
			}
			continue
		}

		if isFleet {
			// Fleet mode: only print sites with findings
			if siteCritical == 0 && siteWarning == 0 {
				continue
			}
			fmt.Print("\r\033[K")
			for _, result := range siteResults {
				for _, s := range result.Scripts {
					if s.Severity != "ok" {
						color := severityColor(s.Severity)
						fmt.Printf("%s[%s]\033[0m %s — %s — %s\n", color, s.Severity, target.Label, result.Page, s.Label)
					}
				}
				for _, s := range result.Stylesheets {
					if s.Severity != "ok" {
						color := severityColor(s.Severity)
						fmt.Printf("%s[%s]\033[0m %s — %s — %s\n", color, s.Severity, target.Label, result.Page, s.Label)
					}
				}
			}
		} else {
			// Single site: detailed output
			fmt.Printf("Scanning captures for %s...\n", target.Label)
			for _, result := range siteResults {
				fmt.Printf("  Page: %s (%s)\n", result.Page, result.File)

				if len(result.Scripts) > 0 {
					fmt.Printf("\n  Scripts (%d found):\n", len(result.Scripts))
					for _, s := range result.Scripts {
						if malwareMode && s.Severity == "ok" {
							continue
						}
						color := severityColor(s.Severity)
						fmt.Printf("    %s[%s]\033[0m %s\n", color, s.Severity, s.Label)
					}
				}

				if len(result.Stylesheets) > 0 {
					fmt.Printf("\n  Stylesheets (%d found):\n", len(result.Stylesheets))
					for _, s := range result.Stylesheets {
						if malwareMode && s.Severity == "ok" {
							continue
						}
						color := severityColor(s.Severity)
						fmt.Printf("    %s[%s]\033[0m %s\n", color, s.Severity, s.Label)
					}
				}

				// Summary for this page
				pageOk := 0
				pageCrit := 0
				pageWarn := 0
				for _, s := range result.Scripts {
					switch s.Severity {
					case "ok":
						pageOk++
					case "critical", "high":
						pageCrit++
					case "warning":
						pageWarn++
					}
				}
				for _, s := range result.Stylesheets {
					switch s.Severity {
					case "ok":
						pageOk++
					case "critical", "high":
						pageCrit++
					case "warning":
						pageWarn++
					}
				}
				fmt.Printf("\n  Summary: %d critical, %d warning, %d ok\n\n", pageCrit, pageWarn, pageOk)
			}
		}
	}

	if flagFormat == "json" {
		out, _ := json.MarshalIndent(jsonResults, "", "    ")
		fmt.Println(string(out))
		return
	}

	if isFleet {
		fmt.Print("\r\033[K")
		if totalWithFindings == 0 {
			fmt.Printf("\033[32m✓\033[0m %d/%d clean, 0 with findings\n", totalClean, len(targets))
		} else {
			fmt.Printf("\033[32m✓\033[0m %d/%d clean, %d with findings (%d critical, %d warning)\n",
				totalClean, len(targets), totalWithFindings, totalCritical, totalWarning)
		}
	}
}

func init() {
	rootCmd.AddCommand(captureCmd)
	captureCmd.AddCommand(captureGenerateCmd)
	captureCmd.AddCommand(captureScanCmd)
	captureGenerateCmd.Flags().StringVarP(&flagPage, "pages", "", "", "Overrides pages to check. Defaults to site's capture_pages configuration.")
	captureScanCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "Filter results: critical, warning, or external")
	captureScanCmd.Flags().StringVarP(&flagFormat, "format", "", "", "Output format (json)")
	captureScanCmd.Flags().Bool("malware", false, "Malware scan mode: scripts only, hide ok results")
}
