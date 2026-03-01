package models

import "encoding/json"

// Provider stores hosting provider API credentials (e.g. Kinsta, GridPane).
type Provider struct {
	ProviderID     uint   `gorm:"primaryKey;column:provider_id;autoIncrement" json:"provider_id,string"`
	UserID         uint   `gorm:"column:user_id" json:"user_id,string"`
	Name           string `gorm:"column:name" json:"name"`
	Provider       string `gorm:"column:provider" json:"provider"`
	Status         string `gorm:"column:status" json:"status"`
	Details        string `gorm:"column:details;type:text" json:"details"`
	Credentials    string `gorm:"column:credentials;type:text" json:"credentials"`
	Configurations string `gorm:"column:configurations;type:text" json:"configurations"`
	CreatedAt      string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      string `gorm:"column:updated_at" json:"updated_at"`
}

func (Provider) TableName() string {
	return "captaincore_providers"
}

// credentialEntry represents a single name/value pair in the credentials JSON array.
type credentialEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetCredential looks up a credential by name from the JSON array stored in Credentials.
func (p *Provider) GetCredential(name string) string {
	var creds []credentialEntry
	if p.Credentials == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(p.Credentials), &creds); err != nil {
		return ""
	}
	for _, c := range creds {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// GetCredentialsMap returns all credentials as a map of name→value.
func (p *Provider) GetCredentialsMap() map[string]string {
	m := make(map[string]string)
	var creds []credentialEntry
	if p.Credentials == "" {
		return m
	}
	if err := json.Unmarshal([]byte(p.Credentials), &creds); err != nil {
		return m
	}
	for _, c := range creds {
		m[c.Name] = c.Value
	}
	return m
}

// GetAllProviders returns all providers ordered by created_at descending.
func GetAllProviders() ([]Provider, error) {
	var providers []Provider
	err := DB.Order("created_at DESC").Find(&providers).Error
	return providers, err
}

// GetProviderByID returns a provider by its ID.
func GetProviderByID(id uint) (*Provider, error) {
	var p Provider
	result := DB.Where("provider_id = ?", id).First(&p)
	if result.Error != nil {
		return nil, result.Error
	}
	return &p, nil
}

// UpsertProvider inserts or updates a provider record by provider_id.
func UpsertProvider(p Provider) error {
	var existing Provider
	result := DB.Where("provider_id = ?", p.ProviderID).First(&existing)
	if result.Error != nil {
		return DB.Create(&p).Error
	}
	return DB.Model(&existing).Updates(p).Error
}

// DeleteProviderByID removes a provider record by its ID.
func DeleteProviderByID(id uint) error {
	return DB.Where("provider_id = ?", id).Delete(&Provider{}).Error
}
