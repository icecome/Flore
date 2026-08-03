package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
	"gorm.io/gorm"
)

type CacheStats struct {
	ReadabilityCount int64 `json:"readabilityCount"`
	ReadabilitySize  int64 `json:"readabilitySize"`
	FTSItemCount     int64 `json:"ftsItemCount"`
	TotalItems       int64 `json:"totalItems"`
	TotalSources     int64 `json:"totalSources"`
}

// GetCacheStats 获取缓存统计信息
func (s *ReaderService) GetCacheStats() (*CacheStats, error) {
	stats := &CacheStats{}
	if err := s.getDb().Model(&models.ReadabilityCache{}).Count(&stats.ReadabilityCount).Error; err != nil {
		return nil, err
	}
	// 估算 ReadabilityCache 大小（Content 字段为主要体积来源）
	var totalContentLen int64
	row := s.getDb().Model(&models.ReadabilityCache{}).
		Select("COALESCE(SUM(LENGTH(content) + LENGTH(title) + LENGTH(byline) + LENGTH(textContent) + LENGTH(excerpt) + LENGTH(siteName) + LENGTH(url)), 0)").
		Row()
	if err := row.Scan(&totalContentLen); err != nil {
		return nil, fmt.Errorf("failed to query readability size: %w", err)
	}
	stats.ReadabilitySize = totalContentLen

	if err := s.getDb().Raw("SELECT count(*) FROM ItemSearch").Scan(&stats.FTSItemCount).Error; err != nil {
		return nil, fmt.Errorf("failed to query fts count: %w", err)
	}
	if err := s.getDb().Model(&models.Item{}).Count(&stats.TotalItems).Error; err != nil {
		return nil, fmt.Errorf("failed to query total items: %w", err)
	}
	if err := s.getDb().Model(&models.Source{}).Count(&stats.TotalSources).Error; err != nil {
		return nil, fmt.Errorf("failed to query total sources: %w", err)
	}
	return stats, nil
}

// ClearReadabilityCache 清空阅读模式缓存
func (s *ReaderService) ClearReadabilityCache() (int64, error) {
	result := s.getDb().Where("1 = 1").Delete(&models.ReadabilityCache{})
	return result.RowsAffected, result.Error
}

// DatabaseInfo 数据库信息
type DatabaseInfo struct {
	Path            string `json:"path"`
	Size            int64  `json:"size"`
	BackupDir       string `json:"backupDir"`
	BackupDirExists bool   `json:"backupDirExists"`
	// JournalMode 透传当前日志模式（rollback/WAL），使“WAL 是否真的启用”可被观测，
	// 避免配置静默失效（审计 M-E1 根因：配置不可见）。
	JournalMode string `json:"journalMode"`
}

// GetDatabaseInfo 获取数据库文件信息
func (s *ReaderService) GetDatabaseInfo() (*DatabaseInfo, error) {
	dbPath := database.DBPath()
	info := &DatabaseInfo{Path: dbPath}

	if fi, err := os.Stat(dbPath); err == nil {
		info.Size = fi.Size()
		// 检查 WAL 文件
		if walFi, err := os.Stat(dbPath + "-wal"); err == nil {
			info.Size += walFi.Size()
		}
		if shmFi, err := os.Stat(dbPath + "-shm"); err == nil {
			info.Size += shmFi.Size()
		}
	}

	// 透传当前日志模式，使 WAL 是否启用可被前端/诊断观测（M-E1 根因：配置不可见）
	var jm string
	if err := s.db.Raw("PRAGMA journal_mode").Scan(&jm).Error; err == nil {
		info.JournalMode = jm
	}

	backupDir := getBackupDir(dbPath)
	info.BackupDir = backupDir
	if _, err := os.Stat(backupDir); err == nil {
		info.BackupDirExists = true
	}

	return info, nil
}

// getBackupDir 返回备份目录路径。
// 便携模式数据库位于 <root>/data/reader.db，备份目录提升到 <root>/backups，
// 与 data/ 同级，使 data/ 只承载数据与日志；安装模式数据库直接在用户数据目录，
// 备份目录保持为数据库同级的 backups/。
func getBackupDir(dbPath string) string {
	dir := filepath.Dir(dbPath)
	if filepath.Base(dir) == "data" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, "backups")
}

// BackupDirPath 返回备份目录绝对路径（供 handler 安全构造下载路径）
func (s *ReaderService) BackupDirPath() string {
	return getBackupDir(database.DBPath())
}

// Vacuum 压缩数据库（VACUUM）
func (s *ReaderService) Vacuum() error {
	return s.execLocked("VACUUM")
}

// GetAllSettings 获取所有设置项，返回 key->value 映射
func (s *ReaderService) GetAllSettings() (map[string]string, error) {
	var settings []models.Setting
	if err := s.getDb().Find(&settings).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// GetSetting 获取单个设置项，不存在返回空字符串
func (s *ReaderService) GetSetting(key string) string {
	var setting models.Setting
	if err := s.getDb().Where("`key` = ?", key).First(&setting).Error; err != nil {
		return ""
	}
	return setting.Value
}

// GetSettingInt 获取整数型设置项，不存在或解析失败返回默认值
func (s *ReaderService) GetSettingInt(key string, defaultVal int) int {
	v := s.GetSetting(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// GetSettingBool 获取布尔型设置项，不存在或解析失败返回默认值
func (s *ReaderService) GetSettingBool(key string, defaultVal bool) bool {
	v := s.GetSetting(key)
	if v == "" {
		return defaultVal
	}
	return v == "true"
}

// UpdateSettings 批量更新设置项（upsert 语义）
func (s *ReaderService) UpdateSettings(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	err := s.getDb().Transaction(func(tx *gorm.DB) error {
		for k, v := range values {
			setting := models.Setting{Key: k, Value: v}
			if err := tx.Save(&setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 代理设置变更时，递增序列号使缓存失效
	if _, ok := values["proxyEnabled"]; ok {
		atomic.AddInt64(&s.proxySettingsSeq, 1)
	} else if _, ok := values["proxyUrl"]; ok {
		atomic.AddInt64(&s.proxySettingsSeq, 1)
	}
	return nil
}

// CleanupArticles 按留存策略清理已读文章
// retentionDays: 保留最近 N 天的文章（0=不限制）
// retentionMax: 保留最新 N 篇文章（0=不限制）
// excludeStarred: 收藏文章不清理
// excludeReadLater: 稍后阅读文章不清理
func (s *ReaderService) CleanupArticles(retentionDays, retentionMax int, excludeStarred, excludeReadLater bool) (int64, error) {
	if retentionDays <= 0 && retentionMax <= 0 {
		return 0, nil
	}
	tx := s.getDb().Begin()
	defer s.recoverAndRollback(tx)

	query := s.buildCleanupQuery(tx, excludeStarred, excludeReadLater, retentionDays)
	if err := s.applyCleanupRetentionMax(tx, &query, retentionMax, excludeStarred, excludeReadLater); err != nil {
		tx.Rollback()
		return 0, err
	}

	result := query.Delete(&models.Item{})
	if result.Error != nil {
		tx.Rollback()
		return 0, result.Error
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}

func (s *ReaderService) recoverAndRollback(tx *gorm.DB) {
	if r := recover(); r != nil {
		tx.Rollback()
	}
}

func (s *ReaderService) buildCleanupQuery(tx *gorm.DB, excludeStarred, excludeReadLater bool, retentionDays int) *gorm.DB {
	query := tx.Model(&models.Item{}).Where("isRead = ?", true)
	if excludeStarred {
		query = query.Where("isStarred = ?", false)
	}
	if excludeReadLater {
		query = query.Where("isReadLater = ?", false)
	}
	if retentionDays > 0 {
		query = query.Where("createdAt < ?", time.Now().AddDate(0, 0, -retentionDays))
	}
	return query
}

func (s *ReaderService) applyCleanupRetentionMax(tx *gorm.DB, query **gorm.DB, retentionMax int, excludeStarred, excludeReadLater bool) error {
	if retentionMax <= 0 {
		return nil
	}
	keepIDs, err := s.findCleanupKeepItems(tx, retentionMax, excludeStarred, excludeReadLater)
	if err != nil {
		return err
	}
	if len(keepIDs) > 0 {
		*query = (*query).Where("id NOT IN ?", keepIDs)
	}
	return nil
}

func (s *ReaderService) findCleanupKeepItems(tx *gorm.DB, retentionMax int, excludeStarred, excludeReadLater bool) ([]int, error) {
	var keepIDs []int
	subQuery := tx.Model(&models.Item{}).Where("isRead = ?", true)
	if excludeStarred {
		subQuery = subQuery.Where("isStarred = ?", false)
	}
	if excludeReadLater {
		subQuery = subQuery.Where("isReadLater = ?", false)
	}
	err := subQuery.Order("createdAt DESC").Limit(retentionMax).Pluck("id", &keepIDs).Error
	return keepIDs, err
}

// indexWorker 后台 goroutine：从 indexChan 批量消费 itemID，合并写 FTS5 索引和过滤规则。
// 将原本 N 次独立事务合并为 1 次批量事务，大幅降低 SQLite 磁盘 I/O。
