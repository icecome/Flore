package sync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testUser = "alice"
	testPass = "api-password"
)

// greaderRequest 记录一次请求（路径 + 表单 + 查询串）
type greaderRequest struct {
	Path     string
	Form     url.Values
	RawQuery string
}

// fakeGReaderServer 内存版 GReader/FreshRSS 服务
type fakeGReaderServer struct {
	mu sync.Mutex

	loginCount       int
	validToken       string
	revokeFirst      bool // 首个 token 用一次即失效（触发客户端重登录）
	firstRevoked     bool
	subscriptionJSON string
	streamPages      map[string]string // continuation -> 响应；"" 为首页

	requests []greaderRequest
}

func newFakeGReader() *fakeGReaderServer {
	return &fakeGReaderServer{
		subscriptionJSON: `{"subscriptions":[
			{"id":"feed/11","title":"Feed A","url":"https://a.com/feed.xml","categories":[{"id":"user/-/label/Tech","label":"Tech"}]},
			{"id":"feed/12","title":"Feed B","url":"https://b.com/feed.xml"}
		]}`,
		streamPages: map[string]string{
			"": `{"items":[
				{"id":"tag:google.com,2005:reader/item/00000000000003e9","categories":["user/-/state/com.google/reading-list","user/-/state/com.google/read"],"title":"Post 1","published":1690000000,"updated":1690000100,"canonical":[{"href":"https://a.com/1"}],"summary":{"content":"<p>hello</p>"},"author":"Alice","origin":{"streamId":"feed/11"}}
			],"continuation":"page2"}`,
			"page2": `{"items":[
				{"id":"tag:google.com,2005:reader/item/00000000000007d2","categories":["user/-/state/com.google/reading-list","user/-/state/com.google/starred"],"title":"Post 2","published":1690000200,"updated":1690000300,"canonical":[{"href":"https://b.com/2"}],"summary":{"content":"body2"},"origin":{"streamId":"feed/12"}}
			]}`,
		},
	}
}

func (f *fakeGReaderServer) record(r *http.Request) greaderRequest {
	req := greaderRequest{Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err == nil {
			req.Form = r.PostForm
		}
	}
	f.requests = append(f.requests, req)
	return req
}

func (f *fakeGReaderServer) findRequests(pathPart string) []greaderRequest {
	var out []greaderRequest
	for _, req := range f.requests {
		if strings.Contains(req.Path, pathPart) {
			out = append(out, req)
		}
	}
	return out
}

func (f *fakeGReaderServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		req := f.record(r)

		if strings.HasSuffix(r.URL.Path, "/accounts/ClientLogin") {
			f.loginCount++
			if req.Form.Get("Email") != testUser || req.Form.Get("Passwd") != testPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			f.validToken = fmt.Sprintf("token-%d", f.loginCount)
			_, _ = w.Write([]byte("SID=SID\nLSID=LSID\nAuth=" + f.validToken + "\n"))
			return
		}

		if r.Header.Get("Authorization") != "GoogleLogin auth="+f.validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// 首个 token 失效一次：客户端应重登录并重试成功
		if f.revokeFirst && !f.firstRevoked && f.loginCount == 1 {
			f.firstRevoked = true
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/user-info"):
			writeJSON(w, http.StatusOK, map[string]any{"userId": "1", "userName": testUser})
		case strings.HasSuffix(r.URL.Path, "/subscription/list"):
			_, _ = w.Write([]byte(f.subscriptionJSON))
		case strings.HasSuffix(r.URL.Path, "/subscription/quickadd"):
			writeJSON(w, http.StatusOK, map[string]any{"numResults": 1, "streamId": "feed/99"})
		case strings.HasSuffix(r.URL.Path, "/subscription/edit"):
			_, _ = w.Write([]byte("OK"))
		case strings.Contains(r.URL.Path, "/stream/contents/"):
			page, ok := f.streamPages[r.URL.Query().Get("c")]
			if !ok {
				page = `{"items":[]}`
			}
			_, _ = w.Write([]byte(page))
		case strings.HasSuffix(r.URL.Path, "/edit-tag"):
			_, _ = w.Write([]byte("OK"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// newGReaderTest 起 mock 服务并构建 provider（endpoint 填根地址，验证自动拼接）
func newGReaderTest(t *testing.T, fake *fakeGReaderServer, password string) *GReaderProvider {
	t.Helper()
	server := httptest.NewServer(fake.handler())
	t.Cleanup(server.Close)
	return NewGReaderProvider(server.URL, testUser, password, server.Client())
}

// TestGReaderEndpointNormalization endpoint 归一化
func TestGReaderEndpointNormalization(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://x.com", "https://x.com/api/greader.php"},
		{"https://x.com/", "https://x.com/api/greader.php"},
		{"https://x.com/api", "https://x.com/api/greader.php"},
		{"https://x.com/api/greader.php", "https://x.com/api/greader.php"},
	}
	for _, c := range cases {
		if got := NewGReaderProvider(c.in, "u", "p", nil).apiBase; got != c.want {
			t.Errorf("endpoint %q: got %q, want %q", c.in, got, c.want)
		}
	}
}

// TestGReaderHealthCheck 登录 + token 头透传
func TestGReaderHealthCheck(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	if err := provider.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}
	fake.mu.Lock()
	loginCount := fake.loginCount
	fake.mu.Unlock()
	if loginCount != 1 {
		t.Fatalf("expected 1 login, got %d", loginCount)
	}

	logins := fake.findRequests("/accounts/ClientLogin")
	if len(logins) != 1 {
		t.Fatalf("expected 1 ClientLogin request, got %d", len(logins))
	}
	if got := logins[0].Form.Get("Email"); got != testUser {
		t.Errorf("Email form: got %q", got)
	}
	if got := logins[0].Form.Get("Passwd"); got != testPass {
		t.Errorf("Passwd form: got %q", got)
	}
}

// TestGReaderLoginRejected 密码错误应返回 ErrNotAuthorized
func TestGReaderLoginRejected(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, "wrong-pass")

	err := provider.HealthCheck(context.Background())
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("expected ErrNotAuthorized, got %v", err)
	}
}

// TestGReader401ReloginRetry token 失效后应重登录并重试成功
func TestGReader401ReloginRetry(t *testing.T) {
	fake := newFakeGReader()
	fake.revokeFirst = true
	provider := newGReaderTest(t, fake, testPass)

	if err := provider.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check after relogin: %v", err)
	}
	fake.mu.Lock()
	loginCount := fake.loginCount
	fake.mu.Unlock()
	if loginCount != 2 {
		t.Fatalf("expected 2 logins (relogin), got %d", loginCount)
	}
}

// TestGReaderPullSubscriptions 订阅映射（ID/URL/标题/文件夹）
func TestGReaderPullSubscriptions(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	feeds, err := provider.PullSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("pull subscriptions: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(feeds))
	}
	if feeds[0].RemoteID != 11 || feeds[0].URL != "https://a.com/feed.xml" || feeds[0].Title != "Feed A" || feeds[0].FolderName != "Tech" {
		t.Errorf("feed[0] mapping wrong: %+v", feeds[0])
	}
	if feeds[1].RemoteID != 12 || feeds[1].FolderName != "" {
		t.Errorf("feed[1] mapping wrong: %+v", feeds[1])
	}
}

// TestGReaderPullItemsPagination 分页（continuation）+ 字段映射 + ot 游标
func TestGReaderPullItemsPagination(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	since := time.Unix(1690000000, 0)
	items, err := provider.PullItems(context.Background(), since)
	if err != nil {
		t.Fatalf("pull items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items across pages, got %d", len(items))
	}

	first := items[0]
	if first.RemoteID != 1001 || first.SourceRemoteID != 11 || first.SourceURL != "https://a.com/feed.xml" {
		t.Errorf("item[0] ids wrong: %+v", first)
	}
	if first.Link != "https://a.com/1" || first.Title != "Post 1" || first.Content != "<p>hello</p>" || first.Author != "Alice" {
		t.Errorf("item[0] content wrong: %+v", first)
	}
	if !first.IsRead || first.IsStarred {
		t.Errorf("item[0] states wrong: read=%v starred=%v", first.IsRead, first.IsStarred)
	}
	if !first.PubDate.Equal(time.Unix(1690000000, 0)) {
		t.Errorf("item[0] pubDate wrong: %v", first.PubDate)
	}

	second := items[1]
	if second.RemoteID != 2002 || second.SourceURL != "https://b.com/feed.xml" {
		t.Errorf("item[1] ids wrong: %+v", second)
	}
	if second.IsRead || !second.IsStarred {
		t.Errorf("item[1] states wrong: read=%v starred=%v", second.IsRead, second.IsStarred)
	}

	// 恰好两次流请求；首页带 ot，续页带 c
	streams := fake.findRequests("/stream/contents/")
	if len(streams) != 2 {
		t.Fatalf("expected 2 stream requests, got %d", len(streams))
	}
	if !strings.Contains(streams[0].RawQuery, "ot=1690000000") {
		t.Errorf("first page missing ot cursor: %q", streams[0].RawQuery)
	}
	if !strings.Contains(streams[1].RawQuery, "c=page2") {
		t.Errorf("second page missing continuation: %q", streams[1].RawQuery)
	}
}

// TestGReaderPushStates edit-tag 表单体（a= 添加 / r= 移除 / i= 批量）
func TestGReaderPushStates(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	states := []StateChange{
		{Kind: StateChangeRead, RemoteIDs: []int64{1001, 1002}},
		{Kind: StateChangeUnstar, RemoteIDs: []int64{2002}},
	}
	if err := provider.PushStates(context.Background(), states); err != nil {
		t.Fatalf("push states: %v", err)
	}

	edits := fake.findRequests("/edit-tag")
	if len(edits) != 2 {
		t.Fatalf("expected 2 edit-tag requests, got %d", len(edits))
	}
	if got := edits[0].Form.Get("a"); got != greaderStreamRead {
		t.Errorf("batch[0] a: got %q", got)
	}
	if ids := edits[0].Form["i"]; len(ids) != 2 || ids[0] != "1001" || ids[1] != "1002" {
		t.Errorf("batch[0] i: got %v", ids)
	}
	if got := edits[1].Form.Get("r"); got != greaderStreamStarred {
		t.Errorf("batch[1] r: got %q", got)
	}
	if ids := edits[1].Form["i"]; len(ids) != 1 || ids[0] != "2002" {
		t.Errorf("batch[1] i: got %v", ids)
	}
}

// TestGReaderSubscribe 订阅推送：quickadd + edit（标题与文件夹）
func TestGReaderSubscribe(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	op := SubscriptionOp{
		Op:         OpSubscribe,
		URL:        "https://new.com/feed.xml",
		Title:      "New",
		FolderName: "Tech",
	}
	if err := provider.PushSubscription(context.Background(), op); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	quick := fake.findRequests("/subscription/quickadd")
	if len(quick) != 1 {
		t.Fatalf("expected 1 quickadd request, got %d", len(quick))
	}
	if got := quick[0].Form.Get("quickadd"); got != "feed/https://new.com/feed.xml" {
		t.Errorf("quickadd form: got %q", got)
	}

	edits := fake.findRequests("/subscription/edit")
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit request, got %d", len(edits))
	}
	edit := edits[0].Form
	if edit.Get("ac") != "subscribe" || edit.Get("s") != "feed/https://new.com/feed.xml" {
		t.Errorf("edit ac/s wrong: %v", edit)
	}
	if edit.Get("t") != "New" || edit.Get("a") != "user/-/label/Tech" {
		t.Errorf("edit title/folder wrong: %v", edit)
	}
}

// TestGReaderUnsubscribe 退订：优先使用数字流 ID
func TestGReaderUnsubscribe(t *testing.T) {
	fake := newFakeGReader()
	provider := newGReaderTest(t, fake, testPass)

	op := SubscriptionOp{Op: OpUnsubscribe, RemoteID: 11, URL: "https://a.com/feed.xml"}
	if err := provider.PushSubscription(context.Background(), op); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	edits := fake.findRequests("/subscription/edit")
	if len(edits) != 1 {
		t.Fatalf("expected 1 edit request, got %d", len(edits))
	}
	if got := edits[0].Form.Get("s"); got != "feed/11" {
		t.Errorf("expected feed/11, got %q", got)
	}
}
