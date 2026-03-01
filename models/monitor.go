package models

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MonitorDB is a separate database connection for monitor_stats.db.
var MonitorDB *gorm.DB

// MonitorRun represents a row in the "runs" table of monitor_stats.db.
type MonitorRun struct {
	ID          uint    `gorm:"primaryKey;autoIncrement"`
	Command     string  `gorm:"column:command"`
	StartTime   int64   `gorm:"column:start_time"`
	EndTime     int64   `gorm:"column:end_time"`
	Duration    int64   `gorm:"column:duration"`
	SiteCount   int     `gorm:"column:site_count"`
	Parallelism int     `gorm:"column:parallelism"`
	SystemLoad       float64 `gorm:"column:system_load"`
	Status           string  `gorm:"column:status"`
	ErrorCount       int     `gorm:"column:error_count"`
	RetryAttempts    int     `gorm:"column:retry_attempts"`
	NotificationSent bool    `gorm:"column:notification_sent"`
	RestoredCount    int     `gorm:"column:restored_count"`
}

func (MonitorRun) TableName() string {
	return "runs"
}

// InitMonitorDB opens (or creates) the SQLite database at data/monitor_stats.db
// and runs AutoMigrate for the MonitorRun model.
func InitMonitorDB() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dbPath := filepath.Join(home, ".captaincore", "data", "monitor_stats.db")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	MonitorDB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return err
	}

	MonitorDB.Exec("PRAGMA journal_mode = WAL")
	MonitorDB.Exec("PRAGMA busy_timeout = 30000")
	MonitorDB.Exec("PRAGMA synchronous = NORMAL")

	sqlDB, err := MonitorDB.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)

	return MonitorDB.AutoMigrate(&MonitorRun{})
}

// GetSystemLoad returns the 1-minute system load average.
func GetSystemLoad() float64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
		if err != nil {
			return 0
		}
		// Output looks like "{ 1.23 4.56 7.89 }"
		fields := strings.Fields(strings.Trim(string(out), "{ }\n"))
		if len(fields) >= 1 {
			var load float64
			fmt.Sscanf(fields[0], "%f", &load)
			return load
		}
	default:
		data, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return 0
		}
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			var load float64
			fmt.Sscanf(fields[0], "%f", &load)
			return load
		}
	}
	return 0
}

// LogMonitorRunStart inserts a new run record with status "running" and returns its ID.
func LogMonitorRunStart(command string, parallelism int) uint {
	if MonitorDB == nil {
		return 0
	}
	run := MonitorRun{
		Command:     command,
		StartTime:   time.Now().Unix(),
		Parallelism: parallelism,
		SystemLoad:  GetSystemLoad(),
		Status:      "running",
	}
	MonitorDB.Create(&run)
	return run.ID
}

// LogMonitorRunEnd updates a run record with end time, duration, site count, and completed status.
func LogMonitorRunEnd(runID uint, siteCount, errorCount, retryAttempts, restoredCount int, notificationSent bool) {
	if MonitorDB == nil || runID == 0 {
		return
	}
	now := time.Now().Unix()
	MonitorDB.Model(&MonitorRun{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"end_time":          now,
		"duration":          gorm.Expr("? - start_time", now),
		"site_count":        siteCount,
		"status":            "completed",
		"error_count":       errorCount,
		"retry_attempts":    retryAttempts,
		"notification_sent": notificationSent,
		"restored_count":    restoredCount,
	})
}

// LogMonitorRunSkipped inserts a run record with status "skipped_locked".
func LogMonitorRunSkipped(parallelism int) {
	if MonitorDB == nil {
		return
	}
	now := time.Now().Unix()
	run := MonitorRun{
		Command:     "monitor",
		StartTime:   now,
		EndTime:     now,
		Duration:    0,
		Parallelism: parallelism,
		SystemLoad:  GetSystemLoad(),
		Status:      "skipped_locked",
	}
	MonitorDB.Create(&run)
}
