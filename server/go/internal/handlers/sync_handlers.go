package handlers

import (
	"net/http"

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
