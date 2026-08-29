package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// logFile 将日志写入用户数据目录，便于在 GUI 模式下调试后端启动问题。
type logFile struct {
	mu   sync.Mutex
	file *os.File
	path string
	// maxSize / maxBackups 供运行期轮转使用（N4）
	maxSize    int64
	maxBackups int
}

const (
	logMaxSize    = 10 << 20
	logMaxBackups = 5
)

func newLogFile(dir string) *logFile {
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "floredesktop.log")

	f, err := newRotatingLogFile(logPath, logMaxSize, logMaxBackups)
	if err != nil {
		// 降级到 os.DevNull：绝不能返回 nil *os.File，
		// 否则子进程的 Stdout/Stderr 会拿到 typed-nil 接口值导致写入 panic（M3）。
		devnull, devErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if devErr != nil {
			return &logFile{file: nil, path: logPath}
		}
		return &logFile{file: devnull, path: logPath}
	}
	return &logFile{
		file:       f,
		path:       logPath,
		maxSize:    logMaxSize,
		maxBackups: logMaxBackups,
	}
}

// newRotatingLogFile 打开日志文件，支持轮转
func newRotatingLogFile(path string, maxSize int64, maxBackups int) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	if info.Size() > maxSize {
		f.Close()
		if err := rotateLogFile(path, maxBackups); err != nil {
			return nil, fmt.Errorf("rotate log: %w", err)
		}
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, fmt.Errorf("reopen log file: %w", err)
		}
	}

	return f, nil
}

func rotateLogFile(path string, maxBackups int) error {
	if maxBackups > 0 {
		oldPath := path + "." + fmt.Sprint(maxBackups)
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove old backup: %w", err)
		}
	}
	for i := maxBackups - 1; i >= 1; i-- {
		src := path + "." + fmt.Sprint(i)
		dst := path + "." + fmt.Sprint(i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				return fmt.Errorf("rename backup %d: %w", i, err)
			}
		}
	}
	return os.Rename(path, path+".1")
}

func (l *logFile) Printf(format string, args ...interface{}) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "["+time.Now().Format("15:04:05")+"] "+format+"\n", args...)
	_ = l.file.Sync()
	l.rotateIfNeededLocked()
}

// rotateIfNeededLocked 在运行期检查体积并轮转，避免长时间运行的实例日志无限增长（N4）。
// 注意：子进程的 Stdout/Stderr 已绑定到旧句柄，轮转后其输出会继续写入被重命名的文件，
// 这是可接受的取舍（后端另有独立 FLORE_LOG_FILE）。
func (l *logFile) rotateIfNeededLocked() {
	if l.file == nil || l.path == "" || l.maxSize <= 0 {
		return
	}
	info, err := l.file.Stat()
	if err != nil || info.Size() <= l.maxSize {
		return
	}
	_ = l.file.Close()
	l.file = nil
	if err := rotateLogFile(l.path, l.maxBackups); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return
	}
	l.file = f
}

// writer 返回可安全传给子进程的 io.Writer；日志不可用时返回 nil（调用方需判空）。
func (l *logFile) writer() *os.File {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file
}

func (l *logFile) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}
