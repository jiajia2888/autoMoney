package dao

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
)

func Open(dsn string) (*gorm.DB, error) { return OpenLocal(dsn, "../.local/monitor.db") }
func OpenLocal(mysqlDSN, path string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if mysqlDSN != "" {
		dialector = mysql.Open(mysqlDSN)
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		dialector = sqlite.Open(path + "?_busy_timeout=5000&_journal_mode=WAL")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// Migrate is only called by the explicit -migrate CLI command.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&BenchmarkAccount{}, &Video{}, &Notification{}, &CrawlRun{}, &VideoTranscript{})
}
func CheckSchema(db *gorm.DB) error {
	for _, model := range []any{&BenchmarkAccount{}, &Video{}, &Notification{}, &CrawlRun{}, &VideoTranscript{}} {
		if !db.Migrator().HasTable(model) {
			return fmt.Errorf("数据库尚未初始化，请先运行 make migrate")
		}
	}
	if !db.Migrator().HasColumn(&BenchmarkAccount{}, "history_complete") || !db.Migrator().HasColumn(&BenchmarkAccount{}, "history_cursor") || !db.Migrator().HasColumn(&BenchmarkAccount{}, "history_end") || !db.Migrator().HasColumn(&Video{}, "hearts") || !db.Migrator().HasColumn(&Video{}, "nonce") || !db.Migrator().HasColumn(&Video{}, "first_over_10k_at") || !db.Migrator().HasColumn(&Video{}, "already_over_10k") {
		return fmt.Errorf("数据库版本需要更新，请先运行 make migrate")
	}
	return nil
}
