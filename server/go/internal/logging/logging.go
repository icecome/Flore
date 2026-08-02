package logging

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// RotatingWriter 实现日志轮转
type RotatingWriter struct {
	mu          sync.Mutex
	path        string
	maxSize     int64
	maxBackups  int
	file        *os.File
	currentSize int64
}

// NewRotatingWriter 创建支持轮转的日志写入器
func NewRotatingWriter(path string, maxSize int, maxBackups int) (*RotatingWriter, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	return &RotatingWriter{
		path:        path,
		maxSize:     int64(maxSize),
		maxBackups:  maxBackups,
		file:        f,
		currentSize: info.Size(),
	}, nil
}

func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentSize+int64(len(p)) > w.maxSize {
		if closeErr := w.file.Close(); closeErr != nil {
			return 0, fmt.Errorf("close log file before rotate: %w", closeErr)
		}
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("rotate log: %w", err)
		}
	}

	n, err = w.file.Write(p)
	w.currentSize += int64(n)
	return n, err
}

func (w *RotatingWriter) rotate() error {
	if w.maxBackups > 0 {
		oldPath := w.path + "." + fmt.Sprint(w.maxBackups)
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old backup: %w", err)
		}
	}

	for i := w.maxBackups - 1; i >= 1; i-- {
		src := w.path + "." + fmt.Sprint(i)
		dst := w.path + "." + fmt.Sprint(i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("rename backup %d: %w", i, err)
			}
		}
	}

	if err := os.Rename(w.path, w.path+".1"); err != nil {
		return fmt.Errorf("rename current log: %w", err)
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open new log file: %w", err)
	}
	w.file = f
	w.currentSize = 0
	return nil
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// ParseLogLevel 解析日志级别字符串
func ParseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "DEBUG", "debug":
		return slog.LevelDebug, nil
	case "INFO", "info":
		return slog.LevelInfo, nil
	case "WARN", "warn":
		return slog.LevelWarn, nil
	case "ERROR", "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", s)
	}
}

// DesensitizeAttr 对日志属性进行脱敏处理（SEC-01）
func DesensitizeAttr(key string, value slog.Value) slog.Value {
	switch key {
	case "url", "referer", "ref":
		return slog.StringValue(sanitizeURL(value.String()))
	case "source":
		return slog.StringValue("source_" + value.String())
	case "path", "db_path", "database":
		return slog.StringValue("[REDACTED_PATH]")
	case "error":
		return slog.StringValue(sanitizeError(value.String()))
	default:
		return value
	}
}

func sanitizeURL(s string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		if u, err := url.Parse(s); err == nil {
			return u.Scheme + "://" + u.Host
		}
	}
	return s
}

func sanitizeError(s string) string {
	// 使用正则替换所有 URL 路径部分
	re := regexp.MustCompile(`https?://[^/\s?]+(?:/[^\s]*)?`)
	return re.ReplaceAllStringFunc(s, func(match string) string {
		if u, err := url.Parse(match); err == nil {
			return u.Scheme + "://" + u.Host
		}
		return match
	})
}

// NewDesensitizeHandler 创建带脱敏的 slog.Handler
func NewDesensitizeHandler(handler slog.Handler) slog.Handler {
	return &desensitizeHandler{next: handler}
}

type desensitizeHandler struct {
	next slog.Handler
}

func (h *desensitizeHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.next.Enabled(nil, level)
}

func (h *desensitizeHandler) Handle(_ context.Context, r slog.Record) error {
	// 脱敏记录中的属性：创建新 Record 并替换属性
	newR := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	var desensitizedAttrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		a.Value = DesensitizeAttr(a.Key, a.Value)
		desensitizedAttrs = append(desensitizedAttrs, a)
		return true
	})
	newR.AddAttrs(desensitizedAttrs...)
	return h.next.Handle(nil, newR)
}

func (h *desensitizeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	filtered := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		attr.Value = DesensitizeAttr(attr.Key, attr.Value)
		filtered = append(filtered, attr)
	}
	return &desensitizeHandler{next: h.next.WithAttrs(filtered)}
}

func (h *desensitizeHandler) WithGroup(name string) slog.Handler {
	return h
}

// ReplaceAttr 是 slog.HandlerOptions.ReplaceAttr 的实现
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case "url", "referer", "ref", "source", "path", "db_path", "database", "error":
		if a.Value.Kind() == slog.KindString {
			a.Value = DesensitizeAttr(a.Key, a.Value)
		}
	}
	return a
}
