package services

import (
	"testing"

	"github.com/rss/go-server/internal/models"
)

func TestFolderCRUD(t *testing.T) {
	s := newTestService(t)

	created, err := s.CreateFolder("技术")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if created.ID == 0 || created.Name != "技术" {
		t.Fatalf("unexpected folder: %+v", created)
	}

	if _, err := s.CreateFolder("生活"); err != nil {
		t.Fatalf("CreateFolder 2: %v", err)
	}
	folders, err := s.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("folders = %d, want 2", len(folders))
	}

	if err := s.UpdateFolder(created.ID, "科技"); err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	folders, err = s.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders after rename: %v", err)
	}
	if folders[0].Name != "科技" {
		t.Fatalf("renamed folder = %q, want 科技", folders[0].Name)
	}

	if err := s.DeleteFolder(created.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	folders, err = s.GetFolders()
	if err != nil {
		t.Fatalf("GetFolders after delete: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("folders after delete = %d, want 1", len(folders))
	}
}

func TestDeleteFolderKeepsSourcesAndChildren(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	parent, err := s.CreateFolder("父级")
	if err != nil {
		t.Fatalf("CreateFolder parent: %v", err)
	}
	child := models.Folder{Name: "子级", ParentID: &parent.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child folder: %v", err)
	}
	source := models.Source{Name: "源", URL: "https://example.com/feed", ListRule: "rss", FolderID: &parent.ID, Active: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	if err := s.DeleteFolder(parent.ID); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}

	// 源被移出到根目录（不删除）
	var moved models.Source
	if err := db.First(&moved, source.ID).Error; err != nil {
		t.Fatalf("source should survive: %v", err)
	}
	if moved.FolderID != nil {
		t.Fatalf("source folderId = %v, want nil", *moved.FolderID)
	}
	// 子文件夹被提升到根层级
	var promoted models.Folder
	if err := db.First(&promoted, child.ID).Error; err != nil {
		t.Fatalf("child folder should survive: %v", err)
	}
	if promoted.ParentID != nil {
		t.Fatalf("child parentId = %v, want nil", *promoted.ParentID)
	}
}

func TestUpdateSourceMovesSourceWithoutCleaningFolder(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	from, err := s.CreateFolder("原文件夹")
	if err != nil {
		t.Fatalf("CreateFolder from: %v", err)
	}
	to, err := s.CreateFolder("目标文件夹")
	if err != nil {
		t.Fatalf("CreateFolder to: %v", err)
	}
	source := models.Source{Name: "源", URL: "https://example.com/feed", ListRule: "rss", FolderID: &from.ID, Active: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	// 前端实际使用的移动路径是 PUT /sources/:id（UpdateSource），而非专用的 MoveSourceFolder。
	toID := to.ID
	if err := s.UpdateSource(source.ID, UpdateSourceRequest{FolderID: &toID}); err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}
	var moved models.Source
	if err := db.First(&moved, source.ID).Error; err != nil {
		t.Fatalf("load source: %v", err)
	}
	if moved.FolderID == nil || *moved.FolderID != to.ID {
		t.Fatalf("source folderId = %v, want %d", moved.FolderID, to.ID)
	}
}

func TestClearFolder(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()

	folder, err := s.CreateFolder("待清空")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	source := models.Source{Name: "源", URL: "https://example.com/feed", ListRule: "rss", FolderID: &folder.ID, Active: true}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	if err := s.ClearFolder(folder.ID); err != nil {
		t.Fatalf("ClearFolder: %v", err)
	}

	// 源保留但移到根目录，空文件夹被删除
	var kept models.Source
	if err := db.First(&kept, source.ID).Error; err != nil {
		t.Fatalf("source should survive clear: %v", err)
	}
	if kept.FolderID != nil {
		t.Fatalf("source folderId = %v, want nil", *kept.FolderID)
	}
	var folderCount int64
	if err := db.Model(&models.Folder{}).Where("id = ?", folder.ID).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folder: %v", err)
	}
	if folderCount != 0 {
		t.Fatal("cleared folder should be deleted")
	}
}
