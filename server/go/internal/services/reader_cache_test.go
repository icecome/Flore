package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
)

func newCacheTestService(t *testing.T) *ReaderService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&models.Item{}); err != nil {
		t.Fatalf("automigrate Item: %v", err)
	}
	return &ReaderService{db: db}
}

// TestUnreadCountCache_Lifecycle 验证 P2-1/B 缓存机制：首次重算、命中、整体失效后重算。
func TestUnreadCountCache_Lifecycle(t *testing.T) {
	s := newCacheTestService(t)

	// 首次：未就绪 → compute（空库 → 空 map），并填充缓存
	m1, err := s.loadUnreadCountMap()
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if len(m1) != 0 {
		t.Fatalf("expected empty map, got %v", m1)
	}

	// 第二次：命中缓存（ready=true），返回副本且内容一致
	m2, err := s.loadUnreadCountMap()
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(m2) != 0 {
		t.Fatalf("expected cache hit empty, got %v", m2)
	}

	// 失效：ready 复位、cache 清空
	s.invalidateUnreadCount()
	s.unreadCountMu.RLock()
	if s.unreadCountReady {
		t.Fatal("expected unreadCountReady=false after invalidate")
	}
	s.unreadCountMu.RUnlock()

	// 失效后再次访问：重新 compute（无 panic），仍正确返回空 map
	m3, err := s.loadUnreadCountMap()
	if err != nil {
		t.Fatalf("post-invalidate load: %v", err)
	}
	if len(m3) != 0 {
		t.Fatalf("expected empty after recompute, got %v", m3)
	}
}
