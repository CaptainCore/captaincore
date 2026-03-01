package apiclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP calls to the CaptainCore API, replacing wp_remote_post().
type Client struct {
	APIURL    string
	Token     string
	SkipSSL   bool
	Timeout   time.Duration
}

// NewClient creates a new API client.
func NewClient(apiURL, token string, skipSSL bool) *Client {
	return &Client{
		APIURL:  apiURL,
		Token:   token,
		SkipSSL: skipSSL,
		Timeout: 30 * time.Second,
	}
}

// APIRequest represents the JSON body sent to the CaptainCore API.
type APIRequest struct {
	Command string `json:"command"`
	Token   string `json:"token"`
	SiteID  uint   `json:"site_id,omitempty"`
	AccountID uint `json:"account_id,omitempty"`
	// Additional fields can be added as json.RawMessage
	Extra map[string]interface{} `json:"-"`
}

// Post sends a POST request to the CaptainCore API.
func (c *Client) Post(command string, payload map[string]interface{}) ([]byte, error) {
	if payload == nil {
		payload = make(map[string]interface{})
	}
	payload["command"] = command
	payload["token"] = c.Token

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpClient := &http.Client{Timeout: c.Timeout}
	if c.SkipSSL {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := httpClient.Post(c.APIURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return respBody, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// PostSiteGetRaw fetches raw site data from the API.
func (c *Client) PostSiteGetRaw(siteID uint) ([]byte, error) {
	return c.Post("site-get-raw", map[string]interface{}{
		"site_id": siteID,
	})
}

// PostAccountGetRaw fetches raw account data from the API.
func (c *Client) PostAccountGetRaw(accountID uint) ([]byte, error) {
	return c.Post("account-get-raw", map[string]interface{}{
		"account_id": accountID,
	})
}

// PostProvidersListRaw fetches all provider records from the API.
func (c *Client) PostProvidersListRaw() ([]byte, error) {
	return c.Post("providers-list-raw", map[string]interface{}{})
}

// PostSyncData sends sync data to the API.
func (c *Client) PostSyncData(payload map[string]interface{}) ([]byte, error) {
	return c.Post("sync-data", payload)
}
