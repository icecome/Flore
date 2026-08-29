package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotAuthorized 认证失败（登录被拒，或 token 失效后重登录仍失败）
var ErrNotAuthorized = errors.New("not authorized")

const (
	// greaderItemIDPrefix GReader 条目长式 ID 前缀
	greaderItemIDPrefix = "tag:google.com,2005:reader/item/"
	// greaderStreamRead / greaderStreamStarred GReader 读写状态流标识
	greaderStreamRead    = "user/-/state/com.google/read"
	greaderStreamStarred = "user/-/state/com.google/starred"
	// greaderReadingList 全部条目流（拉取入口）
	greaderReadingList = "/reader/api/0/stream/contents/user/-/state/com.google/reading-list"
)

// greaderSubscriptionList subscription/list 响应
type greaderSubscriptionList struct {
	Subscriptions []greaderSubscription `json:"subscriptions"`
}

// greaderSubscription 单条订阅（id 形如 feed/123）
type greaderSubscription struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	URL        string               `json:"url"`
	Categories []greaderCategoryRef `json:"categories"`
}

// greaderCategoryRef 订阅归属的分类（label 即文件夹名）
type greaderCategoryRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// greaderStream stream/contents 响应
type greaderStream struct {
	Items        []greaderStreamItem `json:"items"`
	Continuation string              `json:"continuation"`
}

// greaderStreamItem 单条目（id 长式 tag:google.com,2005:reader/item/<num>）
type greaderStreamItem struct {
	ID         string         `json:"id"`
	Categories []string       `json:"categories"`
	Title      string         `json:"title"`
	Published  int64          `json:"published"`
	Updated    int64          `json:"updated"`
	Canonical  []greaderLink  `json:"canonical"`
	Alternate  []greaderLink  `json:"alternate"`
	Summary    greaderContent `json:"summary"`
	Author     string         `json:"author"`
	Origin     greaderOrigin  `json:"origin"`
}

// greaderLink 条目正文链接
type greaderLink struct {
	Href string `json:"href"`
}

// greaderContent 条目摘要（正文 HTML）
type greaderContent struct {
	Content string `json:"content"`
}

// greaderOrigin 条目来源（streamId 形如 feed/123）
type greaderOrigin struct {
	StreamID string `json:"streamId"`
}

// GReaderProvider 基于 GReader 兼容协议的同步后端（FreshRSS / TTRSS freshapi 插件）
type GReaderProvider struct {
	apiBase    string
	username   string
	password   string
	httpClient *http.Client

	tokenMu sync.Mutex
	token   string

	// subsMu/subsCache 订阅列表实例内缓存：引擎每轮同步复用同一 provider，
	// PullSubscriptions 与 PullItems 共享，避免一轮同步拉取两次列表（M-R4）
	subsMu    sync.Mutex
	subsCache []greaderSubscription
}

// 编译期接口断言
var _ RemoteProvider = (*GReaderProvider)(nil)

// NewGReaderProvider 创建 GReader 提供器。
// endpoint 兼容多种填法：服务器根地址、…/api 或完整 greader.php 路径，内部统一归一化为
// FreshRSS 的 {root}/api/greader.php；httpClient 由调用方构建，复用代理策略。
func NewGReaderProvider(endpoint, username, password string, httpClient *http.Client) *GReaderProvider {
	base := strings.TrimSuffix(endpoint, "/")
	switch {
	case strings.HasSuffix(base, "greader.php"):
	case strings.HasSuffix(base, "/api"):
		base += "/greader.php"
	default:
		base += "/api/greader.php"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &GReaderProvider{
		apiBase:    base,
		username:   username,
		password:   password,
		httpClient: httpClient,
	}
}

// Name 提供器名称
func (p *GReaderProvider) Name() string {
	return ProviderGReader
}

// PullSubscriptions 拉取远端订阅源列表（含文件夹归类）
func (p *GReaderProvider) PullSubscriptions(ctx context.Context) ([]RemoteFeed, error) {
	subs, err := p.listSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RemoteFeed, 0, len(subs))
	for _, sub := range subs {
		folder := ""
		if len(sub.Categories) > 0 {
			folder = sub.Categories[0].Label
		}
		result = append(result, RemoteFeed{
			RemoteID:   parseGReaderStreamID(sub.ID),
			URL:        sub.URL,
			Title:      sub.Title,
			FolderName: folder,
		})
	}
	return result, nil
}

// listSubscriptions 拉取原始订阅列表（保留 streamId 形式的 ID），实例内缓存复用
func (p *GReaderProvider) listSubscriptions(ctx context.Context) ([]greaderSubscription, error) {
	p.subsMu.Lock()
	if p.subsCache != nil {
		subs := p.subsCache
		p.subsMu.Unlock()
		return subs, nil
	}
	p.subsMu.Unlock()

	body, err := p.do(ctx, http.MethodGet, "/reader/api/0/subscription/list?output=json", nil)
	if err != nil {
		return nil, err
	}
	var list greaderSubscriptionList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("greader: parse subscriptions failed: %w", err)
	}
	p.subsMu.Lock()
	p.subsCache = list.Subscriptions
	p.subsMu.Unlock()
	return list.Subscriptions, nil
}

// PushSubscription 推送订阅变更（subscribe/unsubscribe/rename/move）
func (p *GReaderProvider) PushSubscription(ctx context.Context, op SubscriptionOp) error {
	switch op.Op {
	case OpSubscribe:
		// 先 quickadd 完成订阅，再按需补标题与文件夹
		form := url.Values{"quickadd": {"feed/" + op.URL}}
		body, err := p.do(ctx, http.MethodPost, "/reader/api/0/subscription/quickadd", form)
		if err != nil {
			return fmt.Errorf("greader: subscribe feed failed: %w", err)
		}
		var quick struct {
			NumResults int `json:"numResults"`
		}
		if err := json.Unmarshal(body, &quick); err != nil {
			return fmt.Errorf("greader: parse quickadd response failed: %w", err)
		}
		if quick.NumResults < 1 {
			return fmt.Errorf("greader: subscribe feed rejected by remote")
		}
		if op.Title == "" && op.FolderName == "" {
			return nil
		}
		edit := url.Values{"ac": {"subscribe"}, "s": {"feed/" + op.URL}}
		if op.Title != "" {
			edit.Set("t", op.Title)
		}
		if op.FolderName != "" {
			edit.Set("a", "user/-/label/"+op.FolderName)
		}
		if _, err := p.do(ctx, http.MethodPost, "/reader/api/0/subscription/edit", edit); err != nil {
			return fmt.Errorf("greader: set subscription meta failed: %w", err)
		}
		return nil
	case OpUnsubscribe:
		form := url.Values{"ac": {"unsubscribe"}, "s": {greaderFeedStream(op)}}
		if _, err := p.do(ctx, http.MethodPost, "/reader/api/0/subscription/edit", form); err != nil {
			return fmt.Errorf("greader: unsubscribe feed failed: %w", err)
		}
		return nil
	case OpRename:
		if op.Title == "" {
			return fmt.Errorf("greader: rename requires title")
		}
		form := url.Values{"ac": {"edit"}, "s": {greaderFeedStream(op)}, "t": {op.Title}}
		if _, err := p.do(ctx, http.MethodPost, "/reader/api/0/subscription/edit", form); err != nil {
			return fmt.Errorf("greader: rename feed failed: %w", err)
		}
		return nil
	case OpMove:
		if op.FolderName == "" {
			return fmt.Errorf("greader: move requires folder")
		}
		form := url.Values{"ac": {"edit"}, "s": {greaderFeedStream(op)}, "a": {"user/-/label/" + op.FolderName}}
		if _, err := p.do(ctx, http.MethodPost, "/reader/api/0/subscription/edit", form); err != nil {
			return fmt.Errorf("greader: move feed failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("greader: unknown subscription op: %s", op.Op)
	}
}

// PullItems 拉取自 since 起的条目（ot 时间起点 + continuation 分页）。
// 条目流不直接携带 feed XML 地址，先经 subscription/list 建立 streamId -> URL 映射。
func (p *GReaderProvider) PullItems(ctx context.Context, since time.Time) ([]RemoteItem, error) {
	subs, err := p.listSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	feedURLs := make(map[string]string, len(subs))
	for _, sub := range subs {
		feedURLs[sub.ID] = sub.URL
	}

	var result []RemoteItem
	continuation := ""
	for {
		query := url.Values{"output": {"json"}, "n": {"1000"}}
		if !since.IsZero() {
			query.Set("ot", strconv.FormatInt(since.Unix(), 10))
		}
		if continuation != "" {
			query.Set("c", continuation)
		}
		body, err := p.do(ctx, http.MethodGet, greaderReadingList+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var stream greaderStream
		if err := json.Unmarshal(body, &stream); err != nil {
			return nil, fmt.Errorf("greader: parse stream failed: %w", err)
		}
		for _, item := range stream.Items {
			link := ""
			if len(item.Canonical) > 0 {
				link = item.Canonical[0].Href
			} else if len(item.Alternate) > 0 {
				link = item.Alternate[0].Href
			}
			isRead, isStarred := false, false
			for _, category := range item.Categories {
				switch category {
				case greaderStreamRead:
					isRead = true
				case greaderStreamStarred:
					isStarred = true
				}
			}
			result = append(result, RemoteItem{
				RemoteID:       parseGReaderItemID(item.ID),
				SourceRemoteID: parseGReaderStreamID(item.Origin.StreamID),
				SourceURL:      feedURLs[item.Origin.StreamID],
				Link:           link,
				Title:          item.Title,
				Content:        item.Summary.Content,
				Author:         item.Author,
				PubDate:        greaderUnixTime(item.Published),
				IsRead:         isRead,
				IsStarred:      isStarred,
				UpdatedAt:      greaderUnixTime(item.Updated),
			})
		}
		if stream.Continuation == "" {
			break
		}
		continuation = stream.Continuation
	}
	return result, nil
}

// PushStates 推送状态变更（a= 添加状态，r= 移除状态，i= 条目批量）
func (p *GReaderProvider) PushStates(ctx context.Context, states []StateChange) error {
	for _, state := range states {
		if len(state.RemoteIDs) == 0 {
			continue
		}
		form := url.Values{}
		for _, id := range state.RemoteIDs {
			form.Add("i", strconv.FormatInt(id, 10))
		}
		switch state.Kind {
		case StateChangeRead:
			form.Set("a", greaderStreamRead)
		case StateChangeUnread:
			form.Set("r", greaderStreamRead)
		case StateChangeStar:
			form.Set("a", greaderStreamStarred)
		case StateChangeUnstar:
			form.Set("r", greaderStreamStarred)
		default:
			return fmt.Errorf("greader: unknown state change kind: %s", state.Kind)
		}
		if _, err := p.do(ctx, http.MethodPost, "/reader/api/0/edit-tag", form); err != nil {
			return fmt.Errorf("greader: push states failed: %w", err)
		}
	}
	return nil
}

// HealthCheck 检测可达性与认证有效性（user-info 且 userId 非空）
func (p *GReaderProvider) HealthCheck(ctx context.Context) error {
	body, err := p.do(ctx, http.MethodGet, "/reader/api/0/user-info?output=json", nil)
	if err != nil {
		return err
	}
	var info struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return fmt.Errorf("greader: parse user-info failed: %w", err)
	}
	if info.UserID == "" {
		return errors.New("greader: user-info missing userId")
	}
	return nil
}

// do 执行带认证的请求；token 失效（401）时重登录并重试一次
func (p *GReaderProvider) do(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		token, err := p.ensureToken()
		if err != nil {
			return nil, err
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, p.apiBase+path, body)
		if err != nil {
			return nil, fmt.Errorf("greader: build request failed: %w", err)
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("Authorization", "GoogleLogin auth="+token)
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("greader: request %s failed: %w", path, err)
		}
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("greader: read response failed: %w", readErr)
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			p.tokenMu.Lock()
			p.token = ""
			p.tokenMu.Unlock()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("greader: %s failed (http %d)", path, resp.StatusCode)
		}
		return respBody, nil
	}
}

// ensureToken 返回缓存的 Auth token，为空时登录
func (p *GReaderProvider) ensureToken() (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.token != "" {
		return p.token, nil
	}
	if err := p.login(); err != nil {
		return "", err
	}
	return p.token, nil
}

// login ClientLogin：表单提交用户名/密码，从响应 Auth= 行提取 token
func (p *GReaderProvider) login() error {
	form := url.Values{"Email": {p.username}, "Passwd": {p.password}}
	resp, err := p.httpClient.PostForm(p.apiBase+"/accounts/ClientLogin", form)
	if err != nil {
		return fmt.Errorf("greader: login request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("greader: read login response failed: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: greader login rejected (http %d)", ErrNotAuthorized, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("greader: login failed (http %d)", resp.StatusCode)
	}
	for _, line := range strings.Split(string(body), "\n") {
		if value, ok := strings.CutPrefix(line, "Auth="); ok {
			p.token = strings.TrimSpace(value)
			return nil
		}
	}
	return errors.New("greader: login response missing Auth token")
}

// greaderFeedStream 构造 feed 流标识：优先用数字 ID（feed/123），否则退回 URL 形式
func greaderFeedStream(op SubscriptionOp) string {
	if op.RemoteID > 0 {
		return "feed/" + strconv.FormatInt(op.RemoteID, 10)
	}
	return "feed/" + op.URL
}

// parseGReaderStreamID 解析 "feed/123" 为数字；失败返回 0
func parseGReaderStreamID(streamID string) int64 {
	_, num, found := strings.Cut(streamID, "feed/")
	if !found {
		return 0
	}
	if id, err := strconv.ParseInt(num, 10, 64); err == nil {
		return id
	}
	return 0
}

// parseGReaderItemID 解析条目 ID。
// 长式（带 tag:google.com,2005:reader/item/ 前缀）按协议规范为补零十六进制，仅按 16 进制解析；
// 短式保留十进制优先、十六进制兜底兼容。
func parseGReaderItemID(id string) int64 {
	num, ok := strings.CutPrefix(id, greaderItemIDPrefix)
	if ok {
		if value, err := strconv.ParseInt(num, 16, 64); err == nil {
			return value
		}
		return 0
	}
	if value, err := strconv.ParseInt(num, 10, 64); err == nil {
		return value
	}
	if value, err := strconv.ParseInt(num, 16, 64); err == nil {
		return value
	}
	return 0
}

// greaderUnixTime Unix 秒转 time；0 返回零值
func greaderUnixTime(unix int64) time.Time {
	if unix <= 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}
