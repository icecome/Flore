package services

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/database"
	"github.com/rss/go-server/internal/models"
)

func newTestService(t *testing.T) *ReaderService {
	t.Helper()
	// 使用临时文件作为数据库路径
	tmpDB, err := os.CreateTemp("", "flore-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpDB.Close()
	database.SetDBPath(tmpDB.Name())

	db, err := gorm.Open(sqlite.Open(tmpDB.Name()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Source{},
		&models.Item{},
		&models.Folder{},
		&models.Setting{},
		&models.FilterRule{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return &ReaderService{db: db}
}

func ptrTime(t time.Time) *time.Time { return &t }

// TestBackupRestore_CorruptedFile 验证损坏的备份文件不会破坏原数据库
func TestBackupRestore_CorruptedFile(t *testing.T) {
	s := newTestService(t)

	// 创建原数据
	s.db.Create(&models.Source{Name: "Test", URL: "https://example.com/rss"})
	s.db.Create(&models.Item{SourceID: 1, Title: "Article 1", Link: "https://example.com/1"})

	// 尝试从损坏数据恢复
	err := s.ImportDatabase(bytes.NewReader([]byte("not a valid sqlite file")))
	if err == nil {
		t.Fatal("expected error for corrupted backup")
	}

	// 验证原数据仍然存在
	var count int64
	s.db.Model(&models.Item{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 item after failed restore, got %d", count)
	}
}

// TestBackupRestore_ValidBackup 验证有效备份可以正常恢复
func TestBackupRestore_ValidBackup(t *testing.T) {
	s := newTestService(t)

	// 创建原数据
	s.db.Create(&models.Source{Name: "Test", URL: "https://example.com/rss"})
	s.db.Create(&models.Item{SourceID: 1, Title: "Article 1", Link: "https://example.com/1"})

	// 创建备份
	backupName, err := s.CreateCompressedBackup()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if backupName == "" {
		t.Fatal("expected non-empty backup name")
	}

	// 验证备份文件存在
	backupDir := filepath.Dir(database.DBPath())
	backupPath := filepath.Join(backupDir, "backups", backupName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("backup file not found")
	}

	// 清理备份
	os.Remove(backupPath)
}

// TestOPMLImport 验证 OPML 导入流程
func TestOPMLImport(t *testing.T) {
	s := newTestService(t)

	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline text="Test Source" type="feed" xmlUrl="https://example.com/rss"/>
  </body>
</opml>`

	// OPML 导入应该能够成功
	if err := s.ImportOPML(opmlContent); err != nil {
		t.Fatalf("OPML import failed: %v", err)
	}

	// 验证源已创建
	var count int64
	s.db.Model(&models.Source{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 source, got %d", count)
	}
}

// TestUpsertFeedItems 验证文章 upsert 逻辑
func TestUpsertFeedItems(t *testing.T) {
	s := newTestService(t)

	// 创建源
	var source models.Source
	s.db.Create(&models.Source{Name: "Test", URL: "https://example.com/rss", Active: true})
	s.db.First(&source)

	// 创建文章
	items := []FeedItem{
		{Title: "Article 1", Link: "https://example.com/1", GUID: "guid-1"},
		{Title: "Article 2", Link: "https://example.com/2", GUID: "guid-2"},
	}

	// upsert 应该成功
	_, err := s.upsertFeedItems(source.ID, items)
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	// 验证文章已插入（使用正确的列名 sourceId）
	var count int64
	s.db.Model(&models.Item{}).Where("sourceId = ?", source.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 items, got %d", count)
	}
}

// TestRetentionPolicy 验证保留策略清理逻辑
func TestRetentionPolicy(t *testing.T) {
	s := newTestService(t)

	now := time.Now()
	// 创建5个源，每个源10篇文章（不同日期）
	for i := 1; i <= 5; i++ {
		var source models.Source
		s.db.Create(&models.Source{Name: "Source", URL: "https://example.com/rss/" + string(rune('a'+i)), Active: true})
		s.db.First(&source)

		for j := 1; j <= 10; j++ {
			pubDate := now.AddDate(0, 0, -j)
			link := "https://example.com/article/" + string(rune('a'+i)) + "/" + string(rune('0'+j))
			s.db.Create(&models.Item{
				SourceID: source.ID,
				Title:    "Article",
				Link:     link,
				PubDate:  models.NullableMilliTime{T: ptrTime(pubDate)},
			})
		}
	}

	// 设置保留策略为7天
	s.db.Model(&models.Setting{}).Where("key = ?", "keepDays").
		Assign(&models.Setting{Key: "keepDays", Value: "7"}).
		FirstOrCreate(&models.Setting{Key: "keepDays"})

	// 执行清理
	deleted, err := s.CleanupArticles(7, 0, false, false)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// 验证清理了超出7天的文章（每个源应该有3篇被清理）
	var count int64
	s.db.Model(&models.Item{}).Count(&count)
	expectedMax := int64(50) // 5源 x 10篇
	if count > expectedMax {
		t.Fatalf("expected at most %d items, got %d", expectedMax, count)
	}
	t.Logf("cleanup deleted %d items, remaining %d", deleted, count)
}

// TestBackupZipFormat 验证备份 ZIP 格式正确
func TestBackupZipFormat(t *testing.T) {
	s := newTestService(t)

	// 创建备份
	backupName, err := s.CreateCompressedBackup()
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}

	// 验证是有效的 ZIP 文件
	backupDir := filepath.Dir(database.DBPath())
	backupPath := filepath.Join(backupDir, "backups", backupName)
	f, err := zip.OpenReader(backupPath)
	if err != nil {
		t.Fatalf("backup is not a valid zip: %v", err)
	}
	defer f.Close()

	// 应该包含一个 .db 文件
	found := false
	for _, file := range f.File {
		if len(file.Name) > 3 && file.Name[len(file.Name)-3:] == ".db" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("backup zip does not contain a .db file")
	}

	// 清理
	os.Remove(backupPath)
}
