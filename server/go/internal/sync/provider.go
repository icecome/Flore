// Package sync 定义远端服务端同步的 Provider 接口与传输类型。
// 本包是叶子包：不依赖 services（避免循环导入），
// 引擎与配置逻辑位于 services 包。
package sync

import (
	"context"
	"errors"
	"time"
)

// 预定义 Provider 名称，同时是 Source.RemoteOrigin 的取值域。
// 只列出已实现的 Provider；新增实现时在此登记，sync_config 的白名单与引擎分支同步更新。
const (
	ProviderMiniflux = "miniflux"
	ProviderGReader  = "greader"
)

// ErrUnsupported 操作当前 Provider 不支持（如 P0 阶段 Miniflux 的 rename/move）
var ErrUnsupported = errors.New("operation not supported by provider")

// RemoteFeed 远端订阅源描述
type RemoteFeed struct {
	// RemoteID 远端 feed 标识（Miniflux feed_id / GReader streamID / TTRSS feed_id）
	RemoteID int64
	// URL 订阅源地址（对齐主键）
	URL string
	// Title 源标题
	Title string
	// FolderName 远端文件夹/分类名（空表示未分组）
	FolderName string
}

// RemoteItem 远端文章条目
type RemoteItem struct {
	// RemoteID 远端条目标识（增量游标用）
	RemoteID int64
	// SourceRemoteID 所属源的远端标识
	SourceRemoteID int64
	// SourceURL 所属源地址（本地对齐主键）
	SourceURL string
	// Link 文章链接（本地去重主键之一）
	Link string
	// Title 标题
	Title string
	// Content 正文 HTML（可为空）
	Content string
	// Author 作者
	Author string
	// PubDate 发布时间
	PubDate time.Time
	// IsRead / IsStarred 远端状态
	IsRead    bool
	IsStarred bool
	// UpdatedAt 远端条目更新时间（LWW 合并依据）
	UpdatedAt time.Time
}

// StateChange 状态变更推送（本地 -> 远端）
type StateChange struct {
	// RemoteIDs 远端条目标识列表（批量同一状态变更）
	RemoteIDs []int64
	// Kind 变更种类
	Kind StateChangeKind
}

// StateChangeKind 状态变更种类
type StateChangeKind string

const (
	// StateChangeRead 标记已读
	StateChangeRead StateChangeKind = "read"
	// StateChangeUnread 标记未读
	StateChangeUnread StateChangeKind = "unread"
	// StateChangeStar 加星标
	StateChangeStar StateChangeKind = "star"
	// StateChangeUnstar 取消星标
	StateChangeUnstar StateChangeKind = "unstar"
)

// SubscriptionOp 订阅变更操作（本地 -> 远端）
type SubscriptionOp struct {
	// Op 操作种类
	Op SubscriptionOpKind
	// URL 订阅源地址
	URL string
	// RemoteID 目标源的远端标识（unsubscribe/rename/move 用）
	RemoteID int64
	// Title 新标题（rename 用）
	Title string
	// FolderName 目标文件夹名（move/subscribe 归类用）
	FolderName string
}

// SubscriptionOpKind 订阅操作种类
type SubscriptionOpKind string

const (
	// OpSubscribe 添加订阅
	OpSubscribe SubscriptionOpKind = "subscribe"
	// OpUnsubscribe 取消订阅
	OpUnsubscribe SubscriptionOpKind = "unsubscribe"
	// OpRename 重命名
	OpRename SubscriptionOpKind = "rename"
	// OpMove 移动文件夹
	OpMove SubscriptionOpKind = "move"
)

// RemoteProvider API 类后端（Miniflux / GReader 兼容 / TTRSS）
type RemoteProvider interface {
	// Name 提供器名称（ProviderMiniflux 等）
	Name() string

	// PullSubscriptions 拉取远端订阅源列表（含文件夹归类）
	PullSubscriptions(ctx context.Context) ([]RemoteFeed, error)

	// PushSubscription 推送订阅变更（subscribe / unsubscribe / rename / move）
	PushSubscription(ctx context.Context, op SubscriptionOp) error

	// PullItems 拉取自 since 起变更的条目（含状态与正文）
	PullItems(ctx context.Context, since time.Time) ([]RemoteItem, error)

	// PushStates 推送状态变更（已读/星标）
	PushStates(ctx context.Context, states []StateChange) error

	// HealthCheck 检测可达性与认证有效性
	HealthCheck(ctx context.Context) error
}
