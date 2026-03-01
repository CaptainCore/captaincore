package models

import (
	"log"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database connection used by all model queries.
var DB *gorm.DB

// InitDB opens (or creates) the SQLite database at data/captaincore.db
// under the user's ~/.captaincore directory and runs AutoMigrate for every model.
func InitDB() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dbPath := filepath.Join(home, ".captaincore", "data", "captaincore.db")

	// Ensure the data directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return err
	}

	// Enable WAL mode for concurrent access (bulk commands run up to 10
	// parallel processes that all read/write this database).
	DB.Exec("PRAGMA journal_mode = WAL")
	DB.Exec("PRAGMA busy_timeout = 30000")
	DB.Exec("PRAGMA synchronous = NORMAL")

	// Limit to one open connection to prevent "database is locked" errors.
	// SQLite only supports one writer at a time; serializing through a single
	// connection lets GORM queue writes instead of racing for the lock.
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)

	// AutoMigrate all models
	err = DB.AutoMigrate(
		&Site{},
		&Environment{},
		&Account{},
		&Domain{},
		&AccountSite{},
		&AccountDomain{},
		&AccountUser{},
		&Capture{},
		&Snapshot{},
		&Key{},
		&Configuration{},
		&Recipe{},
		&Connection{},
		&Provider{},
	)
	if err != nil {
		log.Printf("Warning: AutoMigrate failed: %v", err)
		return err
	}

	return nil
}

// Checkpoint truncates the WAL file to prevent unbounded growth during
// long-running bulk operations. Call after bulk commands complete.
func Checkpoint() {
	if DB != nil {
		DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
}
