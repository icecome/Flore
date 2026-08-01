package main

import (
	"context"
	"io"
	"log/slog"
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
	// 优先使用 FLORE_LOG_FILE 环境变量；否则使用数据库同目录下的 flore-backend.log
	logFile := os.Getenv("FLORE_LOG_FILE")
	if logFile == "" {
		dbPath := os.Getenv("DATABASE_URL")
		if dbPath == "" {
			dbPath = "reader.db"
		}
		logFile = filepath.Join(filepath.Dir(dbPath), "flore-backend.log")
	}
	if f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, f), &slog.HandlerOptions{Level: slog.LevelDebug})))
	}
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

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.SetTrustedProxies([]string{})
	r.Use(cors.New(buildCORSConfig()))

	apiToken := os.Getenv("FLORE_API_TOKEN")
	readerHandler := handlers.NewReaderHandler()
	readerHandler.RegisterRoutes(r, apiToken)

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
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	gracefulShutdown(srv, scheduler, coordinator, errCh)
}

func gracefulShutdown(srv *http.Server, scheduler *services.Scheduler, coordinator *services.FetchCoordinator, errCh <-chan error) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		slog.Info("shutting down server...")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
	scheduler.Stop()
	// 停止抓取协调器，等待所有进行中抓取完成，避免关闭数据库时访问已关闭连接
	coordinator.Stop()
	if sqlDB, err := database.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	slog.Info("server exited")
}

func buildCORSConfig() cors.Config {
	config := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}

	envCORSOrigins := os.Getenv("CORS_ORIGINS")
	if envCORSOrigins == "*" {
		config.AllowAllOrigins = true
	} else if envCORSOrigins != "" {
		config.AllowOrigins = strings.Split(envCORSOrigins, ",")
	} else {
		config.AllowOrigins = []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:34115",
			"http://wails.localhost",
			"https://wails.localhost",
		}
	}
	return config
}

func resolvePort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return "3002"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		slog.Error("invalid PORT, must be a number between 1 and 65535", "port", port)
		os.Exit(1)
	}
	return port
}
