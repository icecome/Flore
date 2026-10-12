package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rss/go-server/internal/models"
	"github.com/rss/go-server/pkg/paths"
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

// defaultReaderDBPath 返回 Reader 默认数据库路径（用户数据目录下的 reader.db）。
// 用户数据目录策略统一由 paths.UserDataDir() 提供，避免与桌面壳双份维护。
func defaultReaderDBPath() (string, error) {
	baseDir, err := paths.UserDataDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user data dir: %w", err)
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

	// 启动完整性快检（P0-006）：quick_check 跳过索引-表交叉校验，开销毫秒级，
	// 是启动阶段的标准选择。失败不直接退出，交由调用方决定进入恢复模式。
	if err := QuickCheck(); err != nil {
		dbMutex.Lock()
		DB = db
		dbMutex.Unlock()
		return err
	}

	dbMutex.Lock()
	DB = db
	dbMutex.Unlock()
	return nil
}

// QuickCheck 运行 PRAGMA quick_check 检测数据库页级损坏。
// 返回 ok 时 database.go 的 QuickCheckResult() 可读到具体结果串。
// 与 integrity_check 的区别：quick_check 不做索引-表交叉校验，启动阶段足够且极快。
func QuickCheck() error {
	ok, detail, err := QuickCheckDetailed()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("database integrity check failed: %s", detail)
	}
	return nil
}

// QuickCheckDetailed 返回 (是否通过, quick_check 结果串, 错误)。
// 结果串为 "ok" 表示通过；否则为损坏详情（供恢复界面/日志展示）。
func QuickCheckDetailed() (bool, string, error) {
	db := GetDB()
	if db == nil {
		return false, "", fmt.Errorf("database not initialized")
	}
	var results []string
	if err := db.Raw("PRAGMA quick_check").Scan(&results).Error; err != nil {
		return false, "", fmt.Errorf("quick_check query failed: %w", err)
	}
	if len(results) == 0 {
		return false, "", fmt.Errorf("quick_check returned no rows")
	}
	// quick_check 正常返回单行 "ok"；损坏时每行一条错误描述
	if len(results) == 1 && results[0] == "ok" {
		return true, "ok", nil
	}
	return false, strings.Join(results, "; "), nil
}
func SetDBPath(path string) {
	resolvedDBPath = path
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
	// 索引可经 IF EXISTS 容忍不存在，但 DROP 失败需上报，避免遗留错误表结构影响迁移正确性。
	if err := DB.Exec("DROP INDEX IF EXISTS idx_items_link").Error; err != nil {
		return fmt.Errorf("failed to drop legacy index idx_items_link: %w", err)
	}
	if err := DB.Exec("DROP INDEX IF EXISTS uni_items_link").Error; err != nil {
		return fmt.Errorf("failed to drop legacy index uni_items_link: %w", err)
	}
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
