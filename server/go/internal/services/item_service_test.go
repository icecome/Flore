package services

import (
	"testing"
	"time"

	"github.com/rss/go-server/internal/models"
)

// seedItems 构建两个源（一个普通、一个私密）各两篇文章，返回 (普通源, 私密源)
func seedItems(t *testing.T, s *ReaderService) (models.Source, models.Source) {
	t.Helper()
	db := s.getDb()
	normal := models.Source{Name: "普通源", URL: "https://normal.example.com/feed", ListRule: "rss", Active: true}
	private := models.Source{Name: "私密源", URL: "https://private.example.com/feed", ListRule: "rss", Active: true, IsPrivate: true}
	if err := db.Create(&normal).Error; err != nil {
		t.Fatalf("create normal source: %v", err)
	}
	if err := db.Create(&private).Error; err != nil {
		t.Fatalf("create private source: %v", err)
	}
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	items := []models.Item{
		{SourceID: normal.ID, Title: "n1", Link: "https://normal.example.com/1", PubDate: models.NullableMilliTime{T: &base}},
		{SourceID: normal.ID, Title: "n2", Link: "https://normal.example.com/2", PubDate: models.NullableMilliTime{T: ptrTime(base.Add(1 * time.Hour))}, IsRead: true},
		{SourceID: private.ID, Title: "p1", Link: "https://private.example.com/1", PubDate: models.NullableMilliTime{T: ptrTime(base.Add(2 * time.Hour))}},
		{SourceID: private.ID, Title: "p2", Link: "https://private.example.com/2", PubDate: models.NullableMilliTime{T: ptrTime(base.Add(3 * time.Hour))}, IsStarred: true},
	}
	for i := range items {
		if err := db.Create(&items[i]).Error; err != nil {
			t.Fatalf("create item %d: %v", i, err)
		}
	}
	return normal, private
}

func TestGetItemsFilters(t *testing.T) {
	s := newTestService(t)
	normal, _ := seedItems(t, s)

	cases := []struct {
		name        string
		sourceID    *int
		onlyUnread  bool
		onlyStarred bool
		hidePrivate bool
		wantTitles  []string
	}{
		{name: "全部文章（默认）", wantTitles: []string{"n1", "n2", "p1", "p2"}},
		{name: "仅未读", onlyUnread: true, wantTitles: []string{"n1", "p1", "p2"}},
		{name: "仅收藏", onlyStarred: true, wantTitles: []string{"p2"}},
		{name: "按源筛选", sourceID: &normal.ID, wantTitles: []string{"n1", "n2"}},
		{name: "隐藏私密源", hidePrivate: true, wantTitles: []string{"n1", "n2"}},
		{name: "隐藏私密 + 仅未读", hidePrivate: true, onlyUnread: true, wantTitles: []string{"n1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := s.GetItems(tc.sourceID, nil, tc.onlyUnread, tc.onlyStarred, false, tc.hidePrivate, 0, 0, "newest")
			if err != nil {
				t.Fatalf("GetItems: %v", err)
			}
			got := make([]string, len(items))
			for i, it := range items {
				got[i] = it.Title
			}
			if len(got) != len(tc.wantTitles) {
				t.Fatalf("got %v, want %v", got, tc.wantTitles)
			}
			// newest 排序：pubDate 降序
			for i := range got {
				if got[i] != tc.wantTitles[len(tc.wantTitles)-1-i] {
					t.Fatalf("got %v, want %v (desc)", got, tc.wantTitles)
				}
			}
		})
	}
}

func TestGetItemsOrderAndPaging(t *testing.T) {
	s := newTestService(t)
	normal, _ := seedItems(t, s)

	oldest, err := s.GetItems(&normal.ID, nil, false, false, false, false, 0, 0, "oldest")
	if err != nil {
		t.Fatalf("GetItems oldest: %v", err)
	}
	if oldest[0].Title != "n1" || oldest[1].Title != "n2" {
		t.Fatalf("oldest order = [%s %s], want [n1 n2]", oldest[0].Title, oldest[1].Title)
	}

	newest, err := s.GetItems(&normal.ID, nil, false, false, false, false, 0, 0, "newest")
	if err != nil {
		t.Fatalf("GetItems newest: %v", err)
	}
	if newest[0].Title != "n2" || newest[1].Title != "n1" {
		t.Fatalf("newest order = [%s %s], want [n2 n1]", newest[0].Title, newest[1].Title)
	}

	// limit 默认 50、offset 分页
	page1, err := s.GetItems(&normal.ID, nil, false, false, false, false, 1, 0, "newest")
	if err != nil {
		t.Fatalf("GetItems page1: %v", err)
	}
	page2, err := s.GetItems(&normal.ID, nil, false, false, false, false, 1, 1, "newest")
	if err != nil {
		t.Fatalf("GetItems page2: %v", err)
	}
	if len(page1) != 1 || len(page2) != 1 || page1[0].Title == page2[0].Title {
		t.Fatalf("paging broken: page1=%v page2=%v", page1, page2)
	}
}

func TestMarkStates(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()
	normal, _ := seedItems(t, s)

	var item models.Item
	if err := db.Where("title = ?", "n1").First(&item).Error; err != nil {
		t.Fatalf("load n1: %v", err)
	}
	if item.SourceID != normal.ID {
		t.Fatalf("n1 belongs to source %d, want normal source %d", item.SourceID, normal.ID)
	}

	if err := s.MarkRead(item.ID, true); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := s.MarkStarred(item.ID, true); err != nil {
		t.Fatalf("MarkStarred: %v", err)
	}
	if err := s.MarkReadLater(item.ID, true); err != nil {
		t.Fatalf("MarkReadLater: %v", err)
	}
	updated, err := s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if !updated.IsRead || !updated.IsStarred || !updated.IsReadLater {
		t.Fatalf("states not applied: read=%v star=%v later=%v", updated.IsRead, updated.IsStarred, updated.IsReadLater)
	}

	// 回退未读
	if err := s.MarkRead(item.ID, false); err != nil {
		t.Fatalf("MarkRead undo: %v", err)
	}
	updated, err = s.GetItem(item.ID)
	if err != nil {
		t.Fatalf("GetItem after undo: %v", err)
	}
	if updated.IsRead {
		t.Fatal("item should be unread after undo")
	}

	// 批量标记：把普通源两篇都标已读，仅剩私密源两篇未读
	if err := s.MarkReadBatch([]int{1, 2}, true); err != nil {
		t.Fatalf("MarkReadBatch: %v", err)
	}
	count, err := s.GetUnreadCount(false)
	if err != nil {
		t.Fatalf("GetUnreadCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("unread after batch = %d, want 2", count)
	}
}

func TestGetUnreadCount(t *testing.T) {
	s := newTestService(t)
	seedItems(t, s)

	count, err := s.GetUnreadCount(false)
	if err != nil {
		t.Fatalf("GetUnreadCount: %v", err)
	}
	if count != 3 { // n1 + p1 + p2
		t.Fatalf("unread count = %d, want 3", count)
	}
	privateCount, err := s.GetUnreadCount(true)
	if err != nil {
		t.Fatalf("GetUnreadCount(hidePrivate): %v", err)
	}
	if privateCount != 1 { // 仅 n1
		t.Fatalf("unread count (hidePrivate) = %d, want 1", privateCount)
	}
}

func TestClearReadLater(t *testing.T) {
	s := newTestService(t)
	db := s.getDb()
	normal, _ := seedItems(t, s)

	if err := s.MarkReadLater(1, true); err != nil {
		t.Fatalf("MarkReadLater: %v", err)
	}
	if err := s.MarkReadLater(3, true); err != nil {
		t.Fatalf("MarkReadLater: %v", err)
	}

	// 仅清空普通源范围
	if err := s.ClearReadLater(&normal.ID, nil); err != nil {
		t.Fatalf("ClearReadLater: %v", err)
	}
	var n1, p1 models.Item
	if err := db.First(&n1, 1).Error; err != nil {
		t.Fatalf("load n1: %v", err)
	}
	if err := db.First(&p1, 3).Error; err != nil {
		t.Fatalf("load p1: %v", err)
	}
	if n1.IsReadLater {
		t.Error("n1 readLater should be cleared")
	}
	if !p1.IsReadLater {
		t.Error("p1 readLater should be kept")
	}
}
