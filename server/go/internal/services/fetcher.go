package services

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/araddon/dateparse"
	"gorm.io/gorm"

	"github.com/rss/go-server/internal/models"
)

// FeedItem 表示从 RSS/Atom feed 中解析出的单条条目
type FeedItem struct {
	Title       string
	Link        string
	Description string
	Author      string
	PubDate     time.Time
	GUID        string
}

// FeedCond 携带条件请求头，用于增量抓取（HTTP 304 协商）
type FeedCond struct {
	IfModifiedSince *string
	IfNoneMatch     *string
}

// FeedFetchMeta 抓取响应元数据，用于回写 Last-Modified / ETag 供下次条件请求
type FeedFetchMeta struct {
	StatusCode   int
	LastModified *string
	ETag         *string
}

// ErrNotModified 表示源未变化（HTTP 304）
var ErrNotModified = errors.New("feed not modified")

// rssFeed RSS 2.0 结构
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	Description string  `xml:"description"`
	Author      string  `xml:"author"`
	PubDate     string  `xml:"pubDate"`
	GUID        rssGUID `xml:"guid"`
	Content     string  `xml:"encoded"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink bool   `xml:"isPermaLink,attr"`
}

// atomFeed Atom 结构
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Link      atomLink   `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Author    atomAuthor `xml:"author"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	ID        string     `xml:"id"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// fetchHTTPClient 已移除：FetchRSSFeed/FetchRSSFeedWithContext 为死代码，
// 实际抓取通过 ReaderService.BuildFetchHTTPClient() 构建配置化客户端。
// 保留 SSRF 防护 Transport 的构造函数供 BuildFetchHTTPClient 使用。

// FetchSourceFeed 抓取指定订阅源的 RSS/Atom feed 并持久化条目。
// 由 ReaderService 调用，统一处理健康状态、过滤规则与搜索索引。
// 根据设置表中的 fetchTimeout、proxyEnabled、proxyUrl 构建配置化 HTTP 客户端
func (s *ReaderService) FetchSourceFeed(sourceID int) (int, error) {
	nowTime := time.Now()
	now := models.NullableMilliTime{T: &nowTime}

	source, err := s.GetSource(sourceID)
	if err != nil {
		s.updateSourceHealth(sourceID, now, models.NullableMilliTime{}, fmt.Sprintf("source not found: %v", err), 0, source.Interval)
		return 0, fmt.Errorf("source not found: %w", err)
	}

	// 读取该源上次响应的条件请求头，构造增量请求（HTTP 304 协商）
	var cond *FeedCond
	var health models.SourceHealth
	if err := s.getDb().Where("sourceId = ?", sourceID).First(&health).Error; err == nil {
		cond = &FeedCond{IfModifiedSince: health.FeedLastModified, IfNoneMatch: health.FeedETag}
	}

	fetchStart := time.Now()
	client := s.BuildFetchHTTPClient()
	items, meta, err := FetchRSSFeedWithClient(context.Background(), source.URL, client, cond)
	fetchDuration := time.Since(fetchStart)
	if err != nil {
		if errors.Is(err, ErrNotModified) {
			// 源未变化（304）：视为可访问，清退避；无新内容不更新 LastSuccessAt 语义
			s.updateSourceHealth(sourceID, now, now, "", 0, source.Interval)
			slog.Info("fetch not modified (304)", "source", source.Name, "url", source.URL)
			return 0, nil
		}
		errMsg := err.Error()
		s.updateSourceHealth(sourceID, now, models.NullableMilliTime{}, errMsg, 0, source.Interval)
		if fetchDuration > 5*time.Second {
			slog.Warn("fetch slow (failed)", "source", source.Name, "url", source.URL, "duration", fetchDuration, "error", err)
		}
		return 0, fmt.Errorf("failed to fetch feed %s: %w", source.URL, err)
	}

	upsertStart := time.Now()
	newCount, err := s.upsertFeedItems(sourceID, items)
	if err != nil {
		errMsg := err.Error()
		s.updateSourceHealth(sourceID, now, models.NullableMilliTime{}, errMsg, 0, source.Interval)
		return 0, fmt.Errorf("failed to upsert feed items: %w", err)
	}
	upsertDuration := time.Since(upsertStart)
	totalDuration := time.Since(fetchStart)

	s.updateSourceHealth(sourceID, now, now, "", newCount, source.Interval)

	// 回写条件请求头（Last-Modified / ETag），供下次增量抓取
	if meta != nil {
		updates := map[string]interface{}{}
		if meta.LastModified != nil {
			updates["feedLastModified"] = *meta.LastModified
		}
		if meta.ETag != nil {
			updates["feedEtag"] = *meta.ETag
		}
		if len(updates) > 0 {
			if err := s.getDb().Model(&models.SourceHealth{}).Where("sourceId = ?", sourceID).Updates(updates).Error; err != nil {
				slog.Warn("failed to update feed conditional headers", "source_id", sourceID, "error", err)
			}
		}
	}

	if totalDuration > 5*time.Second {
		slog.Warn("fetch slow", "source", source.Name, "url", source.URL,
			"total", totalDuration, "http", fetchDuration, "db", upsertDuration,
			"items", len(items), "new", newCount)
	} else {
		slog.Info("fetch ok", "source", source.Name, "duration", totalDuration,
			"http", fetchDuration, "db", upsertDuration, "items", len(items), "new", newCount)
	}
	return newCount, nil
}

// BuildFetchHTTPClient 根据设置表构建 HTTP 客户端
// - fetchTimeout: 抓取超时（秒），默认 15
// - proxyEnabled + proxyUrl: HTTP 代理
// 启用代理时使用普通 Transport，因为 SSRF 防护的 DialContext 会阻止连接到本地代理
// 内部缓存客户端实例，代理设置变更时自动重建
func (s *ReaderService) BuildFetchHTTPClient() *http.Client {
	timeoutSec := s.GetSettingInt("fetchTimeout", 15)
	if timeoutSec <= 0 {
		timeoutSec = 15
	}

	currentSeq := atomic.LoadInt64(&s.proxySettingsSeq)

	s.cachedHTTPClientMu.RLock()
	if s.cachedHTTPClient != nil && s.cachedHTTPClientSeq == currentSeq {
		client := s.cachedHTTPClient
		s.cachedHTTPClientMu.RUnlock()
		return client
	}
	s.cachedHTTPClientMu.RUnlock()

	s.cachedHTTPClientMu.Lock()
	defer s.cachedHTTPClientMu.Unlock()

	// 双重检查
	if s.cachedHTTPClient != nil && s.cachedHTTPClientSeq == currentSeq {
		return s.cachedHTTPClient
	}

	var transport *http.Transport
	if s.GetSettingBool("proxyEnabled", false) {
		proxyURL := s.GetSetting("proxyUrl")
		if proxyURL != "" {
			if u, err := url.Parse(proxyURL); err == nil {
				transport = &http.Transport{Proxy: http.ProxyURL(u)}
			}
		}
	}
	if transport == nil {
		transport = TransportWithSSRFProtection()
	}

	client := &http.Client{
		Timeout:   time.Duration(timeoutSec) * time.Second,
		Transport: transport,
	}

	jar, err := cookiejar.New(nil)
	if err == nil {
		client.Jar = jar
	}

	s.cachedHTTPClient = client
	s.cachedHTTPClientSeq = currentSeq

	return client
}

// FetchRSSFeedWithClient 从 URL 抓取并解析 RSS 2.0 或 Atom feed，返回条目列表。
// 允许传入自定义 http.Client 用于支持配置化的超时与代理。
// 抓取阶段（DNS/TLS/HTTP）受 ctx 控制；ctx 不影响后续 XML 解析。
func FetchRSSFeedWithClient(ctx context.Context, rawURL string, client *http.Client, cond *FeedCond) ([]FeedItem, *FeedFetchMeta, error) {
	// 使用 ValidateURLOnly 跳过 DNS 解析，因为 SSRF transport 的 DialContext 已在连接阶段
	// 阻断私有 IP，避免与 transport 层的 DNS 解析重复。
	if err := ValidateURLOnly(rawURL); err != nil {
		return nil, nil, fmt.Errorf("url validation failed: %w", err)
	}

	// 为单次抓取设置上限超时，避免 ctx 无 deadline 时请求长期挂起
	// client.Timeout 已限制整体超时，此处用 20s 作为 ctx 上限兜底
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	if cond != nil {
		if cond.IfModifiedSince != nil {
			req.Header.Set("If-Modified-Since", *cond.IfModifiedSince)
		}
		if cond.IfNoneMatch != nil {
			req.Header.Set("If-None-Match", *cond.IfNoneMatch)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	meta := &FeedFetchMeta{StatusCode: resp.StatusCode}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		v := lm
		meta.LastModified = &v
	}
	if et := resp.Header.Get("ETag"); et != "" {
		v := et
		meta.ETag = &v
	}

	// 源未变化：直接返回 304 标记，由调用方跳过解析与 upsert
	if resp.StatusCode == http.StatusNotModified {
		return nil, meta, ErrNotModified
	}
	if resp.StatusCode != http.StatusOK {
		return nil, meta, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}

	// 限制响应体大小为 16MB，防止恶意 RSS 源导致 OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, meta, err
	}

	// 通过检查 XML 根元素判断 feed 类型：Atom 的根元素是 <feed>，RSS 的根元素是 <rss>
	// 限制搜索范围到前 500 字节，避免误匹配正文内容中的 <feed 字样
	bodyStr := string(body)
	searchHead := bodyStr
	if len(searchHead) > 500 {
		searchHead = searchHead[:500]
	}
	if strings.Contains(searchHead, "<feed") {
		items, err := parseAtom(body)
		return items, meta, err
	}
	items, err := parseRSS(body)
	return items, meta, err
}

func parseRSS(body []byte) ([]FeedItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse rss failed: %w", err)
	}

	items := make([]FeedItem, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		pubDate := parseFeedDate(it.PubDate)
		guid := it.GUID.Value
		if guid == "" {
			guid = it.Link
		}
		desc := it.Description
		if desc == "" && it.Content != "" {
			desc = it.Content
		}
		items = append(items, FeedItem{
			Title:       strings.TrimSpace(it.Title),
			Link:        strings.TrimSpace(it.Link),
			Description: strings.TrimSpace(desc),
			Author:      strings.TrimSpace(it.Author),
			PubDate:     pubDate,
			GUID:        guid,
		})
	}
	return items, nil
}

func parseAtom(body []byte) ([]FeedItem, error) {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse atom failed: %w", err)
	}

	items := make([]FeedItem, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		pubDate := parseFeedDate(entry.Published)
		if pubDate.IsZero() {
			pubDate = parseFeedDate(entry.Updated)
		}
		link := entry.Link.Href
		guid := entry.ID
		if guid == "" {
			guid = link
		}
		desc := entry.Summary
		if desc == "" && entry.Content != "" {
			desc = entry.Content
		}
		items = append(items, FeedItem{
			Title:       strings.TrimSpace(entry.Title),
			Link:        strings.TrimSpace(link),
			Description: strings.TrimSpace(desc),
			Author:      strings.TrimSpace(entry.Author.Name),
			PubDate:     pubDate,
			GUID:        guid,
		})
	}
	return items, nil
}

// FetchFeedTitle 抓取 RSS/Atom feed 并返回其标题
func FetchFeedTitle(ctx context.Context, rawURL string, client *http.Client) (string, error) {
	if err := ValidateURLOnly(rawURL); err != nil {
		return "", fmt.Errorf("url validation failed: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}

	bodyStr := string(body)
	searchHead := bodyStr
	if len(searchHead) > 500 {
		searchHead = searchHead[:500]
	}

	if strings.Contains(searchHead, "<feed") {
		var feed atomFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			return "", fmt.Errorf("parse atom feed title failed: %w", err)
		}
		return strings.TrimSpace(feed.Title), nil
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return "", fmt.Errorf("parse rss feed title failed: %w", err)
	}
	return strings.TrimSpace(feed.Channel.Title), nil
}

func parseFeedDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := dateparse.ParseAny(raw); err == nil {
		return t
	}
	return time.Time{}
}

// upsertFeedItems 将 feed 条目写入数据库，按 link 去重。
// 批量查询已有 link 避免逐条 N+1 查询；新文章 ID 推入 indexChan 由后台 goroutine 批量索引。
func (s *ReaderService) upsertFeedItems(sourceID int, items []FeedItem) (int, error) {
	validItems := filterValidItems(items)
	if len(validItems) == 0 {
		return 0, nil
	}

	existingMap, err := s.batchGetExistingItems(sourceID, validItems)
	if err != nil {
		return 0, err
	}

	newItemIDs, err := s.upsertItemsInTx(sourceID, validItems, existingMap)
	if err != nil {
		return 0, err
	}

	// 有新未读写入时使未读计数缓存失效，下次 GetSources 重算（P2-1/B）
	if len(newItemIDs) > 0 {
		s.invalidateUnreadCount()
		// 将新文章 ID 推入批量索引通道，由后台 goroutine 异步处理 FTS5 + 过滤规则
		// 使用非阻塞发送：通道满时跳过，不影响核心抓取流程
		for _, id := range newItemIDs {
			select {
			case s.indexChan <- id:
				s.indexWG.Add(1)
			default:
				// 通道满，降级为同步处理，避免阻塞 worker
				if err := s.indexBatch([]int{id}); err != nil {
					slog.Warn("index batch failed (fallback)", "item_id", id, "error", err)
				}
			}
		}
	}
	// 返回本轮新增条目数，供 Coordinator 累计与前端通知计数使用（根治 C-02）
	return len(newItemIDs), nil
}

// filterValidItems 过滤出有 link 的有效条目
func filterValidItems(items []FeedItem) []FeedItem {
	validItems := make([]FeedItem, 0, len(items))
	for _, it := range items {
		if it.Link != "" {
			validItems = append(validItems, it)
		}
	}
	return validItems
}

// batchGetExistingItems 批量查询已存在的条目
func (s *ReaderService) batchGetExistingItems(sourceID int, items []FeedItem) (map[string]models.Item, error) {
	links := make([]string, len(items))
	for i, it := range items {
		links[i] = it.Link
	}
	var existingItems []models.Item
	if err := s.getDb().Where("sourceId = ? AND link IN ?", sourceID, links).Find(&existingItems).Error; err != nil {
		return nil, err
	}
	existingMap := make(map[string]models.Item, len(existingItems))
	for _, ei := range existingItems {
		existingMap[ei.Link] = ei
	}
	return existingMap, nil
}

// upsertItemsInTx 在事务中创建或更新条目
func (s *ReaderService) upsertItemsInTx(sourceID int, items []FeedItem, existingMap map[string]models.Item) ([]int, error) {
	var newItemIDs []int
	err := s.getDb().Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			id, isNew, err := s.upsertSingleItem(tx, sourceID, it, existingMap)
			if err != nil {
				return err
			}
			if isNew {
				newItemIDs = append(newItemIDs, id)
			}
		}
		return nil
	})
	return newItemIDs, err
}

// upsertSingleItem 创建或更新单条条目，返回 itemID 和是否新增
func (s *ReaderService) upsertSingleItem(tx *gorm.DB, sourceID int, it FeedItem, existingMap map[string]models.Item) (int, bool, error) {
	pubDate := models.NullableMilliTime{}
	if !it.PubDate.IsZero() {
		pubDate = models.NullableMilliTime{T: &it.PubDate}
	}
	desc := it.Description
	author := it.Author

	if existing, ok := existingMap[it.Link]; ok {
		updates := map[string]interface{}{
			"title": it.Title,
			"desc":  desc,
		}
		if author != "" {
			updates["author"] = author
		}
		if !it.PubDate.IsZero() {
			updates["pubDate"] = it.PubDate
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return 0, false, err
		}
		return existing.ID, false, nil
	}

	newItem := models.Item{
		SourceID: sourceID,
		Title:    it.Title,
		Link:     it.Link,
		Desc:     &desc,
		Author:   &author,
		PubDate:  pubDate,
	}
	if err := tx.Create(&newItem).Error; err != nil {
		return 0, false, err
	}
	return newItem.ID, true, nil
}

func (s *ReaderService) applyFiltersAndIndex(itemID int) {
	if err := s.ApplyFilterRules(itemID); err != nil {
		slog.Warn("apply filter rules failed for item", "item_id", itemID, "error", err)
	}
	if err := s.IndexItemForSearch(itemID); err != nil {
		slog.Warn("index item failed", "item_id", itemID, "error", err)
	}
}
