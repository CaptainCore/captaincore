package models

// Account mirrors captaincore_accounts from the PHP schema.
type Account struct {
	AccountID     uint   `gorm:"primaryKey;column:account_id;autoIncrement" json:"account_id,string"`
	BillingUserID uint   `gorm:"column:billing_user_id" json:"billing_user_id,string"`
	Name          string `gorm:"column:name" json:"name"`
	Defaults      string `gorm:"column:defaults;type:text" json:"defaults"`
	Plan          string `gorm:"column:plan;type:text" json:"plan"`
	Metrics       string `gorm:"column:metrics" json:"metrics"`
	Status        string `gorm:"column:status" json:"status"`
	CreatedAt     string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     string `gorm:"column:updated_at" json:"updated_at"`
}

func (Account) TableName() string {
	return "captaincore_accounts"
}

// GetAccountByID returns an account by its ID.
func GetAccountByID(id uint) (*Account, error) {
	var account Account
	result := DB.Where("account_id = ?", id).First(&account)
	if result.Error != nil {
		return nil, result.Error
	}
	return &account, nil
}

// UpsertAccount inserts or updates an account record by account_id.
func UpsertAccount(account Account) error {
	var existing Account
	result := DB.Where("account_id = ?", account.AccountID).First(&existing)
	if result.Error != nil {
		return DB.Create(&account).Error
	}
	return DB.Model(&existing).Updates(account).Error
}

// DeleteAccountByID removes an account and its related junction table records.
func DeleteAccountByID(accountID uint) error {
	DB.Where("account_id = ?", accountID).Delete(&AccountSite{})
	DB.Where("account_id = ?", accountID).Delete(&AccountDomain{})
	DB.Where("account_id = ?", accountID).Delete(&AccountUser{})
	return DB.Where("account_id = ?", accountID).Delete(&Account{}).Error
}
