package models

// Configuration is a generic key-value store for CaptainCore settings.
type Configuration struct {
	ConfigurationID uint   `gorm:"primaryKey;column:configuration_id;autoIncrement" json:"configuration_id"`
	CaptainID       uint   `gorm:"column:captain_id" json:"captain_id"`
	ConfigKey       string `gorm:"column:config_key" json:"config_key"`
	ConfigValue     string `gorm:"column:config_value;type:text" json:"config_value"`
	CreatedAt       string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt       string `gorm:"column:updated_at" json:"updated_at"`
}

func (Configuration) TableName() string {
	return "captaincore_configurations"
}

// GetConfiguration retrieves a configuration value by captain ID and key.
func GetConfiguration(captainID uint, key string) (string, error) {
	var config Configuration
	result := DB.Where("captain_id = ? AND config_key = ?", captainID, key).First(&config)
	if result.Error != nil {
		return "", result.Error
	}
	return config.ConfigValue, nil
}

// SetConfiguration upserts a configuration value.
func SetConfiguration(captainID uint, key, value string) error {
	var config Configuration
	result := DB.Where("captain_id = ? AND config_key = ?", captainID, key).First(&config)
	if result.Error != nil {
		// Create new
		config = Configuration{
			CaptainID:   captainID,
			ConfigKey:   key,
			ConfigValue: value,
		}
		return DB.Create(&config).Error
	}
	// Update existing
	config.ConfigValue = value
	return DB.Save(&config).Error
}
