package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const gridpaneBaseURL = "https://my.gridpane.com/oauth/api/v1/"

type gridpaneProvider struct {
	systemUsersOnce sync.Once
	systemUsers     []gridpaneSystemUser
	systemUsersErr  error
}

type gridpaneSystemUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
	ServerID int    `json:"server_id"`
}

type gridpaneServer struct {
	ID int    `json:"id"`
	IP string `json:"ip"`
}

func init() {
	Register(&gridpaneProvider{})
}

func (g *gridpaneProvider) Slug() string { return "gridpane" }

func (g *gridpaneProvider) RequiredCredentials() []string {
	return []string{"api_key"}
}

func (g *gridpaneProvider) FetchRemoteSites(credentials map[string]string) ([]RemoteSite, error) {
	apiKey := credentials["api_key"]

	var allSites []RemoteSite
	page := 1

	for {
		body, err := gridpaneGet(apiKey, fmt.Sprintf("site?per_page=200&page=%d", page))
		if err != nil {
			return nil, fmt.Errorf("gridpane fetch sites page %d: %w", page, err)
		}

		var resp struct {
			Data []struct {
				ID           int    `json:"id"`
				Label        string `json:"label"`
				URL          string `json:"url"`
				SystemUserID int    `json:"system_user_id"`
				ServerID     int    `json:"server_id"`
			} `json:"data"`
			LastPage int `json:"last_page"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("gridpane parse sites: %w", err)
		}

		for _, s := range resp.Data {
			raw, _ := json.Marshal(s)
			name := s.Label
			if name == "" {
				name = s.URL
			}
			allSites = append(allSites, RemoteSite{
				RemoteID: fmt.Sprintf("%d", s.ID),
				Name:     name,
				Domain:   s.URL,
				Status:   "active",
				RawData:  string(raw),
			})
		}

		if page >= resp.LastPage {
			break
		}
		page++
	}

	return allSites, nil
}

func (g *gridpaneProvider) EnrichSite(credentials map[string]string, site RemoteSite) (*EnrichedSite, error) {
	apiKey := credentials["api_key"]
	enriched := &EnrichedSite{RemoteSite: site}

	// Parse the raw site data to get system_user_id and server_id
	var siteData struct {
		SystemUserID int `json:"system_user_id"`
		ServerID     int `json:"server_id"`
	}
	if err := json.Unmarshal([]byte(site.RawData), &siteData); err != nil {
		return nil, fmt.Errorf("gridpane parse site raw data: %w", err)
	}

	// Fetch system users (cached per session)
	g.systemUsersOnce.Do(func() {
		g.systemUsers, g.systemUsersErr = g.fetchAllSystemUsers(apiKey)
	})
	if g.systemUsersErr != nil {
		return nil, fmt.Errorf("gridpane fetch system users: %w", g.systemUsersErr)
	}

	// Match system user
	for _, su := range g.systemUsers {
		if su.ID == siteData.SystemUserID {
			enriched.SSHUsername = su.Username
			enriched.SSHPassword = su.Password
			break
		}
	}

	// Fetch server IP
	serverBody, err := gridpaneGet(apiKey, fmt.Sprintf("server/%d", siteData.ServerID))
	if err == nil {
		var serverResp struct {
			Data gridpaneServer `json:"data"`
		}
		if json.Unmarshal(serverBody, &serverResp) == nil {
			enriched.SSHAddress = serverResp.Data.IP
		}
	}

	enriched.SSHPort = "22"
	enriched.HomeDirectory = "/var/www/" + site.Domain + "/htdocs"
	enriched.HomeURL = "https://" + site.Domain

	return enriched, nil
}

func (g *gridpaneProvider) fetchAllSystemUsers(apiKey string) ([]gridpaneSystemUser, error) {
	var allUsers []gridpaneSystemUser
	page := 1

	for {
		body, err := gridpaneGet(apiKey, fmt.Sprintf("system-user?per_page=200&page=%d", page))
		if err != nil {
			return nil, err
		}

		var resp struct {
			Data     []gridpaneSystemUser `json:"data"`
			LastPage int                  `json:"last_page"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}

		allUsers = append(allUsers, resp.Data...)

		if page >= resp.LastPage {
			break
		}
		page++
	}

	return allUsers, nil
}

// gridpaneGet performs an authenticated GET request to the GridPane API.
func gridpaneGet(apiKey, path string) ([]byte, error) {
	req, err := http.NewRequest("GET", gridpaneBaseURL+path, nil)
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
		return nil, fmt.Errorf("gridpane API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
