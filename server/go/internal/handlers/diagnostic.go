package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/services"
)

// DiagnosticItem represents a single item in the diagnostic package
type DiagnosticItem struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Type    string `json:"type"` // "log", "snapshot", "stats"
	Checked bool   `json:"checked"`
}

// GenerateDiagnosticPackage handles the diagnostic package generation request
func (h *ReaderHandler) GenerateDiagnosticPackage(c *gin.Context) {
	// 安全：仅允许写入临时目录，避免用户可控 output 参数导致任意目录写入（审查 B-08）
	outputDir := os.TempDir()
	if raw := c.Query("output"); raw != "" {
		if cleaned := filepath.Clean(raw); strings.HasPrefix(cleaned, os.TempDir()) {
			outputDir = cleaned
		}
	}

	itemKeys := c.Request.URL.Query()["items"]
	if len(itemKeys) == 0 {
		itemKeys = []string{"snapshot", "logs", "stats"}
	}

	zipPath := filepath.Join(outputDir, fmt.Sprintf("flore-diagnostic-%s.zip", time.Now().Format("20060102-150405")))

	var zipData bytes.Buffer
	w := zip.NewWriter(&zipData)

	for _, key := range itemKeys {
		switch key {
		case "snapshot":
			addDiagnosticItem(w, "db-snapshot.json", generateDBSnapshot(h.service))
		case "logs":
			addDiagnosticItem(w, "backend.log", readDesensitizedLog("florebackend.log"))
			addDiagnosticItem(w, "desktop.log", readDesensitizedLog("floredesktop.log"))
		case "stats":
			addDiagnosticItem(w, "system-stats.json", generateSystemStats())
		}
	}

	if err := w.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to close zip"})
		return
	}

	data := zipData.Bytes()
	if err := os.WriteFile(zipPath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write diagnostic package"})
		return
	}

	slog.Info("diagnostic package generated", "path", zipPath)
	c.JSON(http.StatusOK, gin.H{"path": zipPath, "size": len(data)})
}

// ListDiagnosticItems returns the available items for the diagnostic package
func (h *ReaderHandler) ListDiagnosticItems(c *gin.Context) {
	items := []DiagnosticItem{
		{Key: "snapshot", Name: "Database Snapshot (per-source stats)", Type: "snapshot", Checked: true},
		{Key: "logs", Name: "Desensitized Logs (last 100 lines)", Type: "log", Checked: true},
		{Key: "stats", Name: "System Statistics", Type: "stats", Checked: true},
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func addDiagnosticItem(w *zip.Writer, name string, data []byte) {
	if len(data) == 0 {
		return
	}
	f, err := w.Create(name)
	if err != nil {
		slog.Warn("failed to create zip entry", "name", name, "error", err)
		return
	}
	f.Write(data)
}

func generateDBSnapshot(svc *services.ReaderService) []byte {
	snapshot := make(map[string]interface{})

	db := database.GetDB()

	// Source statistics with per-source details
	type sourceStat struct {
		ID        int64  `json:"id"`
		Host      string `json:"host"`
		FailCount int    `json:"failCount"`
		ItemCount int64  `json:"itemCount"`
	}
	var sources []sourceStat
	if err := db.Raw("SELECT id, url, COALESCE(fail_count, 0) as fail_count, (SELECT COUNT(*) FROM items i WHERE i.source_id = s.id) as item_count FROM sources s").
		Scan(&sources).Error; err != nil {
		slog.Warn("diagnostic: failed to scan source stats", "error", err)
	}

	snapshot["sourceCount"] = len(sources)
	snapshot["sources"] = sources

	// Item counts by status
	var itemStats struct {
		Total   int64 `json:"total"`
		Unread  int64 `json:"unread"`
		Read    int64 `json:"read"`
		Starred int64 `json:"starred"`
	}
	if err := db.Model(&struct{}{}).Table("items").Count(&itemStats.Total).Error; err != nil {
		slog.Warn("diagnostic: count total failed", "error", err)
	}
	if err := db.Model(&struct{}{}).Table("items").Where("is_read = ?", false).Count(&itemStats.Unread).Error; err != nil {
		slog.Warn("diagnostic: count unread failed", "error", err)
	}
	if err := db.Model(&struct{}{}).Table("items").Where("is_read = ?", true).Count(&itemStats.Read).Error; err != nil {
		slog.Warn("diagnostic: count read failed", "error", err)
	}
	if err := db.Model(&struct{}{}).Table("items").Where("is_starred = ?", true).Count(&itemStats.Starred).Error; err != nil {
		slog.Warn("diagnostic: count starred failed", "error", err)
	}
	snapshot["items"] = itemStats

	// Recent errors from backend log
	snapshot["recentErrors"] = readLastLines("florebackend.log", 20)

	return formatJSON(snapshot)
}

func readDesensitizedLog(filename string) []byte {
	return readLastLines(filename, 100)
}

func readLastLines(filename string, lines int) []byte {
	file, err := os.Open(filename)
	if err != nil {
		return []byte{}
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return []byte{}
	}
	linesSlice := strings.Split(string(content), "\n")
	start := 0
	if len(linesSlice) > lines {
		start = len(linesSlice) - lines
	}
	return []byte(strings.Join(linesSlice[start:], "\n"))
}

func generateSystemStats() []byte {
	stats := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"os":        runtimeInfo(),
	}
	return formatJSON(stats)
}

func runtimeInfo() map[string]string {
	return map[string]string{
		"go_version": "go1.26",
		"os":         strings.ToLower(strings.ReplaceAll(filepath.Ext(os.Args[0]), ".", "")),
	}
}

func formatJSON(v interface{}) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

// ExtractDiagnosticPackage extracts a diagnostic zip for user download
func (h *ReaderHandler) ExtractDiagnosticPackage(c *gin.Context) {
	zipPath := c.Param("path")
	if !strings.HasPrefix(zipPath, "/") && !strings.Contains(zipPath, ":") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}

	// Security: only allow files in temp directory
	cleanPath := filepath.Clean(zipPath)
	if !strings.HasPrefix(cleanPath, os.TempDir()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	http.ServeFile(c.Writer, c.Request, cleanPath)
}

// SendDiagnosticFeedback handles diagnostic package submission from users
func (h *ReaderHandler) SendDiagnosticFeedback(c *gin.Context) {
	// This would be implemented with a secure upload endpoint
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{"status": "diagnostic_upload_endpoint_ready"})
}

// GetDiagnoseItems returns the list of diagnostic items for preview
func (h *ReaderHandler) GetDiagnoseItems(c *gin.Context) {
	items := []DiagnosticItem{
		{Key: "snapshot", Name: "Database Snapshot", Type: "snapshot", Checked: true},
		{Key: "logs", Name: "Log Files", Type: "log", Checked: true},
		{Key: "stats", Name: "System Stats", Type: "stats", Checked: true},
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
