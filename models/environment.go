package models

import "encoding/json"

// Environment mirrors captaincore_environments from the PHP schema.
type Environment struct {
	EnvironmentID        uint   `gorm:"primaryKey;column:environment_id;autoIncrement" json:"environment_id,string"`
	SiteID               uint   `gorm:"column:site_id" json:"site_id,string"`
	CreatedAt            string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt            string `gorm:"column:updated_at" json:"updated_at"`
	Environment          string `gorm:"column:environment" json:"environment"`
	Address              string `gorm:"column:address" json:"address"`
	Username             string `gorm:"column:username" json:"username"`
	Password             string `gorm:"column:password" json:"password"`
	Protocol             string `gorm:"column:protocol" json:"protocol"`
	Port                 string `gorm:"column:port" json:"port"`
	Fathom               string `gorm:"column:fathom;type:text" json:"fathom"`
	HomeDirectory        string `gorm:"column:home_directory" json:"home_directory"`
	DatabaseName         string `gorm:"column:database_name" json:"database_name"`
	DatabaseUsername     string `gorm:"column:database_username" json:"database_username"`
	DatabasePassword     string `gorm:"column:database_password" json:"database_password"`
	OffloadEnabled       string `gorm:"column:offload_enabled" json:"offload_enabled"`
	OffloadProvider      string `gorm:"column:offload_provider" json:"offload_provider"`
	OffloadAccessKey     string `gorm:"column:offload_access_key" json:"offload_access_key"`
	OffloadSecretKey     string `gorm:"column:offload_secret_key" json:"offload_secret_key"`
	OffloadBucket        string `gorm:"column:offload_bucket" json:"offload_bucket"`
	OffloadPath          string `gorm:"column:offload_path" json:"offload_path"`
	Token                string `gorm:"column:token" json:"token"`
	PHPMemory            string `gorm:"column:php_memory" json:"php_memory"`
	Storage              string `gorm:"column:storage" json:"storage"`
	Visits               string `gorm:"column:visits" json:"visits"`
	Core                 string `gorm:"column:core" json:"core"`
	CoreVerifyChecksums  string `gorm:"column:core_verify_checksums;default:'1'" json:"core_verify_checksums"`
	SubsiteCount         string `gorm:"column:subsite_count" json:"subsite_count"`
	HomeURL              string `gorm:"column:home_url" json:"home_url"`
	CapturePages         string `gorm:"column:capture_pages;type:text" json:"capture_pages"`
	Themes               string `gorm:"column:themes;type:text" json:"themes"`
	Plugins              string `gorm:"column:plugins;type:text" json:"plugins"`
	Users                string `gorm:"column:users;type:text" json:"users"`
	Details              string `gorm:"column:details;type:text" json:"details"`
	Screenshot           string `gorm:"column:screenshot" json:"screenshot"`
	MonitorEnabled       string `gorm:"column:monitor_enabled" json:"monitor_enabled"`
	UpdatesEnabled       string `gorm:"column:updates_enabled" json:"updates_enabled"`
	UpdatesExcludeThemes  string `gorm:"column:updates_exclude_themes;type:text" json:"updates_exclude_themes"`
	UpdatesExcludePlugins string `gorm:"column:updates_exclude_plugins;type:text" json:"updates_exclude_plugins"`
}

func (Environment) TableName() string {
	return "captaincore_environments"
}

// EnvironmentDetails represents the JSON stored in the environment details column.
type EnvironmentDetails struct {
	Fathom         json.RawMessage `json:"fathom"`
	Auth           *AuthDetails    `json:"auth"`
	ScreenshotBase string          `json:"screenshot_base"`
	ConsoleErrors  json.RawMessage `json:"console_errors"`
}

type AuthDetails struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ParseDetails parses the JSON details column.
func (e *Environment) ParseDetails() EnvironmentDetails {
	var d EnvironmentDetails
	if e.Details != "" {
		json.Unmarshal([]byte(e.Details), &d)
	}
	return d
}

// FindEnvironmentsBySiteID returns all environments for a given site ID.
func FindEnvironmentsBySiteID(siteID uint) ([]Environment, error) {
	var envs []Environment
	err := DB.Where("site_id = ?", siteID).Find(&envs).Error
	return envs, err
}

// UpsertEnvironment inserts or updates an environment record by environment_id.
func UpsertEnvironment(env Environment, skipUsers bool) error {
	var existing Environment
	result := DB.Where("environment_id = ?", env.EnvironmentID).First(&existing)
	if result.Error != nil {
		// Insert new
		return DB.Create(&env).Error
	}
	// Update existing - optionally skip users field
	if skipUsers {
		env.Users = existing.Users
	}
	return DB.Model(&existing).Updates(env).Error
}

// DeleteEnvironmentByID removes an environment record by its ID.
func DeleteEnvironmentByID(envID uint) error {
	return DB.Where("environment_id = ?", envID).Delete(&Environment{}).Error
}

// GetEnvironmentByID returns an environment by its ID.
func GetEnvironmentByID(id uint) (*Environment, error) {
	var env Environment
	result := DB.Where("environment_id = ?", id).First(&env)
	if result.Error != nil {
		return nil, result.Error
	}
	return &env, nil
}
