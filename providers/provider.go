package providers

import (
	"fmt"
	"sync"
)

// RemoteSite represents a site fetched from a hosting provider API.
type RemoteSite struct {
	RemoteID string `json:"remote_id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Status   string `json:"status"`
	RawData  string `json:"raw_data"`
}

// EnrichedSite extends RemoteSite with SSH/SFTP credentials and additional metadata.
type EnrichedSite struct {
	RemoteSite
	SSHUsername    string `json:"ssh_username"`
	SSHPassword   string `json:"ssh_password"`
	SSHAddress    string `json:"ssh_address"`
	SSHPort       string `json:"ssh_port"`
	HomeDirectory string `json:"home_directory"`
	HomeURL       string `json:"home_url"`
	WPVersion     string `json:"wp_version"`
	MonthlyVisits string `json:"monthly_visits"`
}

// HostingProvider is the interface that hosting provider integrations must implement.
type HostingProvider interface {
	Slug() string
	RequiredCredentials() []string
	FetchRemoteSites(credentials map[string]string) ([]RemoteSite, error)
	EnrichSite(credentials map[string]string, site RemoteSite) (*EnrichedSite, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]HostingProvider)
)

// Register adds a hosting provider to the global registry. Providers call this in init().
func Register(p HostingProvider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Slug()] = p
}

// Get returns a registered hosting provider by slug, or an error if not found.
func Get(slug string) (HostingProvider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[slug]
	if !ok {
		return nil, fmt.Errorf("unknown hosting provider: %s", slug)
	}
	return p, nil
}

// All returns all registered provider slugs.
func All() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	slugs := make([]string, 0, len(registry))
	for slug := range registry {
		slugs = append(slugs, slug)
	}
	return slugs
}
