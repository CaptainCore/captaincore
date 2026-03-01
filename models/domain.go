package models

// Domain mirrors captaincore_domains from the PHP schema.
type Domain struct {
	DomainID         uint   `gorm:"primaryKey;column:domain_id;autoIncrement" json:"domain_id,string"`
	RemoteID         string `gorm:"column:remote_id" json:"remote_id"`
	ProviderID       string `gorm:"column:provider_id" json:"provider_id"`
	ProviderDomainID string `gorm:"column:provider_domain_id" json:"provider_domain_id"`
	Status           string `gorm:"column:status" json:"status"`
	Price            string `gorm:"column:price" json:"price"`
	Name             string `gorm:"column:name" json:"name"`
	Details          string `gorm:"column:details;type:text" json:"details"`
	CreatedAt        string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        string `gorm:"column:updated_at" json:"updated_at"`
}

func (Domain) TableName() string {
	return "captaincore_domains"
}

// UpsertDomain inserts or updates a domain record by domain_id.
func UpsertDomain(d Domain) error {
	var existing Domain
	result := DB.Where("domain_id = ?", d.DomainID).First(&existing)
	if result.Error != nil {
		return DB.Create(&d).Error
	}
	return DB.Model(&existing).Updates(d).Error
}

// DeleteDomainByID removes a domain record by its ID.
func DeleteDomainByID(id uint) error {
	DB.Where("domain_id = ?", id).Delete(&AccountDomain{})
	return DB.Where("domain_id = ?", id).Delete(&Domain{}).Error
}
