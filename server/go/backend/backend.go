// Package backend 提供可在「独立二进制」与「桌面壳自衍生子进程」两种模式下复用的
// 后端启动逻辑。
//
// 桌面壳（apps/desktop）通过自身二进制以 `--backend` 参数启动子进程来跑后端，
// 这样分发包里不再有「第二个独立可执行文件」——被 Gatekeeper 静默拦截的正是
// 这种从网上下载、带 quarantine 的嵌套二进制。子进程就是用户已「仍要打开」放行过的
// 同一份 Flore 二进制，Gatekeeper 不再拦截。独立部署（Web 版）则直接运行 cmd/main.go。
package backend

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
	"sync"
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

// Server 持有一个运行中的后端 HTTP 服务实例，供调用方在退出时优雅关闭。
type Server struct {
	srv         *http.Server
	scheduler   *services.Scheduler
	coordinator *services.FetchCoordinator
	logWriter   io.WriteCloser
	shutdownCh  chan struct{}
	stopOnce    sync.Once
}

// Start 启动后端 HTTP 服务并立即返回已监听的实际端口。
// 所有配置（PORT / DATABASE_URL / FLORE_API_TOKEN / FLORE_LOG_FILE /
// FLORE_PORT_FILE / BIND_ADDR / CORS_ORIGINS 等）均从进程环境变量读取，
// 调用方须在启动前设好。服务在独立 goroutine 中运行，进程需自行保持存活，
// 退出时调用 Server.Stop 触发优雅关闭。
func Start() (*Server, error) {
	// 加载 .env 文件作为默认值，但不覆盖已通过进程环境变量传入的值。
	if envMap, err := godotenv.Read(".env"); err == nil {
		for k, v := range envMap {
			if v != "" && os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	// 日志输出到文件，方便调试分析。
	logFile := os.Getenv("FLORE_LOG_FILE")
	if logFile == "" {
		dbPath := os.Getenv("DATABASE_URL")
		if dbPath == "" {
			dbPath = "reader.db"
		}
		logFile = filepath.Join(filepath.Dir(dbPath), "florebackend.log")
	}

	logLevel := slog.LevelInfo
	if levelStr := os.Getenv("FLORE_LOG_LEVEL"); levelStr != "" {
		if level, err := logging.ParseLogLevel(levelStr); err == nil {
			logLevel = level
		}
	}

	logWriter, err := logging.NewRotatingWriter(logFile, 10<<20, 5)
	if err != nil {
		slog.Error("failed to open log file", "path", logFile, "error", err)
		return nil, err
	}

	multiWriter := io.MultiWriter(os.Stderr, logWriter)
	textHandler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: logLevel})
	handler := logging.NewDesensitizeHandler(textHandler)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("backend log file", "path", logFile)

	if err := database.Init(); err != nil {
		slog.Error("failed to initialize database", "error", err)
		_ = logWriter.Close()
		return nil, err
	}
	if err := database.AutoMigrate(); err != nil {
		slog.Error("failed to migrate database", "error", err)
		_ = logWriter.Close()
		return nil, err
	}
	slog.Info("database migration completed")
	if err := database.RunMigrations(); err != nil {
		slog.Error("failed to run versioned migrations", "error", err)
		_ = logWriter.Close()
		return nil, err
	}
	slog.Info("versioned migrations completed")

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.SetTrustedProxies([]string{})
	r.Use(cors.New(buildCORSConfig()))
	// CSRF 防护：拦截来自不可信源的浏览器写请求。必须注册在 CORS 之后。
	r.Use(handlers.CSRFProtection())

	apiToken := os.Getenv("FLORE_API_TOKEN")
	readerHandler := handlers.NewReaderHandler()
	readerHandler.RegisterRoutes(r, apiToken)

	s := &Server{
		logWriter:  logWriter,
		shutdownCh: make(chan struct{}, 1),
	}
	// 优雅关闭端点：桌面壳退出时调用，触发后端自身干净关闭。
	r.POST("/api/shutdown", func(c *gin.Context) {
		select {
		case s.shutdownCh <- struct{}{}:
		default:
		}
		c.JSON(http.StatusOK, gin.H{"status": "shutting_down"})
	})

	// 创建并启动抓取协调器（唯一调度权威）。
	readerService := services.NewReaderService()
	coordinator := services.NewFetchCoordinator(readerService)
	readerService.SetCoordinator(coordinator)
	coordinator.Start()
	scheduler := services.NewScheduler(readerService, 5*time.Minute)
	scheduler.Start()

	s.coordinator = coordinator
	s.scheduler = scheduler

	bindAddr := os.Getenv("BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	port := resolvePort()
	srv := &http.Server{
		Addr:              bindAddr + ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 自行创建 listener：PORT=0 时由系统分配空闲高位端口，消除 TOCTOU 窗口；
	// 绑定成功后把实际端口写入 FLORE_PORT_FILE 供桌面壳读取。
	ln, err := net.Listen("tcp", bindAddr+":"+port)
	if err != nil {
		slog.Error("failed to listen", "addr", bindAddr+":"+port, "error", err)
		_ = logWriter.Close()
		return nil, err
	}
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		slog.Info("server listening", "addr", ln.Addr().String())
		if portFile := os.Getenv("FLORE_PORT_FILE"); portFile != "" {
			if werr := os.WriteFile(portFile, []byte(strconv.Itoa(tcpAddr.Port)), 0644); werr != nil {
				slog.Warn("failed to write port file", "path", portFile, "error", werr)
			}
		}
	}
	s.srv = srv

	go func() {
		if serr := srv.Serve(ln); serr != nil && serr != http.ErrServerClosed {
			slog.Error("server failed", "error", serr)
		}
	}()

	return s, nil
}

// Stop 触发后端优雅关闭（WAL checkpoint、连接释放、调度器停止），幂等。
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if s.srv != nil {
			if err := s.srv.Shutdown(ctx); err != nil {
				slog.Error("server forced to shutdown", "error", err)
			}
		}
		if s.scheduler != nil {
			s.scheduler.Stop()
		}
		if s.coordinator != nil {
			s.coordinator.Stop()
		}
		// 停止抓取协调器后再关闭数据库，避免关闭时访问已关闭连接。
		if sqlDB, err := database.DB.DB(); err == nil {
			if closeErr := sqlDB.Close(); closeErr != nil {
				slog.Warn("failed to close database connection", "error", closeErr)
			}
		} else {
			slog.Warn("failed to get underlying sql db for close", "error", err)
		}
		_ = s.logWriter.Close()
		slog.Info("server exited")
	})
}

// RunBlocking 阻塞当前进程，直到收到 SIGINT/SIGTERM 或 /api/shutdown 请求，
// 随后触发优雅关闭。供「自衍生子进程」与独立二进制入口使用。
func (s *Server) RunBlocking() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		slog.Info("shutting down server (signal)...")
	case <-s.shutdownCh:
		slog.Info("shutting down server (api shutdown request)...")
	}
	s.Stop()
}

// buildCORSConfig 跨域来源校验。
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
		config.AllowAllOrigins = true
	case envCORSOrigins != "":
		config.AllowOrigins = strings.Split(envCORSOrigins, ",")
	default:
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
