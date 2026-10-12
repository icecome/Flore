package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
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
	// 安全：仅允许写入临时目录，避免用户可控 output 参数导致任意目录写入（审查 B-08）。
	// 前缀判断必须带路径分隔符（tempDir + separator），否则 C:\...\Temp2\xxx 会因共享
	// "Temp" 前缀绕过校验——与下方 ExtractDiagnosticPackage 的校验口径一致（m-08）。
	outputDir := os.TempDir()
	tempDir := filepath.Clean(os.TempDir())
	if raw := c.Query("output"); raw != "" {
		cleaned := filepath.Clean(raw)
		if cleaned == tempDir || strings.HasPrefix(cleaned, tempDir+string(os.PathSeparator)) {
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

func addDiagnosticItem(w *zip.Writer, name string, data []byte) {
	if len(data) == 0 {
		return
	}
	f, err := w.Create(name)
	if err != nil {
		slog.Warn("failed to create zip entry", "name", name, "error", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		slog.Warn("failed to write zip entry", "name", name, "error", err)
	}
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
	// 表/列名与 models.go 保持一致（Source/Item、sourceId/fetchFailCount），
	// 旧写法引用了不存在的 sources/items/source_id/fail_count 导致快照恒为空（M-R2）。
	if err := db.Raw("SELECT id, url, COALESCE(fetchFailCount, 0) as fail_count, (SELECT COUNT(*) FROM Item i WHERE i.sourceId = s.id) as item_count FROM Source s").
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
	if err := db.Model(&models.Item{}).Count(&itemStats.Total).Error; err != nil {
		slog.Warn("diagnostic: count total failed", "error", err)
	}
	if err := db.Model(&models.Item{}).Where("isRead = ?", false).Count(&itemStats.Unread).Error; err != nil {
		slog.Warn("diagnostic: count unread failed", "error", err)
	}
	if err := db.Model(&models.Item{}).Where("isRead = ?", true).Count(&itemStats.Read).Error; err != nil {
		slog.Warn("diagnostic: count read failed", "error", err)
	}
	if err := db.Model(&models.Item{}).Where("isStarred = ?", true).Count(&itemStats.Starred).Error; err != nil {
		slog.Warn("diagnostic: count starred failed", "error", err)
	}
	snapshot["items"] = itemStats

	// recentErrors 使用与 GenerateDiagnosticPackage 相同的日志解析，避免依赖 CWD（m-04）
	snapshot["recentErrors"] = readLastLines("florebackend.log", 20)

	return formatJSON(snapshot)
}

// resolveLogPath 返回日志文件的优先候选绝对路径。
// backend 日志：优先 FLORE_LOG_FILE（桌面壳显式注入），否则退回 DATABASE_URL 目录；
// desktop 日志：位于 DATABASE_URL 同级目录。统一切片供 readLastLines 依序尝试，
// 修复原相对路径打开依赖进程 CWD 导致诊断日志恒为空的问题（m-04）。
func resolveLogPath(filename string) []string {
	if abs, err := filepath.Abs(filename); err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			return []string{abs}
		}
	}
	// 用 DB 目录推导：backend 日志可用显式 FLORE_LOG_FILE 覆盖
	baseDir := "."
	if dbPath := os.Getenv("DATABASE_URL"); dbPath != "" {
		baseDir = filepath.Dir(dbPath)
	}
	if filename == "florebackend.log" {
		if explicit := os.Getenv("FLORE_LOG_FILE"); explicit != "" {
			if _, statErr := os.Stat(explicit); statErr == nil {
				return []string{explicit}
			}
			return []string{explicit, filepath.Join(baseDir, filename)}
		}
		return []string{filepath.Join(baseDir, filename)}
	}
	return []string{filepath.Join(baseDir, filename)}
}

func readDesensitizedLog(filename string) []byte {
	return redactSensitive(readLastLines(filename, 100))
}

// redactSensitive 对日志文本做敏感信息脱敏（P3-006）。
// 诊断包会被用户分享给支持渠道/公开渠道，必须在打包前移除：
//   - Bearer Token（Authorization: Bearer xxx）
//   - key=value 形式的 token/password/secret
//   - FLORE_API_TOKEN 等环境变量值
//
// 保留行首时间戳与消息文本，仅替换敏感片段，确保日志仍可诊断。
func redactSensitive(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	out := string(data)
	// Authorization: Bearer <token>
	out = authBearerRe.ReplaceAllString(out, "Authorization: Bearer ****")
	// token=xxx / password=xxx / secret=xxx（含引号包裹与大小写变体）
	out = secretKVRe.ReplaceAllString(out, "${1}=****")
	// FLORE_API_TOKEN=<hex>
	out = apiTokenKVRe.ReplaceAllString(out, "FLORE_API_TOKEN=****")
	return []byte(out)
}

var (
	// Authorization: Bearer <token>（token 取至行尾或空白）
	authBearerRe = regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+\S+`)
	// token|password|passwd|secret = <值>（值可为引号包裹）
	secretKVRe = regexp.MustCompile(`(?i)\b(token|password|passwd|secret)\b(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)
	// FLORE_API_TOKEN=<hex>
	apiTokenKVRe = regexp.MustCompile(`(?i)FLORE_API_TOKEN=\S+`)
)

// readLastLines 从文件尾部反向读取最近 N 行，避免全量读入（日志上限 10MB）。
// 依 resolveLogPath 候选依序尝试，首个存在且可读的文件生效（m-06）。
func readLastLines(filename string, lines int) []byte {
	var content []byte
	for _, path := range resolveLogPath(filename) {
		data, err := readTailLines(path, lines)
		if err == nil {
			content = data
			break
		}
	}
	if len(content) == 0 {
		return []byte{}
	}
	return content
}

// readTailLines 从文件末尾反向扫描，读取最近 lines 行。
func readTailLines(path string, lines int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return []byte{}, nil
	}

	// 从尾部向后读块（块大小上限 64KB），统计换行数直到收集到足够行或到达文件头。
	// 单行超长时限制读取量，避免超大单行撑爆内存。
	const chunkSize = 64 << 10
	const maxScan = 8 << 20 // 最多向后读 8MB，超出视为单行超长
	readSize := size
	if readSize > maxScan {
		readSize = maxScan
	}

	start := size - readSize
	// 跳到块起始对齐边界
	if start > 0 && start < size {
		start -= 1
	}

	offset := start
	buf := make([]byte, 0, chunkSize)
	for offset < size {
		end := offset + chunkSize
		if end > size {
			end = size
		}
		chunk := make([]byte, end-offset)
		if _, err := file.ReadAt(chunk, offset); err != nil {
			return nil, err
		}
		buf = append(buf, chunk...)
		offset = end
	}

	// buf 为读尾部的内容，逆序扫描换行
	str := string(buf)
	linePositions := []int{-1}
	for i, r := range str {
		if r == '\n' {
			linePositions = append(linePositions, i)
		}
	}
	// 收集最近 lines 行
	if len(linePositions) > lines {
		linePositions = linePositions[len(linePositions)-lines-1:]
	}
	if len(linePositions) == 1 {
		// 无换行：整段为一行
		return []byte(str), nil
	}
	joined := str[linePositions[0]+1:]
	return []byte(joined), nil
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
		"go_version": runtime.Version(),
		// 直接用 runtime.GOOS 推断 OS，取代原"可执行文件扩展名"推断（macOS/Linux 恒为空，n-04）
		"os": runtime.GOOS,
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
	if zipPath == "" || strings.Contains(zipPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}

	// Security: only allow files inside the temp directory.
	// 必须使用"目录 + 路径分隔符"的前缀判断：仅 HasPrefix(os.TempDir()) 会让
	// C:\...\Temp2\xxx 因共享 "Temp" 前缀而绕过校验（路径穿越）。
	tempDir := filepath.Clean(os.TempDir())
	cleanPath := filepath.Clean(zipPath)
	if cleanPath != tempDir && !strings.HasPrefix(cleanPath, tempDir+string(os.PathSeparator)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// 确认目标为已存在的普通文件（拒绝目录与不存在路径）
	if fi, err := os.Stat(cleanPath); err != nil || fi.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	http.ServeFile(c.Writer, c.Request, cleanPath)
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
