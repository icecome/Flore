package database

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rss/go-server/internal/models"
)

var (
	DB      *gorm.DB
	dbMutex sync.RWMutex
)

// GetDB 返回当前数据库实例（并发安全）
func GetDB() *gorm.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return DB
}

// DBPath 返回当前使用的数据库文件绝对路径。
// 与 Init 中的路径解析逻辑保持一致，供备份/恢复等功能使用。
func DBPath() string {
	return resolvedDBPath
}

var resolvedDBPath string

// defaultReaderDBPath 返回 Reader 默认数据库路径。
// Windows: %LOCALAPPDATA%\Flore\reader.db
// 其他平台: ~/.flore/reader.db
func defaultReaderDBPath() (string, error) {
	var baseDir string
	if runtime.GOOS == "windows" {
		baseDir = os.Getenv("LOCALAPPDATA")
		if baseDir == "" {
			baseDir = os.Getenv("APPDATA")
		}
		if baseDir == "" {
			baseDir = os.TempDir()
		}
		baseDir = filepath.Join(baseDir, "Flore")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".flore")
	}
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create database directory %s: %w", baseDir, err)
	}
	return filepath.Join(baseDir, "reader.db"), nil
}

// Init 初始化数据库连接
// 注意：.env 由 cmd/main.go 在启动时统一加载，此处不再重复加载
func Init() error {
	dbPath, err := resolveDatabasePath()
	if err != nil {
		return fmt.Errorf("failed to resolve database path: %w", err)
	}
	resolvedDBPath = dbPath

	// 确保数据库文件所在目录存在，避免 SQLite 因目录不存在而无法打开文件
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("failed to create database directory %s: %w", filepath.Dir(dbPath), err)
	}

	// 启用 WAL（预写日志）：提升读写并发与崩溃安全性，并让 -wal/-shm 成为常态文件。
	// 同时设置 auto_vacuum=INCREMENTAL：删除产生的空闲页可被后续写入复用，避免实时库无限膨胀。
	// 注意：pragma 参数名必须正确。`wal_mode` 是错误写法（应为 journal_mode），SQLite 会静默忽略而不报错，
	// 这正是此前 WAL 从未真正启用的根因（见审计 M-E1）。
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=busy_timeout(5000)", dbPath)

	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(dsn), config)
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	if err := configureConnectionPool(db); err != nil {
		return err
	}

	dbMutex.Lock()
	DB = db
	dbMutex.Unlock()
	return nil
}

// resolveDatabasePath 按优先级解析数据库路径：环境变量 > 开发模式 > 便携模式 > 默认路径
func resolveDatabasePath() (string, error) {
	dbPath := os.Getenv("DATABASE_URL")
	if dbPath != "" {
		return resolveAbsolutePath(dbPath), nil
	}

	dbPath = tryDevLocalPath()
	if dbPath != "" {
		return dbPath, nil
	}

	dbPath = tryPortablePath()
	if dbPath != "" {
		return dbPath, nil
	}

	return defaultReaderDBPath()
}

// tryDevLocalPath 检测开发模式下的项目内数据目录
func tryDevLocalPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	localPath := filepath.Join(cwd, "data", "reader.db")
	if _, statErr := os.Stat(localPath); statErr == nil {
		return localPath
	}
	return ""
}

// tryPortablePath 检测便携模式：可执行文件同级目录存在 data/ 目录
func tryPortablePath() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	execDir := filepath.Dir(execPath)
	dataDir := filepath.Join(execDir, "data")
	if info, statErr := os.Stat(dataDir); statErr == nil && info.IsDir() {
		return filepath.Join(dataDir, "reader.db")
	}
	return ""
}

// resolveAbsolutePath 如果是相对路径，转换为绝对路径
func resolveAbsolutePath(dbPath string) string {
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	execPath, err := os.Executable()
	if err != nil {
		return dbPath
	}
	return filepath.Join(filepath.Dir(execPath), dbPath)
}

// configureConnectionPool 设置连接池
// MaxOpenConns 需与抓取并发度匹配，避免连接饥饿
func configureConnectionPool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	return nil
}

// AutoMigrate 自动迁移数据库结构
func AutoMigrate() error {
	if err := DB.AutoMigrate(&models.Folder{}, &models.Source{}, &models.Item{}, &models.ReadabilityCache{}, &models.SourceHealth{}, &models.FilterRule{}, &models.Setting{}); err != nil {
		return err
	}
	// 删除旧的 link 全局 unique 索引（已改为 link+sourceId 复合唯一索引）
	DB.Exec("DROP INDEX IF EXISTS idx_items_link")
	DB.Exec("DROP INDEX IF EXISTS uni_items_link")
	// 创建 FTS5 全文搜索虚拟表（Prisma/GORM 均不支持，需原生 SQL）
	if err := DB.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS ItemSearch USING fts5(
			title,
			content,
			itemId UNINDEXED,
			tokenize = 'porter unicode61'
		)
	`).Error; err != nil {
		return fmt.Errorf("failed to create fts5 table: %w", err)
	}
	// 首次启用或表为空时，一次性导入历史数据（包裹事务避免长时间持锁）
	var ftsCount int64
	if err := DB.Raw("SELECT count(*) FROM ItemSearch").Scan(&ftsCount).Error; err != nil {
		return fmt.Errorf("failed to count fts5 table: %w", err)
	}
	if ftsCount == 0 {
		if err := seedFTS5Batched(DB); err != nil {
			return fmt.Errorf("failed to seed fts5 table: %w", err)
		}
	}
	return nil
}

// seedFTS5Batched 分批导入历史数据到 FTS5 表，避免大事务长时间持锁
func seedFTS5Batched(db *gorm.DB) error {
	const batchSize = 1000
	offset := 0
	for {
		var affected int64
		err := db.Transaction(func(tx *gorm.DB) error {
			result := tx.Exec(`
				INSERT INTO ItemSearch(title, content, itemId)
				SELECT title, COALESCE(desc, ''), id FROM Item
				LIMIT ? OFFSET ?
			`, batchSize, offset)
			affected = result.RowsAffected
			return result.Error
		})
		if err != nil {
			return err
		}
		if affected < batchSize {
			break
		}
		offset += batchSize
	}
	return nil
}
