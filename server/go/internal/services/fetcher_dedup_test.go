package services

import (
	"testing"
	"time"
)

func TestFilterValidItems_Dedup(t *testing.T) {
	now := time.Now()
	items := []FeedItem{
		{Title: "A", Link: "https://example.com/a", PubDate: now},
		{Title: "A", Link: "https://example.com/a", PubDate: now},
		{Title: "B", Link: "https://example.com/b", PubDate: now},
		{Title: "C", Link: "https://example.com/c", PubDate: now},
		{Title: "C", Link: "https://example.com/c", PubDate: now},
		{Title: "D", Link: "", PubDate: now},
	}
	result := filterValidItems(items)
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	links := make(map[string]bool)
	for _, it := range result {
		if it.Link == "" {
			t.Error("empty link should be filtered out")
		}
		if links[it.Link] {
			t.Errorf("duplicate link: %s", it.Link)
		}
		links[it.Link] = true
	}
}
