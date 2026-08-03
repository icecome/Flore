package models

// Folder 对应 Prisma 的 Folder 表
type Folder struct {
	ID            int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name          string    `json:"name" gorm:"not null"`
	ParentID      *int      `json:"parentId" gorm:"column:parentId"`
	CreatedAtTime MilliTime `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAtTime MilliTime `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime;autoCreateTime"`
}

// Source 对应 Prisma 的 Source 表
type Source struct {
	ID             int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name           string    `json:"name" gorm:"not null"`
	URL            string    `json:"url" gorm:"not null"`
	FolderID       *int      `json:"folderId" gorm:"column:folderId;index"`
	ListRule       string    `json:"listRule" gorm:"column:listRule;not null"`
	Interval       int       `json:"interval" gorm:"default:120"`
	Active         bool      `json:"active" gorm:"default:true"`
	IsPrivate      bool      `json:"isPrivate" gorm:"column:isPrivate;default:false;index"`
	HideInTimeline bool      `json:"hideInTimeline" gorm:"column:hideInTimeline;default:false;index"`
	CreatedAtTime  MilliTime `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAtTime  MilliTime `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime;autoCreateTime"`

	// 前端展示用，非数据库字段
	UnreadCount    int64             `json:"unreadCount" gorm:"-"`
	LastFetchAt    NullableMilliTime `json:"lastFetchAt" gorm:"-"`
	LastSuccessAt  NullableMilliTime `json:"lastSuccessAt" gorm:"-"`
	FetchFailCount int               `json:"fetchFailCount" gorm:"-"`
	LastError      *string           `json:"lastError" gorm:"-"`
}

// SourceHealth 订阅源健康状态（由 Go 后端独立维护）
type SourceHealth struct {
	SourceID       int               `json:"sourceId" gorm:"column:sourceId;primaryKey"`
	LastFetchAt    NullableMilliTime `json:"lastFetchAt" gorm:"column:lastFetchAt"`
	LastSuccessAt  NullableMilliTime `json:"lastSuccessAt" gorm:"column:lastSuccessAt"`
	FetchFailCount int               `json:"fetchFailCount" gorm:"column:fetchFailCount;default:0"`
	LastError      *string           `json:"lastError" gorm:"column:lastError"`
	UpdatedAtTime  MilliTime         `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime;autoCreateTime"`
	// NextRetryAtUnix 退避期截止时间（Unix 秒），0 表示无退避。
	// 抓取连续失败时递增，避免僵尸源（超时/503）每次全量刷新都拖慢整体。
	NextRetryAtUnix int64 `json:"nextRetryAtUnix" gorm:"column:nextRetryAtUnix;default:0"`
	// NextCheckAtUnix 下次自动抓取截止时间（Unix 秒），0 表示使用固定 interval 计算。
	// 由 adaptiveNextCheckAt 计算，用于自适应调度：活跃源查得频繁，冷门源查得稀疏。
	NextCheckAtUnix int64 `json:"nextCheckAtUnix" gorm:"column:nextCheckAtUnix;default:0;index"`
	// 以下两字段缓存源的上次响应头，用于增量抓取（HTTP 304 协商）
	FeedLastModified *string `json:"feedLastModified" gorm:"column:feedLastModified"`
	FeedETag         *string `json:"feedEtag" gorm:"column:feedEtag"`
}

// Item 对应 Prisma 的 Item 表
type Item struct {
	ID            int               `json:"id" gorm:"primaryKey;autoIncrement"`
	SourceID      int               `json:"sourceId" gorm:"column:sourceId;not null;index;uniqueIndex:idx_items_link_source,priority:1"`
	Title         string            `json:"title" gorm:"not null"`
	Link          string            `json:"link" gorm:"not null;uniqueIndex:idx_items_link_source,priority:2"`
	Desc          *string           `json:"desc"`
	Author        *string           `json:"author"`
	PubDate       NullableMilliTime `json:"pubDate" gorm:"column:pubDate"`
	IsRead        bool              `json:"isRead" gorm:"column:isRead;default:false;index"`
	IsStarred     bool              `json:"isStarred" gorm:"column:isStarred;default:false;index"`
	IsReadLater   bool              `json:"isReadLater" gorm:"column:isReadLater;default:false;index"`
	CreatedAtTime MilliTime         `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`

	Source *Source `json:"source,omitempty" gorm:"foreignKey:SourceID;references:ID;constraint:OnDelete:CASCADE"`
}

// ItemWithSource 用于返回文章及其源信息
type ItemWithSource struct {
	Item
	SourceName     string  `json:"sourceName"`
	SourceURL      string  `json:"sourceUrl"`
	SourceFolderID *int    `json:"sourceFolderId" gorm:"column:source_folder_id"`
}

// ItemSearch FTS5 全文搜索虚拟表（仅用于 GORM 查询，实际建表使用原生 SQL）
type ItemSearch struct {
	ItemID  int    `json:"itemId" gorm:"column:itemId"`
	Title   string `json:"title" gorm:"column:title"`
	Content string `json:"content" gorm:"column:content"`
}

// TableName 指定 FTS5 虚拟表名
func (ItemSearch) TableName() string {
	return "ItemSearch"
}

// ReadabilityCache 阅读模式内容缓存
type ReadabilityCache struct {
	ItemID      int       `json:"itemId" gorm:"column:itemId;primaryKey"`
	Title       string    `json:"title"`
	Byline      string    `json:"byline"`
	Content     string    `json:"content"`
	TextContent string    `json:"textContent"`
	Excerpt     string    `json:"excerpt"`
	SiteName    string    `json:"siteName"`
	URL         string    `json:"url"`
	CachedAt    MilliTime `json:"cachedAt" gorm:"column:cachedAt;autoCreateTime"`
}

// TableName 指定 ReadabilityCache 表名
func (ReadabilityCache) TableName() string {
	return "ReadabilityCache"
}

// TableName 指定 SourceHealth 表名
func (SourceHealth) TableName() string {
	return "SourceHealth"
}

// TableName 指定 Folder 表名
func (Folder) TableName() string {
	return "Folder"
}

// TableName 指定 Source 表名（Prisma 默认使用模型名）
func (Source) TableName() string {
	return "Source"
}

// TableName 指定 Item 表名
func (Item) TableName() string {
	return "Item"
}

// FilterRule 过滤规则
type FilterRule struct {
	ID         int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name       string    `json:"name" gorm:"column:name"`
	Enabled    bool      `json:"enabled" gorm:"column:enabled;default:true"`
	Priority   int       `json:"priority" gorm:"column:priority;default:0"`
	Scope      string    `json:"scope" gorm:"column:scope;default:global"`
	SourceID   *int      `json:"sourceId" gorm:"column:sourceId"`
	FolderID   *int      `json:"folderId" gorm:"column:folderId"`
	Conditions string    `json:"conditions" gorm:"column:conditions"`
	Action     string    `json:"action" gorm:"column:action"`
	CreatedAt  MilliTime `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
}

// TableName 指定 FilterRule 表名
func (FilterRule) TableName() string {
	return "FilterRule"
}

// Setting 键值对配置存储，替代 localStorage 实现跨平台同步
type Setting struct {
	Key   string `json:"key" gorm:"primaryKey;column:key"`
	Value string `json:"value" gorm:"column:value;not null"`
}

// TableName 指定 Setting 表名
func (Setting) TableName() string {
	return "Setting"
}
