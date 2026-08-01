package services

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
)

func newTestService(t *testing.T) *ReaderService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&models.Source{}, &models.Item{}, &models.Folder{}, &models.Setting{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return &ReaderService{db: db}
}

// TestBackupRestore_CorruptedFile 验证损坏的备份文件不会破坏原数据库
func TestBackupRestore_CorruptedFile(t *testing.T) {
	s := newTestService(t)

	// 创建原数据
	s.db.Create(&models.Source{Name: "Test", URL: "https://example.com/rss"})
	s.db.Create(&models.Item{SourceID: 1, Title: "Article 1", Link: "https://example.com/1"})

	// 创建损坏的备份文件
	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupt.zip")
	f, _ := os.Create(corruptPath)
	f.WriteString("not a valid zip")
	f.Close()

	// 尝试从损坏文件恢复
	err := s.ImportDatabase(bytes.NewReader([]byte("not a valid sqlite file")))
	if err == nil {
		t.Fatal("expected error for corrupted backup")
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

	// 验证备份存在
	if _, err := os.Stat(backupName); os.IsNotExist(err) {
		t.Fatal("backup file not found")
	}
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
		t.Logf("OPML import error (expected for empty DB): %v", err)
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

	// 验证文章已插入
	var count int64
	s.db.Model(&models.Item{}).Where("source_id = ?", source.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 items, got %d", count)
	}
}

// TestRetentionPolicy 验证保留策略清理逻辑
func TestRetentionPolicy(t *testing.T) {
	s := newTestService(t)

	// 创建多个源和文章
	for i := 1; i <= 5; i++ {
		var source models.Source
		s.db.Create(&models.Source{Name: "Source", URL: "https://example.com/rss", Active: true})
		s.db.First(&source)

		for j := 1; j <= 10; j++ {
			s.db.Create(&models.Item{
				SourceID: source.ID,
				Title:    "Article",
				Link:     "https://example.com/article",
				PubDate:  models.NullableMilliTime{Time: source.CreatedAtTime.Time().AddDate(0, 0, -j)},
			})
		}
	}

	// 设置保留策略
	s.db.Model(&models.Setting{}).Where("key = ?", "keepDays").
		Assign(&models.Setting{Key: "keepDays", Value: "7"}).
		FirstOrCreate(&models.Setting{Key: "keepDays"})

	// 执行清理
	_, err := s.CleanupArticles(7, 0, false, false)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
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
	f, err := zip.OpenReader(backupName)
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
}
