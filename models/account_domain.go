package models

// AccountDomain mirrors captaincore_account_domain junction table.
type AccountDomain struct {
	AccountDomainID uint   `gorm:"primaryKey;column:account_domain_id;autoIncrement" json:"account_domain_id,string"`
	AccountID       uint   `gorm:"column:account_id" json:"account_id,string"`
	DomainID        uint   `gorm:"column:domain_id" json:"domain_id,string"`
	CreatedAt       string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       string `gorm:"column:updated_at" json:"updated_at"`
}

func (AccountDomain) TableName() string {
	return "captaincore_account_domain"
}

// UpsertAccountDomain inserts or updates an account_domain record by account_domain_id.
func UpsertAccountDomain(ad AccountDomain) error {
	var existing AccountDomain
	result := DB.Where("account_domain_id = ?", ad.AccountDomainID).First(&existing)
	if result.Error != nil {
		return DB.Create(&ad).Error
	}
	return DB.Model(&existing).Updates(ad).Error
}
