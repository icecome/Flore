# 稍后阅读与多端同步 — 设计方案

**版本**：v2（2026-08-03）
**状态**：设计定稿
**超期**：`docs/planning/pending/bookmark-collector-design.md`（v1，合并入本文档后归档）

---

## TL;DR

本文档定义稍后阅读功能的完整设计，包括：

1. **核心功能**：从任意 URL 保存文章到本地，支持全文提取和 FTS5 搜索
2. **同步架构**：并行多后端模型，用户可同时启用多个同步通道
3. **四个后端**：Miniflux 推送、WebDAV、S3、GitHub
4. **实施路线**：P0-P4 分阶段交付

---

## 1. 整体架构

### 1.1 数据流

```
外部 URL  →  Readability 提取  →  本地 SQLite（主存储）
                                         │
                              ┌──────────┼──────────┐
                              │          │          │
                              ▼          ▼          ▼
                         Miniflux    文件类后端     web 前端
                          (API)     (JSON 文件)     (展示)
                                       │
                              ┌────────┼────────┐
                              │        │        │
                              ▼        ▼        ▼
                          WebDAV     S3     GitHub
```

### 1.2 核心原则

| 原则 | 说明 |
|------|------|
| **本地优先** | 本地 SQLite 始终是主存储，所有后端同步只是副本 |
| **并行不悖** | 各后端互不冲突，用户可启用任意组合 |
| **无自动检测** | 不做自动降级，用户显式配置要启用的后端 |
| **LWW 去重** | 所有后端统一按 URL 去重，Last Writer Wins |

### 1.3 与项目策略的关系

| 策略 | 本设计如何遵循 |
|------|--------------|
| 不自建同步 | 所有同步通道均复用用户已有的基础设施 |
| 数据主权 | 本地 SQLite 始终是主存储，后端只是副本 |
| 协议客户端 | Miniflux 后端使用官方 API 写入，Fever API 读取 |

---

## 2. 同步数据格式

### 2.1 文件类后端（WebDAV / S3 / GitHub）统一格式

所有文件类后端共享同一个 JSON 格式，存储为单一文件 `readlater.json`：

```json
{
  "version": 1,
  "updatedAt": 1725000000000,
  "articles": [
    {
      "id": "a1b2c3d4e5f6...",           // SHA256(URL) 前 32 位
      "url": "https://example.com/article",
      "title": "文章标题",
      "content": "全文 HTML...",
      "textContent": "纯文本内容...",
      "author": "作者名",
      "savedAt": 1725000000000,
      "updatedAt": 1725000000000,
      "isRead": false,
      "isStarred": false
    }
  ]
}
```

### 2.2 Miniflux 后端格式

走 Miniflux 原生 API `POST /v1/entries`，payload：

```json
{
  "feed_id": 42,
  "title": "文章标题",
  "url": "https://example.com/article",
  "content": "全文 HTML...",
  "author": "作者名",
  "published_at": 1725000000,
  "status": "unread",
  "starred": false
}
```

### 2.3 冲突策略

**LWW + URL 去重**：

```
合并两条记录 A 和 B：
  1. 计算 id = SHA256(url)
  2. 若 id 已存在，取 updatedAt 较大的保留
  3. 若 updatedAt 相同，取 savedAt 较大的保留
  4. 若两者都相同，任意保留一条
```

---

## 3. SyncProvider 接口定义

### 3.1 Go 接口

```go
// sync/provider.go

type Article struct {
    ID          string `json:"id"`          // SHA256(URL) 前 32 位
    URL         string `json:"url"`
    Title       string `json:"title"`
    Content     string `json:"content"`
    TextContent string `json:"textContent,omitempty"`
    Author      string `json:"author,omitempty"`
    SavedAt     int64  `json:"savedAt"`     // 毫秒时间戳
    UpdatedAt   int64  `json:"updatedAt"`   // 毫秒时间戳
    IsRead      bool   `json:"isRead"`
    IsStarred   bool   `json:"isStarred"`
}

type SyncProvider interface {
    // Name 返回提供器名称，用于日志和配置 KEY
    Name() string

    // Push 推送单篇文章到后端
    Push(ctx context.Context, article Article) error

    // PushBatch 批量推送（文件类后端可合并写入减少请求）
    PushBatch(ctx context.Context, articles []Article) error

    // Pull 拉取后端所有文章
    Pull(ctx context.Context) ([]Article, error)

    // Delete 从后端删除指定文章
    Delete(ctx context.Context, id string) error

    // HealthCheck 检测后端可达性
    HealthCheck(ctx context.Context) error
}
```

### 3.2 配置结构

```go
// sync/config.go

type SyncConfig struct {
    Miniflux *MinifluxConfig `json:"miniflux,omitempty"`
    WebDAV   *WebDAVConfig   `json:"webdav,omitempty"`
    S3       *S3Config       `json:"s3,omitempty"`
    GitHub   *GitHubConfig   `json:"github,omitempty"`
}

type MinifluxConfig struct {
    Enabled   bool   `json:"enabled"`
    Endpoint  string `json:"endpoint"`  // https://miniflux.example.org
    APIKey    string `json:"apiKey"`
    FeedID    int64  `json:"feedId"`    // 稍后阅读专用 Feed ID
    Insecure  bool   `json:"insecure"`  // 跳过 TLS 验证
}

type WebDAVConfig struct {
    Enabled  bool   `json:"enabled"`
    URL      string `json:"url"`      // https://webdav.example.org/readlater/
    Username string `json:"username"`
    Password string `json:"password"`
}

type S3Config struct {
    Enabled     bool   `json:"enabled"`
    Endpoint    string `json:"endpoint"`    // s3.amazonaws.com 或自定义
    Region      string `json:"region"`      // us-east-1
    Bucket      string `json:"bucket"`
    AccessKey   string `json:"accessKey"`
    SecretKey   string `json:"secretKey"`
    PathPrefix  string `json:"pathPrefix"`  // 可选，如 "flore/readlater/"
    Insecure    bool   `json:"insecure"`    // 禁用 SSL（MinIO 本地）
}

type GitHubConfig struct {
    Enabled    bool   `json:"enabled"`
    Owner      string `json:"owner"`      // 仓库所有者
    Repo       string `json:"repo"`       // 仓库名
    Branch     string `json:"branch"`     // 默认 main
    Token      string `json:"token"`      // Personal Access Token
    FilePath   string `json:"filePath"`   // 默认 "flore/readlater.json"
}
```

### 3.3 存储位置

配置存储在 `Settings` 表（已有键值对存储），KEY 前缀为 `sync_`：

```
sync_miniflux_enabled  → "true"
sync_miniflux_endpoint → "https://miniflux.example.org"
sync_miniflux_apikey   → "xxx"
sync_miniflux_feedid   → "42"
sync_webdav_enabled    → "true"
sync_webdav_url        → "https://..."
...
```

---

## 4. 各后端实现

### 4.1 Miniflux

**依赖**：`miniflux.app/v2/client`

**流程**：

```
Push：
  1. 调用 client.ImportFeedEntry(feedID, articlePayload)
  2. 返回 201 Created → 成功
  3. 返回 200 OK → 已存在（重复）

Pull：
  1. 通过 Fever API 读取该 Feed 的文章
  2. 或通过 Miniflux API 查询条目
  3. 合并到本地 DB

Delete：
  1. 调用 Miniflux API 更新条目状态
```

**前置条件**：
- 用户需在 Miniflux 上有一个可用的 Feed（可以是任意 Feed，建议创建一个专用 Feed）
- 用户需生成 API Key（Setting > API Keys）

**注意**：`ImportFeedEntry` 要求 `feed_id` 必须对应 Miniflux 上已存在的 Feed。Flore 不负责创建 Feed，用户需自行配置。

### 4.2 WebDAV

**依赖**：`github.com/studio-b12/gowebdav`

**流程**：

```
Push：
  1. Pull 远端 readlater.json
  2. 合并本地文章到远端列表（LWW 去重）
  3. 写回远端 readlater.json

Pull：
  1. 读取远端 readlater.json
  2. 解析为 Article 列表
  3. 合并到本地 DB

Delete：
  1. Pull 远端 readlater.json
  2. 从列表中移除指定 id
  3. 写回远端
```

**文件路径**：`{WebDAV_URL}/readlater.json`

### 4.3 S3

**依赖**：`github.com/minio/minio-go/v7`

**流程**：与 WebDAV 一致，文件路径为 `{PathPrefix}readlater.json`

### 4.4 GitHub

**依赖**：`github.com/google/go-github/v70`

**流程**：

```
Push：
  1. 调用 GetContents 读取远端 readlater.json
  2. 获取当前文件 SHA（用于 UpdateFile 的版本校验）
  3. 合并本地文章到远端列表（LWW 去重）
  4. 调用 CreateFile（首次）或 UpdateFile（更新），传入 SHA

Pull：
  1. 调用 GetContents 读取远端 readlater.json
  2. 解析为 Article 列表
  3. 合并到本地 DB

Delete：
  1. 读取远端 readlater.json
  2. 从列表中移除指定 id
  3. 写回远端
```

**注意**：
- GitHub Contents API 单文件最大 1MB（base64 编码），稍后阅读数据通常 < 100KB，足够
- 每次写入生成一个 commit，commit message 可设为 `"Flore: sync readlater"` 并在描述中记录变更数量
- 需使用经典 PAT 或细粒度 PAT，权限要求 `contents:write`
- 建议文件路径统一为 `flore/readlater.json`

---

## 5. Sync Engine 调度

### 5.1 触发时机

| 时机 | 操作 | 说明 |
|------|------|------|
| 用户保存新文章 | Push 到所有已启用的后端 | 实时推送 |
| 用户标记已读/星标 | Push 更新到所有已启用的后端 | 实时推送 |
| 用户删除文章 | Delete 从所有已启用的后端 | 实时推送 |
| 应用启动时 | Pull 从所有已启用的后端 | 后台合并 |
| 定时（每 5 分钟） | Pull 从所有已启用的后端 | 可选，用于检测其他设备的新增 |

### 5.2 合并逻辑

```
func MergeArticles(local, remote []Article) []Article {
    merged := map[string]Article{}  // key = id

    // 先加载本地
    for _, a := range local {
        merged[a.ID] = a
    }

    // 合并远端，LWW
    for _, a := range remote {
        if existing, ok := merged[a.ID]; ok {
            if a.UpdatedAt > existing.UpdatedAt {
                merged[a.ID] = a
            }
        } else {
            merged[a.ID] = a
        }
    }

    // 转回 slice
    result := make([]Article, 0, len(merged))
    for _, a := range merged {
        result = append(result, a)
    }
    return result
}
```

### 5.3 错误处理

| 错误类型 | 行为 |
|---------|------|
| 后端不可达（网络错误） | 记录日志，跳过该后端，不影响其他后端 |
| 认证失败（401/403） | 记录日志，显示配置错误提示，跳过该后端 |
| 速率限制（429） | 等待后重试最多 3 次，仍失败则跳过 |
| 冲突（SHA 不匹配，仅 GitHub） | 重新 Pull 后重试 Push |

---

## 6. 前端改动

### 6.1 设置页 — 同步配置

在现有的设置页新增「同步」Tab，包含：

```
┌─────────────────────────────────────┐
│ 同步设置                             │
│                                     │
│ □ 启用 Miniflux 同步                 │
│   服务器地址: [________________]     │
│   API Key:    [________________]     │
│   Feed ID:    [________________]     │
│                                     │
│ □ 启用 WebDAV 同步                   │
│   URL:        [________________]     │
│   用户名:     [________________]     │
│   密码:       [________________]     │
│   [测试连接]                         │
│                                     │
│ □ 启用 S3 同步                       │
│   Endpoint:   [________________]     │
│   Bucket:     [________________]     │
│   AccessKey:  [________________]     │
│   SecretKey:  [________________]     │
│                                     │
│ □ 启用 GitHub 同步                    │
│   仓库:  [owner]/[repo]              │
│   分支:  [main    ]                  │
│   Token:      [________________]     │
│   [测试连接]                         │
│                                     │
│ [保存设置]                           │
└─────────────────────────────────────┘
```

### 6.2 添加链接弹窗

沿用 v1 设计中的 `AddLinkModal` 组件，不做改动。

### 6.3 同步状态指示器

在侧边栏底部或工具栏显示同步状态：

```
🔄 同步中...      → 正在同步
✓ 同步完成       → 上次同步成功
⚠️ 同步失败: xxx → 某个后端出错
```

---

## 7. 现有能力复用清单

| 现有能力 | 复用于 |
|---------|-------|
| `Item` 模型 `isReadLater`/`isStarred` 字段 | 稍后阅读核心功能 |
| `GET /api/items?readLater=true` 筛选 | 后端查询 |
| 侧边栏"稍后阅读""收藏"按钮 | 前端入口 |
| `FetchReadability()` 全文提取 | 内容提取 |
| 文章列表/阅读器组件 | 展示 |
| Markdown/JSON 导出 | 数据导出 |
| `Settings` 表键值对存储 | 同步配置存储 |
| 路由鉴权与代理 | 后端 API 保护 |

---

## 8. 实施路线图

### 阶段 0：核心功能（P0，1-2 人日）

| 任务 | 文件 | 说明 |
|------|------|------|
| 数据库初始化添加虚拟 Source | `models/models.go` + `migrations.go` | v3 迁移，创建 `isManual=true` 的 Source |
| 后端 `SaveArticleByURL` Service | `services/reader.go` | 调用 FetchReadability，构造 Item，写入 |
| 后端 `POST /api/items/save` Handler | `handlers/reader.go` | 路由注册 + 校验 |
| 后端 `GET /api/items/sync/config` | `handlers/reader.go` | 同步配置读写接口 |
| 前端 AddLinkModal 组件 | `components/AddLinkModal.tsx` | 新组件 |
| 前端侧边栏/工具栏入口 | `App.tsx` + `Sidebar.tsx` | 按钮 + 弹窗触发 |
| TypeScript + Go 验证 | 全局 | tsc + go vet + go build |

### 阶段 1：同步后端实现（P1-P4，3-5 人日/后端）

按以下顺序实现，每个后端独立交付：

| 顺序 | 后端 | 工作量 | 关键库 |
|------|------|--------|--------|
| 1 | Miniflux | 3-5 人日 | `miniflux.app/v2/client` |
| 2 | WebDAV | 3-4 人日 | `studio-b12/gowebdav` |
| 3 | S3 | 3-4 人日 | `minio/minio-go/v7` |
| 4 | GitHub | 3-4 人日 | `google/go-github` |

**每个后端包含**：

| 任务 | 说明 |
|------|------|
| `sync/<name>.go` | Provider 实现（Push/Pull/Delete/HealthCheck） |
| 路由注册 | 后端配置读写 API |
| 前端设置 UI | 配置表单 + 测试连接按钮 |
| 集成测试 | 最小可用后端验证 |

### 阶段 2：Sync Engine 调度（2-3 人日）

| 任务 | 说明 |
|------|------|
| `sync/engine.go` | 调度引擎：启动时 Pull、保存时 Push、定时 Pull |
| `sync/merge.go` | 合并逻辑：LWW + URL 去重 |
| `sync/config.go` | 配置读写：Settings 表存取 |
| 前端同步状态指示器 | 侧边栏状态显示 |

### 阶段 3：收集入口（1-2 人日）

| 任务 | 说明 |
|------|------|
| Bookmarklet 生成器 | 设置页生成可拖拽书签链接 |
| 浏览器扩展 | 右键菜单 + 工具栏按钮（可选，用已有框架） |
| 系统分享集成 | Wails 桌面端接收分享 URL |

---

## 9. 关键设计决策

### 9.1 为什么不做自动检测降级

用户明确要求：**各后端是并行关系，不是互斥或降级关系**。用户可同时启用多个后端，所有后端同时接收推送，不做自动降级。

### 9.2 为什么文件类后端用单文件而非多文件

| 方案 | 优点 | 缺点 |
|------|------|------|
| 单文件 `readlater.json` | 读写各一次 API 调用，原子性 | 并发写入有冲突风险 |
| 每篇文章一个文件 | 并发安全，GitHub 有版本历史 | 文件数多，API 调用次数多 |

**选择单文件**，原因：
- 稍后阅读文章数量通常 < 1000 篇，单文件 JSON 足够
- 多数用户单人使用，并发冲突概率极低
- LWW 策略已覆盖冲突场景

### 9.3 为什么 Miniflux 用专用 Feed 而非通用 Feed

如果用现有的 RSS Feed 来存放稍后阅读文章，会混入该 Feed 的原始内容，导致：
- 稍后阅读列表被 RSS 文章污染
- 与其他设备的 Fever API 同步逻辑冲突

**建议**：用户在 Miniflux 上创建一个标题为"稍后阅读"的 Feed（随便指向一个不更新的 RSS 地址），然后将其 Feed ID 配置到 Flore 中。Flore 通过 `ImportFeedEntry` 写入该 Feed，其他设备通过 Fever API 读取该 Feed 即可。

---

## 10. 未纳入范围

| 项目 | 原因 |
|------|------|
| 移动端原生 App | 超出单人产能，RICE 低 |
| 自建同步服务 | 与项目策略冲突 |
| 浏览器扩展自动发布 | 需 Chrome/Firefox 开发者账号，延后 |
| 全文搜索索引同步 | FTS5 索引本地重建，不同步 |
| 冲突手工解决 UI | LWW 已足够，增加复杂度无收益 |

---

## 11. 依赖与风险

| 风险 | 缓解措施 |
|------|---------|
| Miniflux API 版本变更 | 使用官方 Go 客户端，版本锁定 |
| 用户网络环境复杂 | 健康检查 + 超时控制 + 错误日志 |
| 单文件并发冲突 | LWW 策略，更新时间戳精度到毫秒 |
| 配置信息泄露（Token 等） | 存储在本地 SQLite，不暴露到前端日志 |
| 国内 GitHub 不可达 | 用户可只启用其他后端，不影响使用 |