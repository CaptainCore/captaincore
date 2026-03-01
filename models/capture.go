package models

// Capture mirrors captaincore_captures from the PHP schema.
type Capture struct {
	CaptureID     uint   `gorm:"primaryKey;column:capture_id;autoIncrement" json:"capture_id"`
	SiteID        uint   `gorm:"column:site_id" json:"site_id"`
	EnvironmentID uint   `gorm:"column:environment_id" json:"environment_id"`
	CreatedAt     string `gorm:"column:created_at" json:"created_at"`
	GitCommit     string `gorm:"column:git_commit" json:"git_commit"`
	Pages         string `gorm:"column:pages;type:text" json:"pages"`
}

func (Capture) TableName() string {
	return "captaincore_captures"
}
