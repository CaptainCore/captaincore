package cmd

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/CaptainCore/captaincore/config"
	"github.com/CaptainCore/captaincore/models"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var flagConnectURL, flagConnectUsername, flagConnectPassword, flagConnectServerURL string
var flagConnectSkipSSL, flagConnectSync bool

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect CLI to a CaptainCore WordPress site",
	Long: `Authenticates with a CaptainCore WordPress site using Application Passwords,
fetches all site/account/provider data, and sets up the local CLI database and config.

Pairing works in both directions. The Manager hands back its CLI token, which is
written to config.json and to data/config.json so a local 'captaincore server'
accepts the Manager's requests. Pass --server-url (or answer the prompt) with the
public URL of this CLI server and the Manager is told where to dispatch jobs, so
no wp-config.php constant is needed.`,
	Run: func(cmd *cobra.Command, args []string) {
		connectRun()
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringVar(&flagConnectURL, "url", "", "WordPress site URL")
	connectCmd.Flags().StringVar(&flagConnectUsername, "username", "", "WordPress username")
	connectCmd.Flags().StringVar(&flagConnectPassword, "password", "", "WordPress application password")
	connectCmd.Flags().StringVar(&flagConnectServerURL, "server-url", "", "Public URL of this CLI server (https://...), registered with the Manager for job dispatch")
	connectCmd.Flags().BoolVar(&flagConnectSkipSSL, "skip-ssl", false, "Skip SSL certificate verification")
	connectCmd.Flags().BoolVar(&flagConnectSync, "sync", false, "Re-sync data using saved credentials (no prompts)")
}

// connectResponse mirrors the JSON returned by /wp-json/captaincore/v1/cli/connect
type connectResponse struct {
	Token          string                 `json:"token"`
	CLIAddress     string                 `json:"cli_address"`
	APIURL         string                 `json:"api_url"`
	GUIURL         string                 `json:"gui_url"`
	Sites          []models.Site          `json:"sites"`
	Environments   []models.Environment   `json:"environments"`
	Accounts       []models.Account       `json:"accounts"`
	Providers      []models.Provider      `json:"providers"`
	Domains        []models.Domain        `json:"domains"`
	AccountSite    []models.AccountSite   `json:"account_site"`
	AccountDomain  []models.AccountDomain `json:"account_domain"`
	AccountUser    []models.AccountUser   `json:"account_user"`
	Configurations json.RawMessage        `json:"configurations"`
	Defaults       json.RawMessage        `json:"defaults"`
}

func connectRun() {
	// --sync mode: use saved token + API URL from config.json
	if flagConnectSync {
		connectSyncRun()
		return
	}

	reader := bufio.NewReader(os.Stdin)

	// Prompt for missing values
	wpURL := flagConnectURL
	if wpURL == "" {
		fmt.Print("WordPress URL: ")
		line, _ := reader.ReadString('\n')
		wpURL = strings.TrimSpace(line)
	}
	wpURL = strings.TrimRight(wpURL, "/")

	username := flagConnectUsername
	if username == "" {
		fmt.Print("Username: ")
		line, _ := reader.ReadString('\n')
		username = strings.TrimSpace(line)
	}

	password := flagConnectPassword
	if password == "" {
		fmt.Print("Application password: ")
		raw, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // newline after hidden input
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading password: %v\n", err)
			os.Exit(1)
		}
		password = strings.TrimSpace(string(raw))
	}

	serverURL := strings.TrimSpace(flagConnectServerURL)
	if serverURL == "" && flagConnectURL == "" && term.IsTerminal(int(syscall.Stdin)) {
		fmt.Print("CLI server URL for job dispatch (optional, blank to skip): ")
		line, _ := reader.ReadString('\n')
		serverURL = strings.TrimSpace(line)
	}
	serverURL = strings.TrimRight(serverURL, "/")
	if serverURL != "" && !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		fmt.Fprintln(os.Stderr, "Error: --server-url must start with http:// or https://")
		os.Exit(1)
	}

	// Build the connect URL
	connectURL := wpURL + "/wp-json/captaincore/v1/cli/connect"

	fmt.Printf("Connecting to %s...\n", wpURL)

	// POST with Basic Auth
	body, err := connectPost(connectURL, username, password, serverURL, flagConnectSkipSSL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Parse response
	var resp connectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		fmt.Fprintf(os.Stderr, "Response body: %s\n", truncate(string(body), 500))
		os.Exit(1)
	}

	if resp.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: No token in response. Is the CaptainCore Manager plugin installed and up to date?")
		os.Exit(1)
	}

	// Initialize the database
	if err := models.InitDB(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		os.Exit(1)
	}

	// Check if this is a resync (database already has data)
	isResync := dbHasData()
	if isResync {
		showSyncPreview(resp)
		if !confirmPrompt("Apply changes?") {
			fmt.Println("Aborted.")
			return
		}
	}

	// Upsert all data
	siteCount := upsertSites(resp.Sites, resp.Environments, resp.AccountSite, isResync)
	accountCount := upsertAccounts(resp.Accounts, resp.AccountUser, resp.AccountDomain, resp.AccountSite, isResync)
	providerCount := upsertProviders(resp.Providers, isResync)
	upsertDomains(resp.Domains, isResync)

	// Store configurations
	if resp.Configurations != nil {
		models.SetConfiguration(1, "configurations", string(resp.Configurations))
	}
	if resp.Defaults != nil {
		models.SetConfiguration(1, "defaults", string(resp.Defaults))
	}

	// Update config.json
	configAction, err := updateConfigFile(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not update config.json: %v\n", err)
	}

	// Let a local `captaincore server` accept the Manager's requests.
	serverConfigAction, err := writeServerTokenFile(resp.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not write data/config.json: %v\n", err)
	}

	// Print summary
	fmt.Printf("\nConnected to %s\n\n", wpURL)
	fmt.Printf("Synced: %d sites, %d accounts, %d providers\n", siteCount, accountCount, providerCount)

	home, _ := os.UserHomeDir()
	fmt.Printf("Config: %s/config.json (%s)\n", home+"/.captaincore", configAction)
	if serverConfigAction != "" {
		fmt.Printf("Server token: %s/data/config.json (%s)\n", home+"/.captaincore", serverConfigAction)
	}
	if serverURL != "" {
		if resp.CLIAddress == serverURL {
			fmt.Printf("Dispatch: Manager will send jobs to %s\n", resp.CLIAddress)
		} else if resp.CLIAddress != "" {
			fmt.Printf("Dispatch: Manager reports CLI address %s (a wp-config constant overrides the saved value)\n", resp.CLIAddress)
		} else {
			fmt.Println("Dispatch: Manager did not confirm the CLI server URL. Is the CaptainCore Manager plugin up to date?")
		}
	} else if resp.CLIAddress != "" {
		fmt.Printf("Dispatch: Manager sends jobs to %s\n", resp.CLIAddress)
	} else {
		fmt.Println("Dispatch: no CLI server URL registered. Re-run with --server-url=https://... if this machine runs 'captaincore server'.")
	}

	checkDependencies()
}

func connectSyncRun() {
	_, system, captain, err := loadCaptainConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: No config.json found. Run 'captaincore connect' first to set up credentials.")
		os.Exit(1)
	}

	// Get the API URL and token from saved config
	apiURL := getVarString(captain, "captaincore_api")
	token := ""
	if captain != nil {
		token = captain.Keys["token"]
	}
	guiURL := getVarString(captain, "captaincore_gui")

	if apiURL == "" || token == "" {
		fmt.Fprintln(os.Stderr, "Error: Missing API URL or token in config.json. Run 'captaincore connect' to set up credentials.")
		os.Exit(1)
	}

	// Derive the connect endpoint from the API URL
	// API URL is like https://site.com/wp-json/captaincore/v1/api
	// Connect URL is   https://site.com/wp-json/captaincore/v1/cli/connect
	connectURL := strings.TrimSuffix(apiURL, "/api") + "/cli/connect"

	skipSSL := flagConnectSkipSSL
	if system != nil && system.CaptainCoreDev != "" && system.CaptainCoreDev != "false" {
		skipSSL = true
	}

	if guiURL != "" {
		fmt.Printf("Syncing from %s...\n", guiURL)
	} else {
		fmt.Println("Syncing...")
	}

	// POST with token auth
	body, err := connectPostWithToken(connectURL, token, skipSSL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var resp connectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if err := models.InitDB(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		os.Exit(1)
	}

	// Always resync in --sync mode, skip confirmation
	siteCount := upsertSites(resp.Sites, resp.Environments, resp.AccountSite, true)
	accountCount := upsertAccounts(resp.Accounts, resp.AccountUser, resp.AccountDomain, resp.AccountSite, true)
	providerCount := upsertProviders(resp.Providers, true)
	upsertDomains(resp.Domains, true)

	if resp.Configurations != nil {
		models.SetConfiguration(1, "configurations", string(resp.Configurations))
	}
	if resp.Defaults != nil {
		models.SetConfiguration(1, "defaults", string(resp.Defaults))
	}

	// Update config with any new token/URL values
	updateConfigFile(resp)

	fmt.Printf("\nSynced: %d sites, %d accounts, %d providers\n", siteCount, accountCount, providerCount)

	checkDependencies()
}

func connectPostWithToken(url, token string, skipSSL bool) ([]byte, error) {
	payload, _ := json.Marshal(map[string]string{"token": token})
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	if skipSSL {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func connectPost(url, username, password, serverURL string, skipSSL bool) ([]byte, error) {
	payload := map[string]string{}
	if serverURL != "" {
		payload["cli_address"] = serverURL
	}
	payloadJSON, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadJSON))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	creds := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	if skipSSL {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	switch resp.StatusCode {
	case 200:
		return body, nil
	case 401:
		return nil, fmt.Errorf("authentication failed (401). Check your username and application password")
	case 403:
		return nil, fmt.Errorf("permission denied (403). Your user account must have the administrator role")
	case 404:
		return nil, fmt.Errorf("endpoint not found (404). Is the CaptainCore Manager plugin installed and updated?")
	default:
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
}

func showSyncPreview(resp connectResponse) {
	// Count existing records
	var localSiteCount, localAccountCount, localProviderCount int64
	models.DB.Table("captaincore_sites").Count(&localSiteCount)
	models.DB.Table("captaincore_accounts").Count(&localAccountCount)
	models.DB.Table("captaincore_providers").Count(&localProviderCount)

	// Build remote ID sets
	remoteSiteIDs := make(map[uint]bool)
	for _, s := range resp.Sites {
		remoteSiteIDs[s.SiteID] = true
	}
	remoteAccountIDs := make(map[uint]bool)
	for _, a := range resp.Accounts {
		remoteAccountIDs[a.AccountID] = true
	}
	remoteProviderIDs := make(map[uint]bool)
	for _, p := range resp.Providers {
		remoteProviderIDs[p.ProviderID] = true
	}

	// Count adds, updates, removes for sites
	siteAdds, siteUpdates, siteRemoves := countChanges("captaincore_sites", "site_id", remoteSiteIDs)
	accountAdds, accountUpdates, accountRemoves := countChanges("captaincore_accounts", "account_id", remoteAccountIDs)
	providerAdds, providerUpdates, providerRemoves := countChanges("captaincore_providers", "provider_id", remoteProviderIDs)

	fmt.Println("\nSync preview:")
	fmt.Printf("  Sites:     %d to add, %d to update, %d to remove\n", siteAdds, siteUpdates, siteRemoves)
	fmt.Printf("  Accounts:  %d to add, %d to update, %d to remove\n", accountAdds, accountUpdates, accountRemoves)
	fmt.Printf("  Providers: %d to add, %d to update, %d to remove\n", providerAdds, providerUpdates, providerRemoves)

	// List removals by name
	if siteRemoves > 0 {
		names := getRemovalNames("captaincore_sites", "site_id", "name", remoteSiteIDs)
		fmt.Printf("\n  Sites to remove: %s\n", strings.Join(names, ", "))
	}
	if accountRemoves > 0 {
		names := getRemovalNames("captaincore_accounts", "account_id", "name", remoteAccountIDs)
		fmt.Printf("  Accounts to remove: %s\n", strings.Join(names, ", "))
	}
	if providerRemoves > 0 {
		names := getRemovalNames("captaincore_providers", "provider_id", "name", remoteProviderIDs)
		fmt.Printf("  Providers to remove: %s\n", strings.Join(names, ", "))
	}
	fmt.Println()
}

func countChanges(table, idCol string, remoteIDs map[uint]bool) (adds, updates, removes int) {
	// Get all local IDs
	var localIDs []uint
	models.DB.Table(table).Pluck(idCol, &localIDs)

	localSet := make(map[uint]bool)
	for _, id := range localIDs {
		localSet[id] = true
	}

	for id := range remoteIDs {
		if localSet[id] {
			updates++
		} else {
			adds++
		}
	}
	for id := range localSet {
		if !remoteIDs[id] {
			removes++
		}
	}
	return
}

func getRemovalNames(table, idCol, nameCol string, remoteIDs map[uint]bool) []string {
	type row struct {
		ID   uint   `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	var rows []row
	models.DB.Table(table).Select(idCol + " as id, " + nameCol + " as name").Find(&rows)

	var names []string
	for _, r := range rows {
		if !remoteIDs[r.ID] {
			names = append(names, r.Name)
		}
	}
	return names
}

func upsertSites(sites []models.Site, environments []models.Environment, accountSites []models.AccountSite, isResync bool) int {
	remoteSiteIDs := make(map[uint]bool)
	for _, s := range sites {
		remoteSiteIDs[s.SiteID] = true
		models.UpsertSite(s)
	}
	for _, env := range environments {
		// Strip shell-dangerous values from structural fields before they reach
		// the SSH command builder (defends against malicious manager/site data).
		sanitizeEnvironment(&env)
		models.UpsertEnvironment(env, false)
	}
	remoteAccountSiteIDs := make(map[uint]bool)
	for _, as := range accountSites {
		remoteAccountSiteIDs[as.AccountSiteID] = true
		models.UpsertAccountSite(as)
	}

	// Remove orphaned sites
	if isResync {
		var localSiteIDs []uint
		models.DB.Table("captaincore_sites").Pluck("site_id", &localSiteIDs)
		for _, id := range localSiteIDs {
			if !remoteSiteIDs[id] {
				models.DeleteSiteByID(id)
			}
		}

		// Remove orphaned account_site junction rows. Without this, a site
		// reassigned between accounts on the Manager keeps its old (stale)
		// join row locally, so deploy-defaults reads the wrong account's
		// defaults instead of falling back to the site's direct account_id.
		var localAccountSiteIDs []uint
		models.DB.Table("captaincore_account_site").Pluck("account_site_id", &localAccountSiteIDs)
		for _, id := range localAccountSiteIDs {
			if !remoteAccountSiteIDs[id] {
				models.DeleteAccountSiteByID(id)
			}
		}
	}

	return len(sites)
}

func upsertAccounts(accounts []models.Account, accountUsers []models.AccountUser, accountDomains []models.AccountDomain, accountSites []models.AccountSite, isResync bool) int {
	remoteAccountIDs := make(map[uint]bool)
	for _, a := range accounts {
		remoteAccountIDs[a.AccountID] = true
		models.UpsertAccount(a)
	}
	for _, u := range accountUsers {
		models.UpsertAccountUser(u)
	}
	for _, d := range accountDomains {
		models.UpsertAccountDomain(d)
	}
	// accountSites already upserted in upsertSites — skip duplicates

	// Remove orphaned accounts
	if isResync {
		var localAccountIDs []uint
		models.DB.Table("captaincore_accounts").Pluck("account_id", &localAccountIDs)
		for _, id := range localAccountIDs {
			if !remoteAccountIDs[id] {
				models.DeleteAccountByID(id)
			}
		}
	}

	return len(accounts)
}

func upsertDomains(domains []models.Domain, isResync bool) {
	remoteDomainIDs := make(map[uint]bool)
	for _, d := range domains {
		remoteDomainIDs[d.DomainID] = true
		models.UpsertDomain(d)
	}
	if isResync {
		var localDomainIDs []uint
		models.DB.Table("captaincore_domains").Pluck("domain_id", &localDomainIDs)
		for _, id := range localDomainIDs {
			if !remoteDomainIDs[id] {
				models.DeleteDomainByID(id)
			}
		}
	}
}

func upsertProviders(providers []models.Provider, isResync bool) int {
	remoteProviderIDs := make(map[uint]bool)
	for _, p := range providers {
		remoteProviderIDs[p.ProviderID] = true
		models.UpsertProvider(p)
	}

	// Remove orphaned providers
	if isResync {
		var localProviderIDs []uint
		models.DB.Table("captaincore_providers").Pluck("provider_id", &localProviderIDs)
		for _, id := range localProviderIDs {
			if !remoteProviderIDs[id] {
				models.DeleteProviderByID(id)
			}
		}
	}

	return len(providers)
}

func updateConfigFile(resp connectResponse) (string, error) {
	action := "updated"

	// Try to load existing config
	configs, err := config.LoadConfig()
	if err != nil {
		// No existing config — create a fresh one
		action = "created"
		configs = config.FullConfig{
			{
				System: &config.SystemConfig{},
			},
			{
				CaptainID: "1",
				Keys:      make(map[string]string),
				Remotes:   make(map[string]string),
				Vars:      make(map[string]json.RawMessage),
			},
		}
	}

	// Find or create the captain entry
	captain := configs.GetCaptain("1")
	if captain == nil {
		configs = append(configs, config.CaptainConfig{
			CaptainID: "1",
			Keys:      make(map[string]string),
			Remotes:   make(map[string]string),
			Vars:      make(map[string]json.RawMessage),
		})
		captain = &configs[len(configs)-1]
	}

	// Ensure maps are initialized
	if captain.Keys == nil {
		captain.Keys = make(map[string]string)
	}
	if captain.Vars == nil {
		captain.Vars = make(map[string]json.RawMessage)
	}

	// Update token, API URL, GUI URL
	captain.Keys["token"] = resp.Token
	captain.Vars["captaincore_api"], _ = json.Marshal(resp.APIURL)
	captain.Vars["captaincore_gui"], _ = json.Marshal(resp.GUIURL)

	// Build websites string from site slugs
	var siteNames []string
	for _, s := range resp.Sites {
		if s.Site != "" && s.Status == "active" {
			siteNames = append(siteNames, s.Site)
		}
	}
	captain.Vars["websites"], _ = json.Marshal(strings.Join(siteNames, " "))

	return action, config.SaveConfig(configs)
}

func confirmPrompt(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N] ", message)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func checkDependencies() {
	type dep struct {
		name    string
		purpose string
		url     string
	}
	deps := []dep{
		{"rclone", "cloud storage sync", "https://rclone.org/install/"},
		{"restic", "encrypted backups", "https://restic.net"},
		{"git", "quicksave version tracking", "https://git-scm.com"},
	}
	var missing []dep
	for _, d := range deps {
		if _, err := exec.LookPath(d.name); err != nil {
			missing = append(missing, d)
		}
	}
	if len(missing) > 0 {
		fmt.Println("\nWarning: Missing dependencies:")
		for _, d := range missing {
			fmt.Printf("  - %s (%s) — install: %s\n", d.name, d.purpose, d.url)
		}
	}
}

// writeServerTokenFile creates or updates ~/.captaincore/data/config.json, the
// file `captaincore server` reads its accepted tokens from. Other keys in an
// existing file are preserved; the captain 1 token is set to the Manager's.
func writeServerTokenFile(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := home + "/.captaincore/data"
	path := dir + "/config.json"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	action := "created"
	existing := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		action = "updated"
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &existing); err != nil {
				return "", fmt.Errorf("existing file is not a JSON object: %w", err)
			}
		}
	}
	type tokenEntry struct {
		CaptainID string `json:"captain_id"`
		Token     string `json:"token"`
	}
	var tokens []tokenEntry
	if raw, ok := existing["tokens"]; ok {
		json.Unmarshal(raw, &tokens)
	}
	found := false
	for i := range tokens {
		if tokens[i].CaptainID == captainID {
			if tokens[i].Token == token {
				return "unchanged", nil
			}
			tokens[i].Token = token
			found = true
		}
	}
	if !found {
		tokens = append(tokens, tokenEntry{CaptainID: captainID, Token: token})
	}
	existing["tokens"], _ = json.Marshal(tokens)
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0600); err != nil {
		return "", err
	}
	return action, nil
}
