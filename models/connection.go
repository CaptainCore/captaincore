package models

// Connection stores CaptainCore connection records (migrated from WordPress options).
type Connection struct {
	ID        uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt string `gorm:"column:created_at" json:"created_at"`
	Domain    string `gorm:"column:domain;uniqueIndex" json:"domain"`
	Token     string `gorm:"column:token" json:"token"`
}

func (Connection) TableName() string {
	return "captaincore_connections"
}

// GetAllConnections returns all connections ordered by created_at descending.
func GetAllConnections() ([]Connection, error) {
	var connections []Connection
	err := DB.Order("created_at DESC").Find(&connections).Error
	return connections, err
}

// GetConnectionByDomain returns a connection matching the given domain.
func GetConnectionByDomain(domain string) (*Connection, error) {
	var conn Connection
	result := DB.Where("domain = ?", domain).First(&conn)
	if result.Error != nil {
		return nil, result.Error
	}
	return &conn, nil
}

// UpsertConnection inserts or updates a connection record by domain.
func UpsertConnection(conn Connection) error {
	var existing Connection
	result := DB.Where("domain = ?", conn.Domain).First(&existing)
	if result.Error != nil {
		return DB.Create(&conn).Error
	}
	return DB.Model(&existing).Updates(conn).Error
}
