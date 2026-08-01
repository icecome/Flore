package logging

import (
	"fmt"
	"os"
	"path/filepath"
)

// NewRotatingLogFile 用于 desktop app，返回 *os.File
func NewRotatingLogFile(path string, maxSize int, maxBackups int) (*os.File, error) {
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

	if info.Size() > int64(maxSize) {
		f.Close()
		if err := rotateLogFile(path, maxSize, maxBackups); err != nil {
			return nil, fmt.Errorf("rotate log on startup: %w", err)
		}
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, fmt.Errorf("reopen log file: %w", err)
		}
	}

	return f, nil
}

func rotateLogFile(path string, maxSize int, maxBackups int) error {
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
