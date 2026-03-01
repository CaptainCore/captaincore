package models

// Key mirrors captaincore_keys from the PHP schema.
type Key struct {
	KeyID       uint   `gorm:"primaryKey;column:key_id;autoIncrement" json:"key_id"`
	UserID      uint   `gorm:"column:user_id" json:"user_id"`
	Title       string `gorm:"column:title" json:"title"`
	Fingerprint string `gorm:"column:fingerprint" json:"fingerprint"`
	Main        bool   `gorm:"column:main" json:"main"`
	CreatedAt   string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   string `gorm:"column:updated_at" json:"updated_at"`
}

func (Key) TableName() string {
	return "captaincore_keys"
}
