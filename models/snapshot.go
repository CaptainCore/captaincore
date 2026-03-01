package models

// Snapshot mirrors captaincore_snapshots from the PHP schema.
type Snapshot struct {
	SnapshotID    uint   `gorm:"primaryKey;column:snapshot_id;autoIncrement" json:"snapshot_id"`
	UserID        uint   `gorm:"column:user_id" json:"user_id"`
	SiteID        uint   `gorm:"column:site_id" json:"site_id"`
	EnvironmentID uint   `gorm:"column:environment_id" json:"environment_id"`
	CreatedAt     string `gorm:"column:created_at" json:"created_at"`
	SnapshotName  string `gorm:"column:snapshot_name" json:"snapshot_name"`
	Storage       string `gorm:"column:storage" json:"storage"`
	Email         string `gorm:"column:email" json:"email"`
	Notes         string `gorm:"column:notes;type:text" json:"notes"`
	ExpiresAt     string `gorm:"column:expires_at" json:"expires_at"`
	Token         string `gorm:"column:token" json:"token"`
}

func (Snapshot) TableName() string {
	return "captaincore_snapshots"
}

// InsertSnapshot inserts a new snapshot record and populates the auto-incremented SnapshotID.
func InsertSnapshot(snapshot *Snapshot) error {
	return DB.Create(snapshot).Error
}

// GetSnapshotByID looks up a snapshot by its ID.
func GetSnapshotByID(id uint) (*Snapshot, error) {
	var snapshot Snapshot
	result := DB.Where("snapshot_id = ?", id).First(&snapshot)
	if result.Error != nil {
		return nil, result.Error
	}
	return &snapshot, nil
}

// CountSnapshotsByEnvironmentID returns the number of snapshots for an environment.
func CountSnapshotsByEnvironmentID(envID uint) (int64, error) {
	var count int64
	err := DB.Model(&Snapshot{}).Where("environment_id = ?", envID).Count(&count).Error
	return count, err
}

// GetSnapshotsByEnvironmentID returns snapshots for an environment, ordered by created_at DESC.
func GetSnapshotsByEnvironmentID(envID uint, limit int) ([]Snapshot, error) {
	var snapshots []Snapshot
	err := DB.Where("environment_id = ?", envID).
		Order("created_at DESC").
		Limit(limit).
		Find(&snapshots).Error
	return snapshots, err
}
