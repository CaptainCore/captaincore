package models

import (
	"encoding/json"
	"strings"
)

// Site mirrors captaincore_sites from the PHP schema.
type Site struct {
	SiteID         uint   `gorm:"primaryKey;column:site_id;autoIncrement" json:"site_id,string"`
	AccountID      uint   `gorm:"column:account_id" json:"account_id,string"`
	CustomerID     uint   `gorm:"column:customer_id" json:"customer_id,string"`
	Name           string `gorm:"column:name" json:"name"`
	Site           string `gorm:"column:site" json:"site"`
	ProviderID     string `gorm:"column:provider_id" json:"provider_id"`
	ProviderSiteID string `gorm:"column:provider_site_id" json:"provider_site_id"`
	Provider       string `gorm:"column:provider" json:"provider"`
	Token          string `gorm:"column:token" json:"token"`
	Status         string `gorm:"column:status" json:"status"`
	Details        string `gorm:"column:details;type:text" json:"details"`
	Screenshot     string `gorm:"column:screenshot" json:"screenshot"`
	CreatedAt      string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      string `gorm:"column:updated_at" json:"updated_at"`
}

func (Site) TableName() string {
	return "captaincore_sites"
}

// SiteDetails represents the JSON stored in the details column.
type SiteDetails struct {
	Key            string          `json:"key"`
	EnvironmentVars json.RawMessage `json:"environment_vars"`
	Subsites       json.RawMessage `json:"subsites"`
	Storage        json.RawMessage `json:"storage"`
	Visits         json.RawMessage `json:"visits"`
	Mailgun        json.RawMessage `json:"mailgun"`
	Core           json.RawMessage `json:"core"`
	Verify         json.RawMessage `json:"verify"`
	RemoteKey      string          `json:"remote_key"`
	BackupSettings *BackupSettings `json:"backup_settings"`
	ScreenshotBase string          `json:"screenshot_base"`
	Removed        json.RawMessage `json:"removed"`
	ConsoleErrors  json.RawMessage `json:"console_errors"`
}

type BackupSettings struct {
	Mode     string `json:"mode"`
	Interval string `json:"interval"`
	Active   bool   `json:"active"`
}

// ParseDetails parses the JSON details column into a SiteDetails struct.
func (s *Site) ParseDetails() SiteDetails {
	var d SiteDetails
	if s.Details != "" {
		json.Unmarshal([]byte(s.Details), &d)
	}
	if d.BackupSettings == nil {
		d.BackupSettings = &BackupSettings{Mode: "direct", Interval: "daily", Active: true}
	}
	return d
}

// GetSiteByName looks up an active site by its slug name.
func GetSiteByName(name string) (*Site, error) {
	var site Site
	result := DB.Where("site = ? AND status = ?", name, "active").First(&site)
	if result.Error != nil {
		return nil, result.Error
	}
	return &site, nil
}

// GetSiteByID looks up a site by its ID.
func GetSiteByID(id uint) (*Site, error) {
	var site Site
	result := DB.Where("site_id = ?", id).First(&site)
	if result.Error != nil {
		return nil, result.Error
	}
	return &site, nil
}

// GetSiteByNameAndProvider looks up an active site by slug and provider.
func GetSiteByNameAndProvider(name, provider string) (*Site, error) {
	var site Site
	result := DB.Where("site = ? AND provider = ? AND status = ?", name, provider, "active").First(&site)
	if result.Error != nil {
		return nil, result.Error
	}
	return &site, nil
}

// GetSiteByProviderSiteID looks up a site by its provider_site_id and provider slug.
func GetSiteByProviderSiteID(providerSiteID, provider string) (*Site, error) {
	var site Site
	result := DB.Where("provider_site_id = ? AND provider = ? AND status = ?", providerSiteID, provider, "active").First(&site)
	if result.Error != nil {
		return nil, result.Error
	}
	return &site, nil
}

// UpsertSite inserts or updates a site record by site_id.
func UpsertSite(site Site) error {
	var existing Site
	result := DB.Where("site_id = ?", site.SiteID).First(&existing)
	if result.Error != nil {
		// Insert new
		return DB.Create(&site).Error
	}
	// Update existing
	return DB.Model(&existing).Updates(site).Error
}

// FetchSiteMatchingArgs holds the arguments for FetchSitesMatching.
type FetchSiteMatchingArgs struct {
	Environment string
	Provider    string
	Field       string
	Targets     []string
	Filter      *SiteFilter
}

// SiteFilter holds filter criteria for site listing.
type SiteFilter struct {
	Type    string
	Name    string
	Version string
	Status  string
}

// SiteEnvironmentResult is a row returned by FetchSitesMatching.
type SiteEnvironmentResult struct {
	Site        string `gorm:"column:site"`
	SiteID      uint   `gorm:"column:site_id"`
	Environment string `gorm:"column:environment"`
	// Dynamic field values (populated by extra SELECT columns)
	Name                  string `gorm:"column:name"`
	Provider              string `gorm:"column:provider"`
	HomeURL               string `gorm:"column:home_url"`
	Address               string `gorm:"column:address"`
	Username              string `gorm:"column:username"`
	Port                  string `gorm:"column:port"`
	Core                  string `gorm:"column:core"`
	Themes                string `gorm:"column:themes"`
	Plugins               string `gorm:"column:plugins"`
	UpdatesEnabled        string `gorm:"column:updates_enabled"`
	MonitorEnabled        string `gorm:"column:monitor_enabled"`
	Storage               string `gorm:"column:storage"`
	Visits                string `gorm:"column:visits"`
	HomeDirField          string `gorm:"column:home_directory"`
	DatabaseUsername      string `gorm:"column:database_username"`
	DatabasePassword      string `gorm:"column:database_password"`
	UpdatesExcludeThemes  string `gorm:"column:updates_exclude_themes"`
	UpdatesExcludePlugins string `gorm:"column:updates_exclude_plugins"`
}

// FetchSitesMatching replicates the PHP Sites::fetch_sites_matching() method.
// It queries sites joined with environments, applying provider/environment/target filters.
func FetchSitesMatching(args FetchSiteMatchingArgs) ([]SiteEnvironmentResult, error) {
	var results []SiteEnvironmentResult

	query := DB.Table("captaincore_sites").
		Select("captaincore_sites.site, captaincore_sites.site_id, captaincore_sites.name, captaincore_sites.provider, captaincore_environments.environment, captaincore_environments.home_url, captaincore_environments.address, captaincore_environments.username, captaincore_environments.port, captaincore_environments.core, captaincore_environments.themes, captaincore_environments.plugins, captaincore_environments.updates_enabled, captaincore_environments.monitor_enabled, captaincore_environments.storage, captaincore_environments.visits, captaincore_environments.home_directory, captaincore_environments.database_username, captaincore_environments.database_password, captaincore_environments.updates_exclude_themes, captaincore_environments.updates_exclude_plugins").
		Joins("INNER JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ?", "active")

	// Provider filter
	if args.Provider != "" {
		query = query.Where("captaincore_sites.provider = ?", args.Provider)
	}

	// Environment filter
	if args.Environment != "" && args.Environment != "all" {
		query = query.Where("captaincore_environments.environment = ?", args.Environment)
	}

	// Target conditions
	for _, target := range args.Targets {
		switch target {
		case "updates-on":
			query = query.Where("captaincore_environments.updates_enabled = ?", "1")
		case "updates-off":
			query = query.Where("captaincore_environments.updates_enabled = ?", "0")
		case "offload-on":
			query = query.Where("captaincore_environments.offload_enabled = ?", "1")
		case "offload-off":
			query = query.Where("captaincore_environments.offload_enabled = ?", "0")
		case "monitor-on":
			query = query.Where("captaincore_environments.monitor_enabled = ?", "1")
		case "backup-local":
			query = query.Where("json_extract(captaincore_sites.details, '$.backup_settings.mode') = ?", "local")
		case "backup-remote":
			query = query.Where("(json_extract(captaincore_sites.details, '$.backup_settings.mode') = ? OR json_extract(captaincore_sites.details, '$.backup_settings.mode') IS NULL)", "direct")
		}
	}

	// Core version filter
	if args.Filter != nil && args.Filter.Type == "core" && args.Filter.Version != "" {
		version := args.Filter.Version
		if strings.HasPrefix(version, "^") {
			query = query.Where("captaincore_environments.core != ?", strings.TrimPrefix(version, "^"))
		} else {
			query = query.Where("captaincore_environments.core = ?", version)
		}
	}

	// Plugin/theme filter: SQL LIKE for initial narrowing, post-filter for precise matching
	if args.Filter != nil && args.Filter.Type != "" && args.Filter.Type != "core" {
		filterName := strings.TrimPrefix(args.Filter.Name, "^")
		if filterName == "" {
			filterName = "%"
		}
		// Only use non-negated name in SQL LIKE for pre-filtering
		if !strings.HasPrefix(args.Filter.Name, "^") {
			pattern := `%"name":"` + filterName + `"%`
			query = query.Where("captaincore_environments."+args.Filter.Type+" LIKE ?", pattern)
		}
	}

	query = query.Order("captaincore_sites.name ASC")

	err := query.Find(&results).Error
	if err != nil {
		return nil, err
	}

	// Post-filter for plugin/theme name, version, and status (supports ^ negation)
	if args.Filter != nil && args.Filter.Type != "" && args.Filter.Type != "core" &&
		(args.Filter.Name != "" || args.Filter.Version != "" || args.Filter.Status != "") {
		results = filterResults(results, args.Filter)
	}

	return results, nil
}

// matchField checks if a field value matches a filter value.
// Filter values prefixed with "^" are negated (field must NOT equal the value).
func matchField(fieldValue interface{}, filterValue string) bool {
	if filterValue == "" {
		return true
	}
	fieldStr, _ := fieldValue.(string)
	if strings.HasPrefix(filterValue, "^") {
		return fieldStr != strings.TrimPrefix(filterValue, "^")
	}
	return fieldStr == filterValue
}

// filterResults does post-query filtering for plugin/theme name, version, and status.
// All filter values support "!" prefix for negation.
func filterResults(results []SiteEnvironmentResult, filter *SiteFilter) []SiteEnvironmentResult {
	var filtered []SiteEnvironmentResult
	for _, r := range results {
		var jsonData string
		if filter.Type == "plugins" {
			jsonData = r.Plugins
		} else if filter.Type == "themes" {
			jsonData = r.Themes
		}
		if jsonData == "" {
			continue
		}
		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &items); err != nil {
			continue
		}
		for _, item := range items {
			if matchField(item["name"], filter.Name) &&
				matchField(item["version"], filter.Version) &&
				matchField(item["status"], filter.Status) {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}

// DeleteSiteByID removes a site and its related environments and account_site records.
func DeleteSiteByID(siteID uint) error {
	DB.Where("site_id = ?", siteID).Delete(&Environment{})
	DB.Where("site_id = ?", siteID).Delete(&AccountSite{})
	return DB.Where("site_id = ?", siteID).Delete(&Site{}).Error
}

// SearchSites searches active sites by LIKE matching on site/name/address columns.
func SearchSites(search, searchField string) ([]Site, error) {
	var sites []Site
	pattern := "%" + search + "%"

	if searchField == "address" {
		err := DB.Table("captaincore_sites").
			Select("DISTINCT captaincore_sites.*").
			Joins("INNER JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
			Where("captaincore_sites.status = ? AND captaincore_environments.address LIKE ?", "active", pattern).
			Order("captaincore_sites.name ASC").
			Find(&sites).Error
		return sites, err
	}

	if searchField != "" {
		// Map user-facing field names to actual database column names
		dbField := searchField
		if searchField == "domain" {
			dbField = "name"
		}
		err := DB.Where("status = ? AND "+dbField+" LIKE ?", "active", pattern).
			Order("name ASC").
			Find(&sites).Error
		return sites, err
	}

	// Default: search site (slug), name (domain), and address columns
	err := DB.Table("captaincore_sites").
		Select("DISTINCT captaincore_sites.*").
		Joins("LEFT JOIN captaincore_environments ON captaincore_sites.site_id = captaincore_environments.site_id").
		Where("captaincore_sites.status = ? AND (captaincore_sites.site LIKE ? OR captaincore_sites.name LIKE ? OR captaincore_environments.address LIKE ?)", "active", pattern, pattern, pattern).
		Order("captaincore_sites.name ASC").
		Find(&sites).Error
	return sites, err
}

// GetAllActiveSites returns all active sites.
func GetAllActiveSites() ([]Site, error) {
	var sites []Site
	err := DB.Where("status = ?", "active").Order("name ASC").Find(&sites).Error
	return sites, err
}

// ParseTargetString parses a target string like "@production.monitor-on" into
// environment and minor target components.
func ParseTargetString(target string) (environment string, minorTargets []string) {
	parts := strings.Split(target, ".")
	for _, p := range parts {
		switch p {
		case "@production":
			environment = "Production"
		case "@staging":
			environment = "Staging"
		case "@all":
			environment = "all"
		case "monitor-on", "updates-on", "updates-off", "offload-on", "offload-off", "backup-local", "backup-remote":
			minorTargets = append(minorTargets, p)
		}
	}
	return
}
