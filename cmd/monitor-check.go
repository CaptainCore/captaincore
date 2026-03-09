package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// MonitorCheckResult holds the outcome of a single health check.
type MonitorCheckResult struct {
	HTTPCode     string `json:"http_code"`
	NumRedirects string `json:"num_redirects"`
	URL          string `json:"url"`
	Name         string `json:"name"`
	HTMLValid    string `json:"html_valid"`
	Error        string `json:"error,omitempty"`
}

// sharedTransport is a shared HTTP transport for all monitor checks.
// Reusing a single transport avoids exhausting local sockets/ports
// when running many concurrent checks.
var sharedTransport = &http.Transport{
	TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
	MaxIdleConns:         100,
	MaxIdleConnsPerHost:  2,
	MaxConnsPerHost:      4,
	IdleConnTimeout:      30 * time.Second,
	DisableKeepAlives:    true,
	TLSHandshakeTimeout: 10 * time.Second,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 0,
	}).DialContext,
}

// retryTransport uses 1.1.1.1 (Cloudflare DNS) directly, bypassing the
// local system resolver. Used on retries to rule out local DNS issues.
var retryTransport = &http.Transport{
	TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
	MaxIdleConns:         100,
	MaxIdleConnsPerHost:  2,
	MaxConnsPerHost:      4,
	IdleConnTimeout:      30 * time.Second,
	DisableKeepAlives:    true,
	TLSHandshakeTimeout: 10 * time.Second,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 0,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", "1.1.1.1:53")
			},
		},
	}).DialContext,
}

// monitorCheckSingle performs an HTTP health check on a single URL.
// It is safe to call from goroutines — no shared state is accessed.
// HTTP URLs are upgraded to HTTPS first; if the connection fails, it falls back to HTTP.
// The timeout parameter controls how long to wait for a response (0 uses the default 15s).
// The transport parameter selects the HTTP transport (nil uses sharedTransport).
func monitorCheckSingle(url, name string, timeout time.Duration, transport *http.Transport) MonitorCheckResult {
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	if transport == nil {
		transport = sharedTransport
	}

	// Normalize: try HTTPS first for http:// URLs
	originalURL := url
	upgraded := false
	if strings.HasPrefix(url, "http://") {
		url = "https://" + strings.TrimPrefix(url, "http://")
		upgraded = true
	}

	result := doHTTPCheck(url, name, timeout, transport)

	// If we upgraded to HTTPS and the connection failed entirely (000),
	// fall back to the original HTTP URL.
	if upgraded && result.HTTPCode == "000" {
		result = doHTTPCheck(originalURL, name, timeout, transport)
	}

	return result
}

// doHTTPCheck performs the actual HTTP request and response validation.
func doHTTPCheck(url, name string, timeout time.Duration, transport *http.Transport) MonitorCheckResult {
	result := MonitorCheckResult{
		URL:          url,
		Name:         name,
		HTTPCode:     "000",
		HTMLValid:    "false",
		NumRedirects: "0",
	}

	redirectCount := 0
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 20 {
				return http.ErrUseLastResponse
			}
			redirectCount++
			return nil
		},
		Transport: transport,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "captaincore/1.0 (CaptainCore Health Check by CaptainCore.io)")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.NumRedirects = fmt.Sprintf("%d", redirectCount)
		return result
	}
	defer resp.Body.Close()

	result.HTTPCode = fmt.Sprintf("%d", resp.StatusCode)
	result.NumRedirects = fmt.Sprintf("%d", redirectCount)

	// Stream the entire body keeping only the last 4KB in a ring buffer.
	// This handles pages of any size (even multi-MB) without excessive
	// memory use while still catching the closing </html> tag at the end.
	const tailSize = 4096
	ring := make([]byte, tailSize)
	ringPos := 0
	totalRead := 0
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		for i := 0; i < n; i++ {
			ring[ringPos] = tmp[i]
			ringPos = (ringPos + 1) % tailSize
		}
		totalRead += n
		if err != nil {
			break
		}
	}

	// Reconstruct the tail from the ring buffer
	var tail string
	if totalRead <= tailSize {
		tail = string(ring[:totalRead])
	} else {
		tail = string(ring[ringPos:]) + string(ring[:ringPos])
	}

	if strings.Contains(strings.ToLower(tail), "</html>") {
		result.HTMLValid = "true"
	}

	return result
}

// monitorCheckNative is the native Go handler for the monitor-check command.
func monitorCheckNative(cmd *cobra.Command, args []string) {
	parts := strings.SplitN(args[0], ",", 2)
	url := parts[0]
	name := ""
	if len(parts) > 1 {
		name = parts[1]
	}

	result := monitorCheckSingle(url, name, 0, nil)

	out, _ := json.Marshal(result)
	fmt.Println(string(out))
}

var monitorCheckCmd = &cobra.Command{
	Use:   "monitor-check <url,name>",
	Short: "Monitor check on a single valid HTTP url",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a <url,name> argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		monitorCheckNative(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(monitorCheckCmd)
}
