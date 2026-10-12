package services

import (
	"testing"

	"github.com/rss/go-server/internal/models"
)

func TestGetSourcesMergesHealthAndUnread(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	folder, err := s.CreateFolder("分组")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	sources := []models.Source{
		{Name: "健康源", URL: "https://ok.example.com/feed", ListRule: "rss", FolderID: &folder.ID, Active: true},
		{Name: "故障源", URL: "https://bad.example.com/feed", ListRule: "rss", Active: true},
	}
	for i := range sources {
		if err := db.Create(&sources[i]).Error; err != nil {
			t.Fatalf("create source %d: %v", i, err)
		}
	}
	lastErr := "timeout"
	if err := db.Create(&models.SourceHealth{SourceID: sources[1].ID, FetchFailCount: 3, LastError: &lastErr}).Error; err != nil {
		t.Fatalf("create health: %v", err)
	}
	items := []models.Item{
		{SourceID: sources[0].ID, Title: "未读1", Link: "https://ok.example.com/1"},
		{SourceID: sources[0].ID, Title: "已读1", Link: "https://ok.example.com/2", IsRead: true},
		{SourceID: sources[0].ID, Title: "未读2", Link: "https://ok.example.com/3"},
	}
	for i := range items {
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("create item %d: %v", i, err)
		}
	}

	got, err := s.GetSources()
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("sources = %d, want 2", len(got))
	}
	for _, src := range got {
		switch src.Name {
		case "健康源":
			if src.UnreadCount != 2 {
				t.Errorf("健康源 unread = %d, want 2", src.UnreadCount)
			}
			if src.FetchFailCount != 0 {
				t.Errorf("健康源 failCount = %d, want 0", src.FetchFailCount)
			}
		case "故障源":
			if src.UnreadCount != 0 {
				t.Errorf("故障源 unread = %d, want 0", src.UnreadCount)
			}
			if src.FetchFailCount != 3 || src.LastError == nil || *src.LastError != "timeout" {
				t.Errorf("故障源健康数据未合并: failCount=%d lastError=%v", src.FetchFailCount, src.LastError)
			}
		}
	}

	// GetSource 单条查询不加载健康数据（非 DB 字段保持零值）
	single, err := s.GetSource(sources[1].ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if single.FetchFailCount != 0 {
		t.Errorf("GetSource 不应加载健康数据, failCount = %d", single.FetchFailCount)
	}
}

func TestUpdateSourcePartialFields(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()
	source := models.Source{Name: "原名", URL: "https://example.com/feed", ListRule: "rss", Active: true, Interval: 120}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	newName := "新名"
	if err := s.UpdateSource(source.ID, UpdateSourceRequest{Name: &newName, IsPrivate: boolPtr(true)}); err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}
	updated, err := s.GetSource(source.ID)
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if updated.Name != "新名" || !updated.IsPrivate {
		t.Fatalf("partial update failed: name=%q isPrivate=%v", updated.Name, updated.IsPrivate)
	}
	// 未更新的字段保持原值
	if updated.URL != source.URL || updated.Interval != 120 || !updated.Active {
		t.Fatalf("untouched fields changed: url=%q interval=%d active=%v", updated.URL, updated.Interval, updated.Active)
	}

	// 空更新请求应报错
	if err := s.UpdateSource(source.ID, UpdateSourceRequest{}); err == nil {
		t.Fatal("empty update should return error")
	}

	// URL 未变化时跳过格式校验（同 URL + 改名应成功）
	if err := s.UpdateSource(source.ID, UpdateSourceRequest{URL: strPtr(source.URL)}); err != nil {
		t.Fatalf("same-url update should skip validation: %v", err)
	}
}

func TestDeleteSourceCleansEmptyFolder(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	folder, err := s.CreateFolder("独占分组")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	source := models.Source{Name: "源", URL: "https://example.com/feed", ListRule: "rss", FolderID: &folder.ID, Active: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	keeper := models.Source{Name: "保留源", URL: "https://keep.example.com/feed", ListRule: "rss", Active: true}
	if err := db.Create(&keeper).Error; err != nil {
		t.Fatalf("create keeper: %v", err)
	}

	if err := s.DeleteSource(source.ID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	// 源删除后其独占的空文件夹被清理
	var folderCount int64
	if err := db.Model(&models.Folder{}).Where("id = ?", folder.ID).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folder: %v", err)
	}
	if folderCount != 0 {
		t.Fatal("empty folder should be cleaned after source deletion")
	}

	// 批量删除
	if err := s.DeleteSourcesBatch([]int{keeper.ID}); err != nil {
		t.Fatalf("DeleteSourcesBatch: %v", err)
	}
	sources, err := s.GetSources()
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources after batch delete = %d, want 0", len(sources))
	}
	// 空 ID 列表不报错
	if err := s.DeleteSourcesBatch(nil); err != nil {
		t.Fatalf("DeleteSourcesBatch(nil) should be no-op: %v", err)
	}
}

func TestCreateSourceValidation(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()
	existing := models.Source{Name: "已有", URL: "https://dup.example.com/feed", ListRule: "rss", Active: true}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing: %v", err)
	}

	// URL 为空直接拒绝（不触网）
	if _, err := s.CreateSource(CreateSourceRequest{URL: ""}); err == nil {
		t.Fatal("empty url should be rejected")
	}
	// 重复 URL 拒绝（不触网；名称非空跳过 feed 标题抓取）
	if _, err := s.CreateSource(CreateSourceRequest{Name: "重名", URL: "https://dup.example.com/feed"}); err == nil {
		t.Fatal("duplicate url should be rejected")
	}
}
