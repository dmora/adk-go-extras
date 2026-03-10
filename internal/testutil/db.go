package testutil

import (
	"path/filepath"
	"testing"

	sqlitedriver "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewTestDB opens a temporary SQLite database configured for concurrent tests.
func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return OpenSQLiteDB(t, filepath.Join(t.TempDir(), "test.db"))
}

// OpenSQLiteDB opens a SQLite database at the provided path using a quiet
// logger and test-friendly pragmas.
func OpenSQLiteDB(t *testing.T, path string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlitedriver.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatalf("set wal mode: %v", err)
	}
	if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		t.Fatalf("set busy_timeout: %v", err)
	}

	return db
}
