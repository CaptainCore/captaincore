package models

// AccountUser mirrors captaincore_account_user junction table.
type AccountUser struct {
	AccountUserID uint   `gorm:"primaryKey;column:account_user_id;autoIncrement" json:"account_user_id,string"`
	AccountID     uint   `gorm:"column:account_id" json:"account_id,string"`
	UserID        uint   `gorm:"column:user_id" json:"user_id,string"`
	Level         string `gorm:"column:level" json:"level"`
	CreatedAt     string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     string `gorm:"column:updated_at" json:"updated_at"`
}

func (AccountUser) TableName() string {
	return "captaincore_account_user"
}

// UpsertAccountUser inserts or updates an account_user record by account_user_id.
func UpsertAccountUser(au AccountUser) error {
	var existing AccountUser
	result := DB.Where("account_user_id = ?", au.AccountUserID).First(&existing)
	if result.Error != nil {
		return DB.Create(&au).Error
	}
	return DB.Model(&existing).Updates(au).Error
}
