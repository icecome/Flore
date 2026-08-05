package services

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

// restoreFromFile 用临时数据库文件替换当前数据库（导入 / 恢复备份共用核心流程）
func (s *ReaderService) restoreFromFile(tmpPath string) error {
	s.dbMu.Lock()
	defer s.dbMu.Unlock()

	currentPath := database.DBPath()
	slog.Info("restore: starting", "currentPath", currentPath, "tmpPath", tmpPath)

	// WAL 模式下先 checkpoint，将 -wal 中已提交的数据合并回主库，
	// 否则备份快照会丢失未合并的数据（启用 WAL 后的必要安全步骤）
	if err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		slog.Warn("wal checkpoint before backup failed", "error", err)
	}

	if err := validateSQLite(tmpPath); err != nil {
		slog.Warn("restore: validate failed", "error", err)
		return err
	}
	slog.Info("restore: validation passed")

	// 在关闭数据库之前读取设置（之后数据库不可用）
	// 注意：不能调用 s.GetSettingInt()，因为它内部调用 s.getDb() 尝试获取读锁，
	// 但 restoreFromFile 已持有 s.dbMu 写锁，Go 的 sync.RWMutex 不可重入，会导致死锁。
	maxKeep := readMaxKeepFromDB(s.db)

	// 使用 VACUUM INTO 在数据库仍打开时创建备份，避免 Windows 文件锁问题。
	// SQLite 的 VACUUM INTO 在内部处理文件读写，不会产生字节范围锁冲突。
	timestamp := time.Now().Format("20060102-150405")
	bakPath := fmt.Sprintf("%s.bak.%s", currentPath, timestamp)
	slog.Info("restore: creating backup via VACUUM INTO", "bakPath", bakPath)
	// 路径必须转义（escapeSQLitePath），bakPath 来自数据库路径，Windows 路径可含单引号
	if err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapeSQLitePath(bakPath))).Error; err != nil {
		slog.Warn("restore: VACUUM INTO failed, falling back to close-and-copy", "error", err)
		if err := s.restoreViaCloseAndCopy(tmpPath, currentPath, bakPath); err != nil {
			return err
		}
	} else {
		slog.Info("restore: VACUUM INTO succeeded, closing database for replacement")
	}
	slog.Info("restore: backup created", "bakPath", bakPath)

	// 清理旧备份（仅文件操作，不涉及数据库查询）
	cleanupOldBackups(currentPath, maxKeep)

	// 关闭数据库连接，释放文件锁，准备替换文件
	if err := closeAndReleaseLocks(currentPath); err != nil {
		return err
	}
	slog.Info("restore: database closed, file lock released")

	// 替换数据库文件
	if err := copyFile(tmpPath, currentPath); err != nil {
		slog.Error("restore: copy failed, attempting rollback", "error", err)
		if rbErr := copyFile(bakPath, currentPath); rbErr != nil {
			slog.Error("restore: rollback copy also failed", "error", rbErr)
		}
		return fmt.Errorf("failed to replace database file: %w", err)
	}
	slog.Info("restore: file replaced")

	// 重新初始化数据库连接
	if err := database.Init(); err != nil {
		s.db = database.GetDB()
		slog.Warn("restore: reinit failed", "error", err)
		return fmt.Errorf("failed to reinitialize database: %w", err)
	}
	slog.Info("restore: database reinitialized")

	if err := database.AutoMigrate(); err != nil {
		s.db = database.GetDB()
		slog.Warn("restore: automigrate failed", "error", err)
		return fmt.Errorf("failed to automigrate after restore: %w", err)
	}
	slog.Info("restore: automigrate done")

	if err := database.RunMigrations(); err != nil {
		s.db = database.GetDB()
		slog.Warn("restore: run migrations failed", "error", err)
		return fmt.Errorf("failed to run migrations after restore: %w", err)
	}
	slog.Info("restore: migrations done")

	// 先重新指向重新打开的连接，再做压缩，避免使用已被关闭的旧连接（M-P1）
	s.db = database.GetDB()
	slog.Info("restore: starting vacuum")
	if err := s.db.Exec("VACUUM").Error; err != nil {
		slog.Warn("vacuum after restore failed", "error", err)
	}
	slog.Info("restore: vacuum done")

	// 导入/恢复后使未读计数缓存失效，下次 GetSources 重算为导入后的真实值（P2-1/B）
	s.invalidateUnreadCount()
	slog.Info("restore: completed")
	return nil
}

// readMaxKeepFromDB 直接从 s.db 读取 backupMaxKeep 设置，避免通过 getDb() 获取读锁导致死锁
func readMaxKeepFromDB(db *gorm.DB) int {
	maxKeep := 10
	var rawVal string
	if err := db.Model(&models.Setting{}).Where("`key` = ?", "backupMaxKeep").Select("value").Scan(&rawVal).Error; err == nil && rawVal != "" {
		if n, convErr := strconv.Atoi(rawVal); convErr == nil {
			maxKeep = n
		}
	}
	return maxKeep
}

// restoreViaCloseAndCopy 在 VACUUM INTO 失败时的降级路径：先关闭数据库，再拷贝文件
func (s *ReaderService) restoreViaCloseAndCopy(tmpPath, currentPath, bakPath string) error {
	if err := closeAndReleaseLocks(currentPath); err != nil {
		return err
	}
	slog.Info("restore: fallback: database closed, copying backup")
	if fbErr := copyFile(currentPath, bakPath); fbErr != nil {
		return fmt.Errorf("failed to create backup via fallback: %w", fbErr)
	}
	if err := copyFile(tmpPath, currentPath); err != nil {
		slog.Error("restore: fallback copy failed, attempting rollback", "error", err)
		if rbErr := copyFile(bakPath, currentPath); rbErr != nil {
			slog.Error("restore: rollback copy also failed", "error", rbErr)
		}
		return fmt.Errorf("failed to replace database file: %w", err)
	}
	return nil
}

// closeAndReleaseLocks 关闭数据库连接并删除 WAL/shm 文件，释放文件锁
func closeAndReleaseLocks(currentPath string) error {
	sqlDB, err := database.GetDB().DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql db: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("failed to close current database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if rmErr := os.Remove(currentPath + suffix); rmErr != nil {
			slog.Warn("restore: failed to remove WAL/shm file", "suffix", suffix, "error", rmErr)
		}
	}
	return nil
}
