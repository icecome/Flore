package backend

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/handlers"
)

// RecoveryServer 是数据库损坏/不可读时进入的降级服务（P0-006）。
// 目标：主库不可用时，前端仍能渲染恢复界面，列出可用的备份与 .bak/<timestamp> 快照，
// 并提供一键恢复到数据目录——而不是进程静默退出或把用户丢到 onboarding。
// 它刻意不触碰 ReaderService/Coordinator/Scheduler（这些依赖可用的主库），
// 只暴露数据目录信息、备份/快照清单与文件复制恢复。
type RecoveryServer struct {
	*Server
	dbPath string
}

// RecoveryInfo 恢复界面所需的数据目录信息。
type RecoveryInfo struct {
	DBPath  string `json:"dbPath"`
	DataDir string `json:"dataDir"`
}

// RecoveryBackup 一个可恢复的备份（本地 .zip 或恢复前自动保存的 .bak/<timestamp> 快照）。
type RecoveryBackup struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "zip" | "snapshot"
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	Path    string `json:"path"`
}

// startRecoveryServer 启动降级恢复服务。端口解析与主服务一致，端口文件同样写入。
func startRecoveryServer(apiToken string, logWriter io.WriteCloser) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.SetTrustedProxies([]string{})
	r.Use(cors.New(buildCORSConfig()))
	r.Use(handlers.CSRFProtection())

	dbPath := database.DBPath()
	rs := &RecoveryServer{Server: &Server{}, dbPath: dbPath}
	rs.logWriter = logWriter
	rs.shutdownCh = make(chan struct{}, 1)

	api := r.Group("/api")
	auth := handlers.AuthMiddleware(apiToken)
	guard := func(c *gin.Context) {
		if auth != nil {
			auth(c)
			return
		}
		c.Next()
	}

	api.GET("/recovery/info", func(c *gin.Context) {
		dir := filepath.Dir(dbPath)
		c.JSON(http.StatusOK, RecoveryInfo{DBPath: dbPath, DataDir: dir})
	})

	api.GET("/recovery/backups", guard, func(c *gin.Context) {
		list, err := rs.listRecoveryBackups()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list backups"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"backups": list})
	})

	// POST /recovery/restore  body: {"path":"<绝对路径>"}
	// 将选中的 .db 快照（或从 .zip 解出的 .db）复制到主库路径；成功后需重启应用生效。
	api.POST("/recovery/restore", guard, func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		src, err := rs.resolveBackupSource(req.Path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": safeRecoveryError(err)})
			return
		}
		defer os.Remove(src.tmp) // 非临时文件时 tmp 为空
		if err := rs.copyOverLiveDB(src.tmp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore database"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	port, err := resolvePort()
	if err != nil {
		slog.Error("recovery: invalid PORT configuration", "error", err)
		_ = logWriter.Close()
		return nil
	}
	ln, lerr := net.Listen("tcp", bindAddr+":"+port)
	if lerr != nil {
		slog.Error("recovery: failed to listen", "addr", bindAddr+":"+port, "error", lerr)
		_ = logWriter.Close()
		return nil
	}
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		if portFile := os.Getenv("FLORE_PORT_FILE"); portFile != "" {
			if werr := os.WriteFile(portFile, []byte(fmt.Sprint(tcpAddr.Port)), 0600); werr != nil {
				slog.Warn("recovery: failed to write port file", "path", portFile, "error", werr)
			}
		}
	}
	srv := &http.Server{
		Addr:              bindAddr + ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	rs.srv = srv
	go func() {
		if serr := srv.Serve(ln); serr != nil && serr != http.ErrServerClosed {
			slog.Error("recovery server failed", "error", serr)
		}
	}()
	slog.Info("recovery server listening", "addr", ln.Addr().String(), "db", dbPath)
	return rs.Server
}

// listRecoveryBackups 列出数据目录下的 .zip 备份与 .bak/<timestamp> 快照（按时间倒序）。
func (rs *RecoveryServer) listRecoveryBackups() ([]RecoveryBackup, error) {
	dir := filepath.Dir(rs.dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RecoveryBackup{}, nil
		}
		return nil, err
	}
	var out []RecoveryBackup
	appendEntry := func(name, typ, full string, info os.FileInfo) {
		out = append(out, RecoveryBackup{
			Name:    name,
			Type:    typ,
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Path:    full,
		})
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		full := filepath.Join(dir, name)
		switch {
		case strings.HasSuffix(name, ".zip"):
			appendEntry(name, "zip", full, info)
		case strings.HasPrefix(name, filepath.Base(rs.dbPath)+".bak."):
			appendEntry(name, "snapshot", full, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// backupSource 是待恢复源：tmp 为可直接复制到主库路径的 .db 文件（zip 会先解出到临时文件）。
type backupSource struct {
	tmp string
}

// resolveBackupSource 校验并解析用户选择的备份路径：
//   - 必须是数据目录内的 .db 快照，或数据目录内的 .zip（解出其中 .db 到临时文件）；
//   - 拒绝路径穿越与目录外路径。
func (rs *RecoveryServer) resolveBackupSource(path string) (*backupSource, error) {
	dir := filepath.Clean(filepath.Dir(rs.dbPath))
	clean := filepath.Clean(path)
	if clean != dir && !strings.HasPrefix(clean, dir+string(os.PathSeparator)) {
		return nil, fmt.Errorf("backup path outside data dir")
	}
	info, err := os.Stat(clean)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("backup not found")
	}
	switch {
	case strings.HasSuffix(clean, ".db"):
		return &backupSource{tmp: clean}, nil
	case strings.HasSuffix(clean, ".zip"):
		tmp, err := extractDBFromZipForRecovery(clean)
		if err != nil {
			return nil, err
		}
		return &backupSource{tmp: tmp}, nil
	default:
		return nil, fmt.Errorf("unsupported backup type")
	}
}

// copyOverLiveDB 原子地用 src 覆盖主库路径（临时文件 + rename），并在覆盖前保存当前库快照。
// 注意：WAL 模式下仅复制 .db 主文件会丢失未 checkpoint 的 WAL 内容；恢复场景主库已损坏，
// 以备份为准，覆盖后需重启应用重新打开。覆盖前仍将当前 .db 另存为 .bak/<timestamp> 兜底。
func (rs *RecoveryServer) copyOverLiveDB(src string) error {
	dir := filepath.Dir(rs.dbPath)
	if _, err := os.Stat(rs.dbPath); err == nil {
		snap := filepath.Join(dir, fmt.Sprintf("%s.bak.%s", filepath.Base(rs.dbPath), time.Now().Format("20060102-150405")))
		if err := os.Rename(rs.dbPath, snap); err != nil {
			slog.Warn("recovery: failed to snapshot current db before restore", "error", err)
		}
	}
	if err := copyFileForRecovery(src, rs.dbPath); err != nil {
		return err
	}
	// 清理可能残留的 WAL/SHM，避免旧日志覆盖恢复出的主库
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(rs.dbPath + suffix)
	}
	slog.Info("recovery: database restored", "dst", rs.dbPath, "src", src)
	return nil
}

func copyFileForRecovery(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "flore-recovery-*.db")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, io.LimitReader(in, 512<<20)); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// extractDBFromZipForRecovery 从备份 zip 中解出 .db 到临时文件并返回其路径。
func extractDBFromZipForRecovery(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".db") {
			if f.UncompressedSize64 > 512<<20 {
				return "", fmt.Errorf("backup db too large")
			}
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			tmp, err := os.CreateTemp("", "flore-recovery-*.db")
			if err != nil {
				rc.Close()
				return "", err
			}
			tmpName := tmp.Name()
			if _, err := io.Copy(tmp, io.LimitReader(rc, 512<<20+1)); err != nil {
				tmp.Close()
				rc.Close()
				os.Remove(tmpName)
				return "", err
			}
			tmp.Close()
			rc.Close()
			return tmpName, nil
		}
	}
	return "", fmt.Errorf("no database file in backup")
}

// safeRecoveryError 返回可读错误，不泄露内部路径细节。
func safeRecoveryError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "outside data dir"):
		return "所选备份不在数据目录内"
	case strings.Contains(msg, "not found"):
		return "备份文件不存在"
	case strings.Contains(msg, "unsupported"):
		return "不支持的备份类型"
	case strings.Contains(msg, "too large"):
		return "备份中的数据库过大"
	case strings.Contains(msg, "no database file"):
		return "备份中没有数据库文件"
	default:
		return "恢复失败"
	}
}

// Stop 触发恢复服务优雅关闭（复用主 Server.Stop 的安全关闭路径）。
func (rs *RecoveryServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rs.srv != nil {
		_ = rs.srv.Shutdown(ctx)
	}
	if rs.logWriter != nil {
		_ = rs.logWriter.Close()
	}
}
