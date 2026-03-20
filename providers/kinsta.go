package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const kinstaBaseURL = "https://api.kinsta.com/v2/"

type kinstaProvider struct{}

func init() {
	Register(&kinstaProvider{})
}

func (k *kinstaProvider) Slug() string { return "kinsta" }

func (k *kinstaProvider) RequiredCredentials() []string {
	return []string{"api_key", "company_id"}
}

// kinstaAPIKey returns the API key from credentials, checking "api_key" first
// then falling back to "api" (the name used by the Manager).
func kinstaAPIKey(credentials map[string]string) string {
	if key := credentials["api_key"]; key != "" {
		return key
	}
	return credentials["api"]
}

// kinstaParseSSHIP extracts the SSH IP address from the API response.
// Handles both the legacy string format and the new object format {"external_ip": "..."}.
func kinstaParseSSHIP(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if ip, ok := val["external_ip"].(string); ok {
			return ip
		}
	}
	return ""
}

func (k *kinstaProvider) FetchRemoteSites(credentials map[string]string) ([]RemoteSite, error) {
	apiKey := kinstaAPIKey(credentials)
	companyID := credentials["company_id"]

	body, err := kinstaGet(apiKey, fmt.Sprintf("sites?company=%s", companyID))
	if err != nil {
		return nil, fmt.Errorf("kinsta fetch sites: %w", err)
	}

	var resp struct {
		Company struct {
			Sites []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"display_name"`
				Status      string `json:"status"`
			} `json:"sites"`
		} `json:"company"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kinsta parse sites: %w", err)
	}

	var sites []RemoteSite
	for _, s := range resp.Company.Sites {
		raw, _ := json.Marshal(s)
		name := s.DisplayName
		if name == "" {
			name = s.Name
		}
		sites = append(sites, RemoteSite{
			RemoteID: s.ID,
			Name:     name,
			Domain:   s.Name,
			Status:   s.Status,
			RawData:  string(raw),
		})
	}
	return sites, nil
}

func (k *kinstaProvider) EnrichSite(credentials map[string]string, site RemoteSite) (*EnrichedSite, error) {
	apiKey := kinstaAPIKey(credentials)
	companyID := credentials["company_id"]

	enriched := &EnrichedSite{RemoteSite: site}

	// Step 1: GET sites/{remote_id} → SSH username (site name)
	siteBody, err := kinstaGet(apiKey, fmt.Sprintf("sites/%s", site.RemoteID))
	if err != nil {
		return nil, fmt.Errorf("kinsta get site: %w", err)
	}
	var siteResp struct {
		Site struct {
			Name string `json:"name"`
		} `json:"site"`
	}
	if err := json.Unmarshal(siteBody, &siteResp); err != nil {
		return nil, fmt.Errorf("kinsta parse site detail: %w", err)
	}
	enriched.SSHUsername = siteResp.Site.Name

	// Step 2: GET sites/{remote_id}/environments → find "live" env
	envsBody, err := kinstaGet(apiKey, fmt.Sprintf("sites/%s/environments", site.RemoteID))
	if err != nil {
		return nil, fmt.Errorf("kinsta get environments: %w", err)
	}
	var envsResp struct {
		Site struct {
			Environments []struct {
				ID               string `json:"id"`
				Name             string `json:"name"`
				IsPremium        bool   `json:"is_premium"`
				SSHConnection    struct {
					SSHIP   interface{} `json:"ssh_ip"`
					SSHPort json.Number `json:"ssh_port"`
				} `json:"ssh_connection"`
				PrimaryDomain struct {
					Name string `json:"name"`
				} `json:"primaryDomain"`
				ContainerInfo struct {
					PHPEngineVersion string `json:"php_engine_version"`
				} `json:"container_info"`
				WebRoot          string `json:"web_root"`
				WordpressVersion string `json:"wordpress_version"`
			} `json:"environments"`
		} `json:"site"`
	}
	if err := json.Unmarshal(envsBody, &envsResp); err != nil {
		return nil, fmt.Errorf("kinsta parse environments: %w", err)
	}

	// Determine which Kinsta environment to match.
	// CaptainCore "production" maps to Kinsta "live", "staging" maps to "staging".
	targetEnv := "live"
	if site.Environment == "staging" {
		targetEnv = "staging"
	}

	var envID string
	for _, env := range envsResp.Site.Environments {
		nameLC := strings.ToLower(env.Name)
		if nameLC == targetEnv || (targetEnv == "live" && !env.IsPremium && envID == "") {
			envID = env.ID
			enriched.SSHAddress = kinstaParseSSHIP(env.SSHConnection.SSHIP)
			enriched.SSHPort = env.SSHConnection.SSHPort.String()
			enriched.HomeURL = "https://" + env.PrimaryDomain.Name
			enriched.HomeDirectory = env.WebRoot
			enriched.WPVersion = env.WordpressVersion
			if env.PrimaryDomain.Name != "" {
				enriched.Domain = env.PrimaryDomain.Name
			}
		}
	}

	if envID == "" {
		return enriched, nil
	}

	// Step 3: GET sites/environments/{env_id}/ssh/password → SFTP password
	passBody, err := kinstaGet(apiKey, fmt.Sprintf("sites/environments/%s/ssh/password", envID))
	if err == nil {
		var passResp struct {
			Password string `json:"password"`
		}
		if json.Unmarshal(passBody, &passResp) == nil {
			enriched.SSHPassword = passResp.Password
		}
	}

	// Step 4: GET sites/environments/{env_id}/analytics/visits → monthly visits
	now := time.Now()
	start := now.AddDate(0, -1, 0)
	visitsURL := fmt.Sprintf(
		"sites/environments/%s/analytics/visits?company_id=%s&timeframe_start=%s&timeframe_end=%s",
		envID, companyID,
		start.Format("2006-01-02T15:04:05.000Z"),
		now.Format("2006-01-02T15:04:05.000Z"),
	)
	visitsBody, err := kinstaGet(apiKey, visitsURL)
	if err == nil {
		var visitsResp struct {
			App struct {
				Analytics struct {
					Visits int64 `json:"visits"`
				} `json:"analytics"`
			} `json:"app"`
		}
		if json.Unmarshal(visitsBody, &visitsResp) == nil {
			enriched.MonthlyVisits = fmt.Sprintf("%d", visitsResp.App.Analytics.Visits)
		}
	}

	return enriched, nil
}

// kinstaGet performs an authenticated GET request to the Kinsta API.
func kinstaGet(apiKey, path string) ([]byte, error) {
	req, err := http.NewRequest("GET", kinstaBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kinsta API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
