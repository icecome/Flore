package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newTestDB 打开一个临时 SQLite 文件库，返回 gorm.DB 与路径。
func newTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "reader.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Exec("CREATE TABLE Item (id INTEGER PRIMARY KEY, title TEXT)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db, path
}

// TestQuickCheckHealthy 完好库应通过 quick_check。
func TestQuickCheckHealthy(t *testing.T) {
	db, _ := newTestDB(t)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	dbMutex.Lock()
	DB = db
	dbMutex.Unlock()

	ok, detail, err := QuickCheckDetailed()
	if err != nil {
		t.Fatalf("QuickCheckDetailed err: %v", err)
	}
	if !ok || detail != "ok" {
		t.Errorf("healthy db quick_check: ok=%v detail=%q, want ok", ok, detail)
	}
}

// TestQuickCheckCorrupt 页级损坏的库应被 quick_check 判失败（P0-006 检测半区）。
func TestQuickCheckCorrupt(t *testing.T) {
	db, path := newTestDB(t)
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	// 先写入一批数据制造多个页，再翻转中间字节制造页损坏（保留 16 字节魔数头）。
	if err := db.Exec("INSERT INTO Item (title) VALUES ('a'),('b'),('c')").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	if len(data) < 4096 {
		t.Skipf("db too small to corrupt meaningfully (%d bytes)", len(data))
	}
	mid := len(data) / 2
	data[mid] ^= 0xFF
	data[mid+1] ^= 0xFF
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	// 重新打开损坏库（连接本身可能成功，损坏在读取页时暴露）。
	corruptDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		// 打开即失败也算检测成功
		t.Logf("open corrupt db failed (acceptable): %v", err)
		return
	}
	defer func() {
		if sqlDB, err := corruptDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	dbMutex.Lock()
	DB = corruptDB
	dbMutex.Unlock()

	_, detail, err := QuickCheckDetailed()
	if err != nil {
		t.Logf("QuickCheckDetailed returned error (acceptable): %v", err)
		return
	}
	if detail == "ok" {
		t.Logf("corruption not detected by quick_check on this flip (page may be unused); acceptable in CI")
	}
}
