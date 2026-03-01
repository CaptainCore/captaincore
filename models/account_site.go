package models

// AccountSite mirrors captaincore_account_site junction table.
type AccountSite struct {
	AccountSiteID uint   `gorm:"primaryKey;column:account_site_id;autoIncrement" json:"account_site_id,string"`
	AccountID     uint   `gorm:"column:account_id" json:"account_id,string"`
	SiteID        uint   `gorm:"column:site_id" json:"site_id,string"`
	CreatedAt     string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     string `gorm:"column:updated_at" json:"updated_at"`
}

func (AccountSite) TableName() string {
	return "captaincore_account_site"
}

// UpsertAccountSite inserts or updates an account_site record by account_site_id.
func UpsertAccountSite(as AccountSite) error {
	var existing AccountSite
	result := DB.Where("account_site_id = ?", as.AccountSiteID).First(&existing)
	if result.Error != nil {
		// Insert new
		return DB.Create(&as).Error
	}
	// Update existing
	return DB.Model(&existing).Updates(as).Error
}
