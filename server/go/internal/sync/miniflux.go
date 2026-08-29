package sync

import (
	"context"
	"fmt"
	"net/http"
	"time"

	miniflux "miniflux.app/v2/client"
)

// MinifluxProvider 基于 Miniflux 原生 REST API 的同步后端
type MinifluxProvider struct {
	client *miniflux.Client
}

// NewMinifluxProvider 创建 Miniflux 提供器。
// httpClient 由调用方（services 层）构建，复用代理与超时配置。
func NewMinifluxProvider(endpoint, token string, httpClient *http.Client) *MinifluxProvider {
	opts := []miniflux.Option{miniflux.WithAPIKey(token)}
	if httpClient != nil {
		opts = append(opts, miniflux.WithHTTPClient(httpClient))
	}
	return &MinifluxProvider{
		client: miniflux.NewClientWithOptions(endpoint, opts...),
	}
}

// Name 提供器名称
func (p *MinifluxProvider) Name() string {
	return ProviderMiniflux
}

// PullSubscriptions 拉取远端订阅源列表（含文件夹归类）
func (p *MinifluxProvider) PullSubscriptions(ctx context.Context) ([]RemoteFeed, error) {
	feeds, err := p.client.Feeds()
	if err != nil {
		return nil, fmt.Errorf("miniflux: pull feeds failed: %w", err)
	}
	result := make([]RemoteFeed, 0, len(feeds))
	for _, feed := range feeds {
		folderName := ""
		if feed.Category != nil {
			folderName = feed.Category.Title
		}
		result = append(result, RemoteFeed{
			RemoteID:   feed.ID,
			URL:        feed.FeedURL,
			Title:      feed.Title,
			FolderName: folderName,
		})
	}
	return result, nil
}

// PushSubscription 推送订阅变更。
// P0 仅支持 subscribe / unsubscribe；rename / move 返回 ErrUnsupported。
func (p *MinifluxProvider) PushSubscription(ctx context.Context, op SubscriptionOp) error {
	switch op.Op {
	case OpSubscribe:
		categoryID, err := p.resolveCategoryID(op.FolderName)
		if err != nil {
			return err
		}
		feedID, err := p.client.CreateFeed(&miniflux.FeedCreationRequest{
			FeedURL:    op.URL,
			CategoryID: categoryID,
		})
		if err != nil {
			return fmt.Errorf("miniflux: subscribe feed failed: %w", err)
		}
		if op.Title != "" {
			// 订阅后重命名为本地标题
			if _, err := p.client.UpdateFeed(feedID, &miniflux.FeedModificationRequest{
				Title: miniflux.SetOptionalField(op.Title),
			}); err != nil {
				return fmt.Errorf("miniflux: rename subscribed feed failed: %w", err)
			}
		}
		return nil
	case OpUnsubscribe:
		if err := p.client.DeleteFeed(op.RemoteID); err != nil {
			return fmt.Errorf("miniflux: unsubscribe feed failed: %w", err)
		}
		return nil
	case OpRename, OpMove:
		return fmt.Errorf("%w: miniflux rename/move", ErrUnsupported)
	default:
		return fmt.Errorf("miniflux: unknown subscription op: %s", op.Op)
	}
}

// PullItems 拉取自 since 起变更的条目（含状态与正文）。
// 通过 changed_after 增量 + offset 分页；新条目与状态变更统一覆盖。
func (p *MinifluxProvider) PullItems(ctx context.Context, since time.Time) ([]RemoteItem, error) {
	// feedID -> feed URL 映射（本地 Source 对齐主键）
	feeds, err := p.client.Feeds()
	if err != nil {
		return nil, fmt.Errorf("miniflux: pull feeds failed: %w", err)
	}
	feedURLs := make(map[int64]string, len(feeds))
	for _, feed := range feeds {
		feedURLs[feed.ID] = feed.FeedURL
	}

	const limit = 100
	var result []RemoteItem
	for offset := 0; ; offset += limit {
		filter := &miniflux.Filter{
			Limit:  limit,
			Offset: offset,
			Order:  "id",
			Statuses: []string{
				miniflux.EntryStatusRead,
				miniflux.EntryStatusUnread,
			},
		}
		if !since.IsZero() {
			filter.ChangedAfter = since.Unix()
		}
		entries, err := p.client.EntriesContext(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("miniflux: pull entries failed: %w", err)
		}
		for _, entry := range entries.Entries {
			result = append(result, RemoteItem{
				RemoteID:       entry.ID,
				SourceRemoteID: entry.FeedID,
				SourceURL:      feedURLs[entry.FeedID],
				Link:           entry.URL,
				Title:          entry.Title,
				Content:        entry.Content,
				Author:         entry.Author,
				PubDate:        entry.Date,
				IsRead:         entry.Status == miniflux.EntryStatusRead,
				IsStarred:      entry.Starred,
				UpdatedAt:      entry.ChangedAt,
			})
		}
		if len(entries.Entries) < limit {
			break
		}
	}
	return result, nil
}

// PushStates 推送状态变更（已读/星标，批量）
func (p *MinifluxProvider) PushStates(ctx context.Context, states []StateChange) error {
	for _, state := range states {
		if len(state.RemoteIDs) == 0 {
			continue
		}
		switch state.Kind {
		case StateChangeRead:
			if err := p.client.UpdateEntriesContext(ctx, state.RemoteIDs, miniflux.EntryStatusRead); err != nil {
				return fmt.Errorf("miniflux: push read states failed: %w", err)
			}
		case StateChangeUnread:
			if err := p.client.UpdateEntriesContext(ctx, state.RemoteIDs, miniflux.EntryStatusUnread); err != nil {
				return fmt.Errorf("miniflux: push unread states failed: %w", err)
			}
		case StateChangeStar:
			if err := p.client.UpdateEntriesStarredContext(ctx, state.RemoteIDs, true); err != nil {
				return fmt.Errorf("miniflux: push starred states failed: %w", err)
			}
		case StateChangeUnstar:
			if err := p.client.UpdateEntriesStarredContext(ctx, state.RemoteIDs, false); err != nil {
				return fmt.Errorf("miniflux: push unstarred states failed: %w", err)
			}
		default:
			return fmt.Errorf("miniflux: unknown state change kind: %s", state.Kind)
		}
	}
	return nil
}

// HealthCheck 检测可达性与认证有效性
func (p *MinifluxProvider) HealthCheck(ctx context.Context) error {
	if _, err := p.client.Me(); err != nil {
		return fmt.Errorf("miniflux: health check failed: %w", err)
	}
	return nil
}

// resolveCategoryID 按名称查找分类，不存在则创建；空名称回落默认分类
func (p *MinifluxProvider) resolveCategoryID(name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	categories, err := p.client.Categories()
	if err != nil {
		return 0, fmt.Errorf("miniflux: list categories failed: %w", err)
	}
	for _, category := range categories {
		if category.Title == name {
			return category.ID, nil
		}
	}
	created, err := p.client.CreateCategory(name)
	if err != nil {
		return 0, fmt.Errorf("miniflux: create category failed: %w", err)
	}
	return created.ID, nil
}
