package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	miniflux "miniflux.app/v2/client"
)

const testToken = "test-api-token"

// newTestProvider 起本地 mock Miniflux 服务并构建 provider
func newTestProvider(t *testing.T, handler http.Handler) *MinifluxProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewMinifluxProvider(server.URL, testToken, server.Client())
}

// requireAuth 验证 API Key 认证头；缺失时返回 401
func requireAuth(r *http.Request) bool {
	return r.Header.Get("X-Auth-Token") == testToken
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

// TestMinifluxHealthCheck 验证认证头透传与请求路径
func TestMinifluxHealthCheck(t *testing.T) {
	var gotPath, gotToken string
	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error_message": "unauthorized"})
			return
		}
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Auth-Token")
		writeJSON(w, http.StatusOK, map[string]any{"id": 1, "username": "user"})
	}))

	if err := provider.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}
	if gotPath != "/v1/me" {
		t.Fatalf("expected path /v1/me, got %s", gotPath)
	}
	if gotToken != testToken {
		t.Fatalf("expected X-Auth-Token header, got %q", gotToken)
	}
}

// TestMinifluxHealthCheckUnauthorized 认证失败应返回 ErrNotAuthorized（可识别错误类型）
func TestMinifluxHealthCheckUnauthorized(t *testing.T) {
	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_message": "unauthorized"})
	}))

	err := provider.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !errors.Is(err, miniflux.ErrNotAuthorized) {
		t.Fatalf("expected ErrNotAuthorized, got %v", err)
	}
}

// TestMinifluxPullSubscriptions 验证 feeds + categories 到 RemoteFeed 的映射
func TestMinifluxPullSubscriptions(t *testing.T) {
	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/feeds" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, []map[string]any{
			{
				"id":       11,
				"feed_url": "https://example.com/a.xml",
				"title":    "Feed A",
				"category": map[string]any{"id": 2, "title": "Tech"},
			},
			{
				"id":       12,
				"feed_url": "https://example.com/b.xml",
				"title":    "Feed B",
			},
		})
	}))

	feeds, err := provider.PullSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("pull subscriptions: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(feeds))
	}
	if feeds[0].RemoteID != 11 || feeds[0].URL != "https://example.com/a.xml" || feeds[0].FolderName != "Tech" {
		t.Fatalf("feed[0] mapping mismatch: %+v", feeds[0])
	}
	if feeds[1].RemoteID != 12 || feeds[1].FolderName != "" {
		t.Fatalf("feed[1] mapping mismatch: %+v", feeds[1])
	}
}

// TestMinifluxPullItemsPaginationAndCursor 验证 offset 分页（满页续拉）、
// changed_after 增量游标透传与条目字段映射
func TestMinifluxPullItemsPaginationAndCursor(t *testing.T) {
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	var requestCount int
	var seenOffsets []string
	var seenChangedAfter []string

	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PullItems 先拉 feeds 建 feedID -> URL 映射
		if r.URL.Path == "/v1/feeds" {
			writeJSON(w, http.StatusOK, []map[string]any{
				{"id": 11, "feed_url": "https://example.com/a.xml", "title": "Feed A"},
			})
			return
		}
		if r.URL.Path != "/v1/entries" {
			http.NotFound(w, r)
			return
		}
		requestCount++
		query := r.URL.Query()
		seenOffsets = append(seenOffsets, query.Get("offset"))
		seenChangedAfter = append(seenChangedAfter, query.Get("changed_after"))

		offset, _ := strconv.Atoi(query.Get("offset"))
		entries := make([]map[string]any, 0, 100)
		// 第一页满 100 条触发续拉，第二页 1 条终止
		count := 1
		if offset == 0 {
			count = 100
		}
		for i := 0; i < count; i++ {
			id := int64(offset + i + 1)
			entries = append(entries, map[string]any{
				"id":           id,
				"feed_id":      11,
				"url":          fmt.Sprintf("https://example.com/article/%d", id),
				"title":        fmt.Sprintf("Article %d", id),
				"content":      "<p>hello</p>",
				"author":       "author",
				"status":       "read",
				"starred":      id == 1,
				"published_at": "2026-08-15T10:00:00Z",
				"changed_at":   "2026-08-16T10:00:00Z",
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"total": count, "entries": entries})
	}))

	items, err := provider.PullItems(context.Background(), since)
	if err != nil {
		t.Fatalf("pull items: %v", err)
	}
	if len(items) != 101 {
		t.Fatalf("expected 101 items (100 + 1), got %d", len(items))
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 paged requests, got %d", requestCount)
	}
	if len(seenOffsets) != 2 || seenOffsets[0] != "0" || seenOffsets[1] != "100" {
		t.Fatalf("offset pagination mismatch: %v", seenOffsets)
	}
	// 增量游标透传（Unix 秒）
	for _, got := range seenChangedAfter {
		if got != strconv.FormatInt(since.Unix(), 10) {
			t.Fatalf("changed_after cursor mismatch: %q", got)
		}
	}
	// 字段映射抽查：首条
	first := items[0]
	if first.RemoteID != 1 || first.SourceRemoteID != 11 || first.Link != "https://example.com/article/1" {
		t.Fatalf("item[0] mapping mismatch: %+v", first)
	}
	if !first.IsRead {
		t.Fatal("expected IsRead=true for status=read")
	}
	if !first.IsStarred {
		t.Fatal("expected IsStarred=true for id=1")
	}
	if !first.UpdatedAt.Equal(time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt mapping mismatch: %v", first.UpdatedAt)
	}
}

// TestMinifluxPushStates 验证状态批量推送的请求体格式
func TestMinifluxPushStates(t *testing.T) {
	var payloads []map[string]any
	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/entries" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		payloads = append(payloads, body)
		w.WriteHeader(http.StatusNoContent)
	}))

	err := provider.PushStates(context.Background(), []StateChange{
		{RemoteIDs: []int64{1, 2, 3}, Kind: StateChangeRead},
		{RemoteIDs: []int64{4}, Kind: StateChangeStar},
		{RemoteIDs: []int64{5}, Kind: StateChangeUnstar},
	})
	if err != nil {
		t.Fatalf("push states: %v", err)
	}
	if len(payloads) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(payloads))
	}
	if payloads[0]["status"] != "read" {
		t.Fatalf("read batch body mismatch: %v", payloads[0])
	}
	if payloads[1]["starred"] != true {
		t.Fatalf("star batch body mismatch: %v", payloads[1])
	}
	if payloads[2]["starred"] != false {
		t.Fatalf("unstar batch body mismatch: %v", payloads[2])
	}
	// entry_ids 批量序列化
	ids, ok := payloads[0]["entry_ids"].([]any)
	if !ok || len(ids) != 3 {
		t.Fatalf("read batch entry_ids mismatch: %v", payloads[0]["entry_ids"])
	}
}

// TestMinifluxPushSubscriptionUnsupported P0 阶段 rename/move 返回 ErrUnsupported
func TestMinifluxPushSubscriptionUnsupported(t *testing.T) {
	provider := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))

	for _, op := range []SubscriptionOpKind{OpRename, OpMove} {
		err := provider.PushSubscription(context.Background(), SubscriptionOp{Op: op, RemoteID: 1, Title: "x"})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("op %s: expected ErrUnsupported, got %v", op, err)
		}
	}
}
