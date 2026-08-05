package handlers

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rss/go-server/internal/services"
)

func (h *ReaderHandler) ExportDatabase(c *gin.Context) {
	snapshotPath, err := h.service.ExportDatabase()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	defer os.Remove(snapshotPath)

	filename := fmt.Sprintf("rss-backup-%s.db", time.Now().Format("2006-01-02-150405"))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.File(snapshotPath)
}

// ImportDatabase 导入 SQLite 数据库文件并替换当前数据库
func (h *ReaderHandler) ImportDatabase(c *gin.Context) {
	// 限制上传文件大小为 256MB
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<20)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file or file too large"})
		return
	}
	defer file.Close()

	if err := h.service.ImportDatabase(file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateSource 创建订阅源
func (h *ReaderHandler) GetDatabaseInfo(c *gin.Context) {
	info, err := h.service.GetDatabaseInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, info)
}

// GetCacheStats 获取缓存统计
func (h *ReaderHandler) GetCacheStats(c *gin.Context) {
	stats, err := h.service.GetCacheStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// ClearReadabilityCache 清空阅读模式缓存
func (h *ReaderHandler) ClearReadabilityCache(c *gin.Context) {
	count, err := h.service.ClearReadabilityCache()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": count})
}

// VacuumDatabase 压缩数据库（VACUUM）
func (h *ReaderHandler) VacuumDatabase(c *gin.Context) {
	if err := h.service.Vacuum(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListBackups 列出备份文件
func (h *ReaderHandler) ListBackups(c *gin.Context) {
	backups, err := h.service.ListBackups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	// 确保即使为 nil 也返回空数组
	if backups == nil {
		backups = []services.BackupEntry{}
	}
	c.JSON(http.StatusOK, backups)
}

// CreateBackup 创建压缩备份
func (h *ReaderHandler) CreateBackup(c *gin.Context) {
	name, err := h.service.CreateCompressedBackup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name})
}

// DeleteBackup 删除指定备份
func (h *ReaderHandler) DeleteBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.DeleteBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CleanupBackups 按策略清理过期备份
func (h *ReaderHandler) CleanupBackups(c *gin.Context) {
	var req struct {
		MaxKeep int `json:"maxKeep"`
		MaxDays int `json:"maxDays"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	if req.MaxKeep <= 0 {
		req.MaxKeep = 10
	}
	if req.MaxDays <= 0 {
		req.MaxDays = 30
	}
	deleted, err := h.service.CleanupBackups(req.MaxKeep, req.MaxDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// RestoreBackup 从指定压缩备份恢复数据库（M-R2：备份不再是只写，可一键回放）
func (h *ReaderHandler) RestoreBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DownloadBackup 下载指定备份 Zip 文件
func (h *ReaderHandler) DownloadBackup(c *gin.Context) {
	name := c.Param("name")
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	fullPath := filepath.Join(h.service.BackupDirPath(), name)
	if _, err := os.Stat(fullPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", name))
	c.File(fullPath)
}

// GetBackupContents 获取备份文件内容清单
func (h *ReaderHandler) GetBackupContents(c *gin.Context) {
	name := c.Param("name")
	contents, err := h.service.GetBackupContents(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, contents)
}

// ImportBackupAndValidate 导入外部备份文件并返回内容清单（供前端选择恢复粒度）
// 接收 multipart/form-data，包含一个 "backup" 文件字段
func (h *ReaderHandler) ImportBackupAndValidate(c *gin.Context) {
	// 从 multipart form 获取上传的 backup.zip 文件
	file, err := c.FormFile("backup")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to get backup file"})
		return
	}

	// 保存到临时文件
	tmpPath, err := h.saveTempUpload(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save backup file"})
		return
	}
	defer os.Remove(tmpPath)

	// 读取备份内容清单
	contents, err := h.service.GetBackupContentsFromPath(tmpPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup file: " + err.Error()})
		return
	}

	// 返回清单信息
	c.JSON(http.StatusOK, gin.H{
		"name":    file.Filename,
		"hasDb":   contents.HasDB,
		"hasCfg":  contents.HasCfg,
		"hasOpml": contents.HasOpm, // 修复：正确使用 HasOpm 字段名
	})
}

// saveTempUpload 保存上传文件到临时路径（限制 256MB）
func (h *ReaderHandler) saveTempUpload(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "flore-restore-import-*.zip")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.Remove(tmpFile.Name())
		}
	}()

	_, err = io.Copy(tmpFile, io.LimitReader(src, 256<<20))
	if err != nil {
		tmpFile.Close()
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

// RestoreConfigFromBackup 从备份中仅恢复配置项
func (h *ReaderHandler) RestoreConfigFromBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreConfigFromBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RestoreOPMLFromBackup 从备份中仅恢复订阅源
func (h *ReaderHandler) RestoreOPMLFromBackup(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.RestoreOPMLFromBackup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetSettings 获取所有设置项
func (h *ReaderHandler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// GetSetting 获取单个设置项
func (h *ReaderHandler) GetSetting(c *gin.Context) {
	key := c.Param("key")
	value := h.service.GetSetting(key)
	c.JSON(http.StatusOK, gin.H{"key": key, "value": value})
}

// UpdateSettings 批量更新设置项
// 接收 JSON 对象：{ "key1": "value1", "key2": "value2", ... }
// 值统一以字符串存储，前端负责类型转换
func (h *ReaderHandler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	// 限制 key 数量和 value 长度，防止滥用存储或 DoS
	const maxSettingsKeys = 100
	const maxValueLen = 10 * 1024 // 10KB
	if len(req) > maxSettingsKeys {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("too many keys, max %d", maxSettingsKeys)})
		return
	}
	for k, v := range req {
		if len(k) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key too long"})
			return
		}
		if len(v) > maxValueLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("value too long for key %s, max %d bytes", k, maxValueLen)})
			return
		}
	}
	if err := h.service.UpdateSettings(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CleanupArticles 按留存策略清理已读文章
func (h *ReaderHandler) CleanupArticles(c *gin.Context) {
	var req struct {
		RetentionDays    int  `json:"retentionDays"`
		RetentionMax     int  `json:"retentionMax"`
		ExcludeStarred   bool `json:"excludeStarred"`
		ExcludeReadLater bool `json:"excludeReadLater"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	deleted, err := h.service.CleanupArticles(req.RetentionDays, req.RetentionMax, req.ExcludeStarred, req.ExcludeReadLater)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// appVersion 由构建时 -ldflags 注入：
// -X github.com/rss/go-server/internal/handlers.appVersion=x.y.z
// 必须保持字符串字面量初始化，若改为函数调用初始化，包 init 会覆盖链接器写入的值。
var appVersion = "dev"

// resolveAppVersion 运行时确定版本号：ldflags 注入值 > 环境变量 FLORE_VERSION > "dev"
func resolveAppVersion() string {
	if appVersion != "" && appVersion != "dev" {
		return appVersion
	}
	if v := os.Getenv("FLORE_VERSION"); v != "" {
		return v
	}
	return "dev"
}

func (h *ReaderHandler) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": resolveAppVersion(),
	})
}
