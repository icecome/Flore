package handlers

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetSyncConfig 读取同步配置（敏感字段脱敏）
func (h *ReaderHandler) GetSyncConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.GetSyncConfig())
}

// UpdateSyncConfig 写入同步配置（敏感字段传掩码值时保留原值）
func (h *ReaderHandler) UpdateSyncConfig(c *gin.Context) {
	var values map[string]string
	if err := c.ShouldBindJSON(&values); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := h.service.UpdateSyncConfig(values); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, h.service.GetSyncConfig())
}

// TestSyncConnection 测试当前同步配置的连通性与认证
func (h *ReaderHandler) TestSyncConnection(c *gin.Context) {
	if h.service.SyncEngine() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	if err := h.service.SyncEngine().TestConnection(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TestFileSyncConnection 测试文件后端（WebDAV 等）连通性与认证
func (h *ReaderHandler) TestFileSyncConnection(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	if err := engine.TestFileConnection(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TriggerSync 手动触发一次全量同步
func (h *ReaderHandler) TriggerSync(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	result, err := engine.TriggerFullSync(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSyncStatus 同步状态（供指示器轮询）
func (h *ReaderHandler) GetSyncStatus(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	c.JSON(http.StatusOK, engine.GetSyncStatus())
}

// AlignSubscriptions 对齐向导：推送本地独有源到远端
func (h *ReaderHandler) AlignSubscriptions(c *gin.Context) {
	var req struct {
		URLs []string `json:"urls"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	result, err := engine.PushLocalOnlySources(c.Request.Context(), req.URLs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ListFileBackups 列出远端 WebDAV 备份目录下的 .zip 备份（供恢复界面选择）。
func (h *ReaderHandler) ListFileBackups(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	backups, err := engine.ListFileBackups(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"backups": backups})
}

// UploadBackupToWebDAV 将本地压缩备份（请求体 ZIP）推送到 WebDAV。
// name 查询参数指定远端文件名；缺失时回退为请求体推导（不强制）。
func (h *ReaderHandler) UploadBackupToWebDAV(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, 256<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read upload failed"})
		return
	}
	name := c.Query("name")
	if name == "" {
		name = fmt.Sprintf("flore-webdav-%s.zip", time.Now().Format("20060102-150405"))
	}
	if err := engine.UploadBackupToWebDAV(c.Request.Context(), name, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name})
}

// CreateBackupToWebDAV 一步完成：创建本地压缩备份并推送到 WebDAV（云端备份按钮）。
func (h *ReaderHandler) CreateBackupToWebDAV(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	name, err := engine.CreateAndUploadBackupToWebDAV(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": safeError(err.Error())})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name})
}

// DownloadBackupFromWebDAV 从 WebDAV 下载指定备份并作为 ZIP 流式返回。
func (h *ReaderHandler) DownloadBackupFromWebDAV(c *gin.Context) {
	engine := h.service.SyncEngine()
	if engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync engine not ready"})
		return
	}
	name := c.Query("name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") || !strings.HasSuffix(name, ".zip") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup name"})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", name))
	if err := engine.DownloadBackupFromWebDAV(c.Request.Context(), name, c.Writer); err != nil {
		// 流式已部分写入时无法改状态码，仅记录；否则返回错误 JSON
		slog.Warn("download webdav backup failed", "name", name, "error", err)
	}
}
