package services

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/glebarez/sqlite"

	"github.com/rss/go-server/internal/database"
)

// escapeSQLitePath 转义路径中的单引号，防止 VACUUM INTO SQL 注入
func escapeSQLitePath(path string) string {
	return strings.ReplaceAll(path, "'", "''")
}

// ExportDatabase 将当前 SQLite 数据库导出为一致快照
func (s *ReaderService) ExportDatabase() (string, error) {
	// 生成临时快照路径
	timestamp := time.Now().Format("20060102-150405")
	snapshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("rss-backup-%s.db", timestamp))

	// 使用 VACUUM INTO 生成一致快照（WAL 模式下也安全），转义路径防注入
	if err := s.execLocked(fmt.Sprintf("VACUUM INTO '%s'", escapeSQLitePath(snapshotPath))); err != nil {
		return "", fmt.Errorf("failed to vacuum into snapshot: %w", err)
	}

	return snapshotPath, nil
}

// validateSQLite 校验文件是否为有效 SQLite 数据库且包含必要的表结构
func validateSQLite(path string) error {
	// 校验 SQLite 魔数，排除非 SQLite 文件（m-04 增强）
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(f, hdr); err != nil {
		f.Close()
		return fmt.Errorf("failed to read file header: %w", err)
	}
	f.Close()
	if !bytes.Equal(hdr, []byte("SQLite format 3\x00")) {
		return fmt.Errorf("invalid sqlite file: bad magic header")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("PRAGMA schema_version").Scan(&version); err != nil {
		return fmt.Errorf("invalid sqlite file: %w", err)
	}
	if version == 0 {
		return fmt.Errorf("database has no schema")
	}

	// 检查是否包含 Item 表，确保是本项目的数据库
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='Item' LIMIT 1").Scan(&name)
	if err != nil {
		return fmt.Errorf("invalid flore database: Item table not found")
	}
	return nil
}

// ImportDatabase 用上传的数据库文件替换当前数据库
func (s *ReaderService) ImportDatabase(reader io.Reader) error {
	tmpPath, err := saveUploadToTempFile(reader)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	return s.restoreFromFile(tmpPath)
}

// RestoreBackup 从指定压缩备份恢复数据库：解压 zip 中的 .db 后复用导入流程（M-R2）
func (s *ReaderService) RestoreBackup(name string) error {
	tmpPath, err := s.extractDBFromBackup(name)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	return s.restoreFromFile(tmpPath)
}

// extractDBFromBackup 从备份 zip 中解压出数据库文件到临时路径
func (s *ReaderService) extractDBFromBackup(name string) (string, error) {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".zip") {
		return "", fmt.Errorf("invalid backup name")
	}
	zipPath := filepath.Join(getBackupDir(database.DBPath()), name)
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open backup: %w", err)
	}
	defer r.Close()

	var dbEntry *zip.File
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".db") {
			dbEntry = f
			break
		}
	}
	if dbEntry == nil {
		return "", fmt.Errorf("no database file found in backup")
	}
	rc, err := dbEntry.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmpFile, err := os.CreateTemp("", "flore-restore-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := io.Copy(tmpFile, rc); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to extract database: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

// saveUploadToTempFile 保存上传文件到临时路径（限制 256MB）
func saveUploadToTempFile(reader io.Reader) (string, error) {
	tmpFile, err := os.CreateTemp("", "rss-restore-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, io.LimitReader(reader, 256<<20)); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write uploaded file: %w", err)
	}
	tmpFile.Close()
	return tmpPath, nil
}

// copyFile 复制文件
// 注意：out.Close() 的错误必须检查，否则 fsync 失败会导致数据未持久化
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return fmt.Errorf("failed to sync output file: %w", err)
	}
	return out.Close()
}

// cleanupOldBackups 保留最近 maxKeep 个 .bak.timestamp 备份文件，删除更早的
func cleanupOldBackups(dbPath string, maxKeep int) {
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)
	prefix := base + ".bak."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type backupEntry struct {
		name     string
		fullPath string
		modTime  time.Time
	}
	var backups []backupEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{
			name:     name,
			fullPath: filepath.Join(dir, name),
			modTime:  info.ModTime(),
		})
	}
	// 按修改时间倒序，保留最新的 maxKeep 个
	for i, j := 0, len(backups)-1; i < j; i, j = i+1, j-1 {
		backups[i], backups[j] = backups[j], backups[i]
	}
	for i := maxKeep; i < len(backups); i++ {
		if err := os.Remove(backups[i].fullPath); err != nil {
			slog.Warn("failed to remove old backup", "path", backups[i].fullPath, "error", err)
		} else {
			slog.Info("removed old database backup", "path", backups[i].fullPath)
		}
	}
}

// BackupEntry 备份文件信息
type BackupEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

// CreateCompressedBackup 创建压缩备份（VACUUM INTO + ZIP）
func (s *ReaderService) CreateCompressedBackup() (string, error) {
	dbPath := database.DBPath()
	backupDir := getBackupDir(dbPath)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// VACUUM INTO 生成一致快照
	timestamp := time.Now().Format("20060102-150405")
	snapshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("flore-backup-%s.db", timestamp))
	if err := s.execLocked(fmt.Sprintf("VACUUM INTO '%s'", escapeSQLitePath(snapshotPath))); err != nil {
		return "", fmt.Errorf("failed to vacuum into snapshot: %w", err)
	}
	defer os.Remove(snapshotPath)

	// 压缩为 ZIP
	zipName := fmt.Sprintf("backup-%s.zip", timestamp)
	zipPath := filepath.Join(backupDir, zipName)
	if err := compressToZip(snapshotPath, zipPath); err != nil {
		return "", fmt.Errorf("failed to compress backup: %w", err)
	}

	// 统一清理：手动与自动备份路径共用同一保留策略，消除手动备份只增不删的问题（M-R1）
	maxKeep := s.GetSettingInt("backupMaxKeep", 10)
	maxDays := s.GetSettingInt("backupMaxDays", 30)
	if deleted, err := s.CleanupBackups(maxKeep, maxDays); err != nil {
		slog.Warn("cleanup after backup failed", "error", err)
	} else if deleted > 0 {
		slog.Info("cleaned expired backups after create", "deleted", deleted)
	}

	return zipName, nil
}

// compressToZip 将单个文件压缩为 ZIP
func compressToZip(srcPath, dstPath string) error {
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	outFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	fw, err := zw.Create(filepath.Base(srcPath))
	if err != nil {
		return err
	}
	if _, err := fw.Write(srcData); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return outFile.Sync()
}

// ListBackups 列出备份目录中的所有 ZIP 文件
func (s *ReaderService) ListBackups() ([]BackupEntry, error) {
	dbPath := database.DBPath()
	backupDir := getBackupDir(dbPath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupEntry{}, nil
		}
		return nil, err
	}

	var backups []BackupEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupEntry{
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	// 按修改时间倒序
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime > backups[j].ModTime
	})

	return backups, nil
}

// DeleteBackup 删除指定备份文件
func (s *ReaderService) DeleteBackup(name string) error {
	dbPath := database.DBPath()
	backupDir := getBackupDir(dbPath)

	// 安全检查：文件名不能包含路径分隔符
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return fmt.Errorf("invalid backup name")
	}
	if !strings.HasSuffix(name, ".zip") {
		return fmt.Errorf("invalid backup file type")
	}

	fullPath := filepath.Join(backupDir, name)
	return os.Remove(fullPath)
}

// CleanupBackups 按保留策略清理过期备份
func (s *ReaderService) CleanupBackups(maxKeep, maxDays int) (int, error) {
	dbPath := database.DBPath()
	backupDir := getBackupDir(dbPath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type zipEntry struct {
		name     string
		fullPath string
		modTime  time.Time
	}
	var zips []zipEntry
	now := time.Now()

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		zips = append(zips, zipEntry{
			name:     e.Name(),
			fullPath: filepath.Join(backupDir, e.Name()),
			modTime:  info.ModTime(),
		})
	}

	// 按修改时间倒序
	sort.Slice(zips, func(i, j int) bool {
		return zips[i].modTime.After(zips[j].modTime)
	})

	deleted := 0
	for i, z := range zips {
		// 超过保留天数 或 超过保留数量
		if (maxDays > 0 && now.Sub(z.modTime).Hours() > float64(maxDays*24)) || (maxKeep > 0 && i >= maxKeep) {
			if err := os.Remove(z.fullPath); err != nil {
				slog.Warn("failed to remove backup", "path", z.fullPath, "error", err)
			} else {
				deleted++
			}
		}
	}

	return deleted, nil
}
