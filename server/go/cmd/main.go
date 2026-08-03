package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/handlers"
	"github.com/rss/go-server/internal/logging"
	"github.com/rss/go-server/internal/services"
)

func main() {
	// 加载 .env 文件作为默认值，但不覆盖已通过进程环境变量传入的值。
	// 桌面端启动时会通过 cmd.Env 传入 DATABASE_URL/PORT 等关键配置，
	// 必须保证这些值不被本地 .env 文件覆盖。
	if envMap, err := godotenv.Read(".env"); err == nil {
		for k, v := range envMap {
			if v != "" && os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	// 配置日志输出到文件，方便调试分析。
	// 优先使用 FLORE_LOG_FILE 环境变量；否则使用数据库同目录下的 florebackend.log
	logFile := os.Getenv("FLORE_LOG_FILE")
	if logFile == "" {
		dbPath := os.Getenv("DATABASE_URL")
		if dbPath == "" {
			dbPath = "reader.db"
		}
		logFile = filepath.Join(filepath.Dir(dbPath), "florebackend.log")
	}

	// 日志级别由 FLORE_LOG_LEVEL 环境变量控制，默认 Info
	logLevel := slog.LevelInfo
	if levelStr := os.Getenv("FLORE_LOG_LEVEL"); levelStr != "" {
		if level, err := logging.ParseLogLevel(levelStr); err == nil {
			logLevel = level
		}
	}

	// 打开日志文件（追加模式）并配置轮转
	logWriter, err := logging.NewRotatingWriter(logFile, 10<<20, 5)
	if err != nil {
		slog.Error("failed to open log file", "path", logFile, "error", err)
		os.Exit(1)
	}
	defer logWriter.Close()

	multiWriter := io.MultiWriter(os.Stderr, logWriter)
	textHandler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: logLevel})
	handler := logging.NewDesensitizeHandler(textHandler)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("backend log file", "path", logFile)

	if err := database.Init(); err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}

	if err := database.AutoMigrate(); err != nil {
		slog.Error("failed to migrate database", "error", err)
		os.Exit(1)
	}
	slog.Info("database migration completed")

	if err := database.RunMigrations(); err != nil {
		slog.Error("failed to run versioned migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("versioned migrations completed")

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.SetTrustedProxies([]string{})
	r.Use(cors.New(buildCORSConfig()))
	// CSRF 防护：拦截来自不可信源的浏览器写请求（详见 handlers.CSRFProtection）。
	// 必须注册在 CORS 之后，以便预检 OPTIONS 由 CORS 中间件先行处理。
	r.Use(handlers.CSRFProtection())

	apiToken := os.Getenv("FLORE_API_TOKEN")
	readerHandler := handlers.NewReaderHandler()
	readerHandler.RegisterRoutes(r, apiToken)

	// 优雅关闭端点：桌面壳退出时调用，触发后端自身干净关闭（含 WAL checkpoint 与连接释放）。
	// 仅绑定 127.0.0.1，与 /health 同安全等级（桌面模式无 token）。
	shutdownCh := make(chan struct{}, 1)
	r.POST("/api/shutdown", func(c *gin.Context) {
		select {
		case shutdownCh <- struct{}{}:
		default:
		}
		c.JSON(http.StatusOK, gin.H{"status": "shutting_down"})
	})

	// 创建并启动抓取协调器（唯一调度权威），注入到 ReaderService 供 handler/scheduler 共用
	readerService := services.NewReaderService()
	coordinator := services.NewFetchCoordinator(readerService)
	readerService.SetCoordinator(coordinator)
	coordinator.Start()

	scheduler := services.NewScheduler(readerService, 5*time.Minute)
	scheduler.Start()

	port := resolvePort()
	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	srv := &http.Server{
		Addr:              bindAddr + ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 自行创建 listener：
	//   - PORT=0 时由系统分配空闲高位端口，消除"桌面壳探测端口后交给后端绑定"的 TOCTOU 窗口；
	//   - 绑定成功后把实际端口写入 FLORE_PORT_FILE 指定文件（供桌面壳读取，用于健康检查与 API 基址）。
	ln, err := net.Listen("tcp", bindAddr+":"+port)
	if err != nil {
		slog.Error("failed to listen", "addr", bindAddr+":"+port, "error", err)
		os.Exit(1)
	}
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		slog.Info("server listening", "addr", ln.Addr().String())
		if portFile := os.Getenv("FLORE_PORT_FILE"); portFile != "" {
			if werr := os.WriteFile(portFile, []byte(strconv.Itoa(tcpAddr.Port)), 0644); werr != nil {
				slog.Warn("failed to write port file", "path", portFile, "error", werr)
			}
		}
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	gracefulShutdown(srv, scheduler, coordinator, errCh, shutdownCh)
}

func gracefulShutdown(srv *http.Server, scheduler *services.Scheduler, coordinator *services.FetchCoordinator, errCh <-chan error, stopCh <-chan struct{}) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		slog.Info("shutting down server (signal)...")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-stopCh:
		slog.Info("shutting down server (api shutdown request)...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
	scheduler.Stop()
	// 停止抓取协调器，等待所有进行中抓取完成，避免关闭数据库时访问已关闭连接
	coordinator.Stop()
	// 检查 sqlDB.Close 错误，避免静默丢弃关闭失败
	if sqlDB, err := database.DB.DB(); err == nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			slog.Warn("failed to close database connection", "error", closeErr)
		}
	} else {
		slog.Warn("failed to get underlying sql db for close", "error", err)
	}
	slog.Info("server exited")
}

// buildCORSConfig 跨域来源校验。
//   - 显式设置 CORS_ORIGINS=* 或具体域名：Web 部署场景，沿用旧逻辑放开。
//   - 未设置（桌面场景默认）：用 AllowOriginFunc 动态反射本地源，
//     放行 127.0.0.1/localhost 动态端口与 Wails WebView 源，
//     拒绝任意外网源与 opaque origin（Origin: null，file:// 页面/沙箱 iframe 会发送），
//     避免绑定 127.0.0.1 时被本机恶意网页读取数据。
//
// 注意：AllowOriginFunc 与 AllowAllOrigins 互斥；AllowCredentials 必须为 false，
// 否则 gin-contrib/cors 会因「* + credentials」直接 panic。前端 fetch 不带凭证，
// 后端写接口由 FLORE_API_TOKEN 与 CSRFProtection 中间件双重保护。
func buildCORSConfig() cors.Config {
	config := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}

	envCORSOrigins := os.Getenv("CORS_ORIGINS")
	switch {
	case envCORSOrigins == "*":
		// Web 部署：显式放开任意源（需配合 AllowCredentials=false）。
		config.AllowAllOrigins = true
	case envCORSOrigins != "":
		// Web 部署：指定具体允许源。
		config.AllowOrigins = strings.Split(envCORSOrigins, ",")
	default:
		// 桌面默认：动态反射本地源；opaque origin（"null"）一律拒绝。
		config.AllowOriginFunc = handlers.IsLocalOrigin
	}
	return config
}

func resolvePort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "3002"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 0 || portNum > 65535 {
		slog.Error("invalid PORT, must be a number between 0 and 65535", "port", port)
		os.Exit(1)
	}
	return port
}
