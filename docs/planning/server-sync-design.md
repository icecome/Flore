# 服务端同步（阅读器级）设计方案

**版本**：v1（2026-08-29）
**状态**：设计初稿，待评审
**前置决策**（已拍板）：
- 三层服务端配置：Miniflux（核心）→ FreshRSS（兼容层）→ TTRSS（扩展层）
- 文件类云端存储：WebDAV + S3（OneDrive 远期评估，本方案不展开）
- 文章数据模式：**远端拉取**（连接服务端后文章来自远端，本地不再抓取同步源）
- 文件后端数据范围：**完整数据包**（文件夹 + 订阅源 + 文章 + 状态）

**与既有方案的关系**：`readlater-sync-design.md`（稍后阅读单篇同步）是独立另案，本方案不触碰；两方案共享 `sync_` 配置前缀时以后续合并设计为准。

---

## TL;DR

Flore 连接远端 RSS 服务端，实现「订阅源 + 文章 + 已读/星标状态」的多端同步：

1. **两类后端**：API 后端（Miniflux / FreshRSS / TTRSS）实时同步；文件后端（WebDAV / S3）整包同步
2. **一套 GReader 兼容客户端**覆盖 FreshRSS、TTRSS（插件）、Miniflux（备用通道），降低多服务端接入成本
3. **本地优先原则不变**：本地 SQLite 始终是主存储；连接服务端时，远端成为「文章来源 + 状态权威」，断网仍可离线阅读本地已有数据
4. **分五阶段交付**（P0 Miniflux → P1 FreshRSS → P2 TTRSS → P3 文件后端 → P4 OneDrive 评估）

---

## 1. 整体架构

### 1.1 数据流

```
                    ┌────────────────────────────────────┐
                    │        远端服务端（用户自备）        │
                    │  Miniflux / FreshRSS / TTRSS        │
                    │  （订阅管理 + 文章抓取 + 状态存储）  │
                    └──────────────┬─────────────────────┘
                                   │ API 后端（Pull/Push 实时同步）
┌──────────────┐                   │
│ WebDAV / S3  │── 文件后端        │
│ (整包 JSON)  │   (整包同步)      ▼
└──────────────┘         ┌─────────────────────────┐
                         │   Sync Engine（Go 后端） │
                         │   provider / engine /    │
                         │   merge / config        │
                         └────────────┬────────────┘
                                      ▼
                         ┌─────────────────────────┐
                         │   本地 SQLite（主存储）   │
                         │   Folder/Source/Item     │
                         └────────────┬────────────┘
                                      ▼
                              Web 前端（展示 + 设置）
```

### 1.2 核心原则

| 原则 | 说明 |
|------|------|
| **本地优先** | 本地 SQLite 始终是主存储；远端不可达时降级为离线阅读（本地已有数据全部可用） |
| **远端权威（同步源）** | 连接 API 后端后，`remoteOrigin` 源的文章与状态以远端为准；本地 fetcher 不再抓取这些源 |
| **一套兼容客户端** | GReader 协议客户端覆盖多服务端，Miniflux 原生 API 作为高精度主通道 |
| **并行不悖** | API 后端与文件后端可同时启用，互不冲突（API 后端负责增量，文件后端负责整包备份/迁移） |
| **LWW 去重** | 状态冲突按 updatedAt Last Writer Wins；订阅按 URL 对齐 |
| **不自建同步服务** | 全部复用用户已有基础设施 |

### 1.3 与现有架构的边界

| 现有组件 | 本方案的改动 |
|---------|-------------|
| `fetcher.go` | 不改逻辑；`remoteOrigin` 源被 scheduler 跳过本地抓取 |
| `scheduler.go` | 全量轮询时过滤 `remoteOrigin` 源；Sync Engine 接管其周期 |
| `Item` / `Source` 模型 | 新增少量列（见 §6） |
| `Setting` 表 | 新增 `sync_*` 配置键（见 §6.2） |
| `urlpolicy.go` SSRF 防护 | 出站连接复用 ValidateURL 校验，防止同步后端被配置为内网地址 |
| `readlater-sync-design.md` | 不受影响，另案 |

---

## 2. 同步模式：远端拉取

用户选择「连接服务端」后，Flore 从本地抓取模式切换为远端拉取模式：

```
连接前（本地模式）：
  scheduler 周期轮询 → fetcher 抓取 RSS → 写入 Item

连接后（同步模式，仅 remoteOrigin 源）：
  Sync Engine 周期 Pull → 远端条目（含正文/状态）→ 合并写入 Item
  本地状态变更 → 防抖 Push → 远端
  本地订阅变更 → Push → 远端
```

**关键语义**：

1. **订阅源归属**：连接服务端时执行一次「订阅对齐」——远端源导入本地（标记 `remoteOrigin`），本地已有同名 URL 源合并为同步源；仅在本地存在的源可选择推送到远端（默认询问用户）。
2. **文章来源**：同步源的文章（标题、链接、摘要、正文）来自远端 API / GReader stream / 文件包。
3. **状态回写**：本地标记已读/星标后推送到远端，其他设备可见；反向亦然（LWW）。
4. **混合模式允许**：本地私有源（不推送）与同步源共存，同一数据库内。

---

## 3. 文件后端统一数据格式

WebDAV / S3 共享单一文件 `flore-reader.json`：

```json
{
  "version": 1,
  "updatedAt": 1725000000000,
  "folders": [
    { "id": "f1", "name": "技术", "parentId": null }
  ],
  "sources": [
    {
      "id": "sha256(url) 前 32 位",
      "url": "https://example.com/feed.xml",
      "title": "示例源",
      "folderId": "f1",
      "active": true
    }
  ],
  "items": [
    {
      "id": "sha256(sourceUrl + link) 前 32 位",
      "sourceUrl": "https://example.com/feed.xml",
      "link": "https://example.com/article",
      "title": "文章标题",
      "content": "全文 HTML（可选，超窗口可裁剪）",
      "author": "作者",
      "pubDate": 1725000000000,
      "isRead": false,
      "isStarred": false,
      "updatedAt": 1725000000000
    }
  ]
}
```

**窗口约束**：`items` 仅包含 `pubDate >= now - syncWindowDays`（默认 30 天，设置可调）内的文章，控制文件体积。窗口外的已读/星标状态仍保留在 `items` 中（状态条目不带 content），保证状态多端一致。

**合并策略**：订阅按 URL 对齐（folderId 归属 LWW）；条目按 id 对齐，`updatedAt` 大者胜出。

---

## 4. SyncProvider 接口定义（Go）

```go
// internal/sync/provider.go

// RemoteProvider API 类后端（Miniflux / GReader 兼容 / TTRSS）
type RemoteProvider interface {
    // Name 提供器名称，用于日志与配置 KEY
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

// FileProvider 文件类后端（WebDAV / S3）
type FileProvider interface {
    // Name 提供器名称
    Name() string

    // FetchPackage 拉取远端整包
    FetchPackage(ctx context.Context) (*ReaderPackage, error)

    // StorePackage 写回整包（读-合并-写）
    StorePackage(ctx context.Context, pkg *ReaderPackage) error

    // HealthCheck 检测可达性与认证有效性
    HealthCheck(ctx context.Context) error
}
```

两个接口不合并：API 后端是增量语义，文件后端是整包语义，行为差异大，强行统一会引入空洞实现。

---

## 5. 各后端实现

### 5.1 Miniflux（核心层，原生 API）

**依赖**：`miniflux.app/v2/client`（官方 Go 客户端）

**协议映射**：

| Flore 操作 | Miniflux API |
|-----------|--------------|
| PullSubscriptions | `GET /v1/feeds` + `GET /v1/categories` |
| PushSubscription(subscribe) | `POST /v1/feeds` |
| PushSubscription(unsubscribe) | `DELETE /v1/feeds/{id}` |
| PullItems | `GET /v1/entries?after_entry_id=...`（order=id 增量） |
| PushStates | `PUT /v1/entries`（read / starred 状态） |

**认证**：`X-Auth-Token`（API Key，用户在 Miniflux 设置中生成）。

**优点**：条目内容完整（全文抓取在服务端完成）、官方客户端、语义精准。**首选主通道**。

### 5.2 GReader 兼容层（覆盖 FreshRSS / TTRSS 插件）

**实现**：自写一个 GReader 协议客户端（`internal/sync/greader.go`），无第三方依赖。

| Flore 操作 | GReader 端点 |
|-----------|-------------|
| 登录 | `POST /accounts/ClientLogin` → Auth Token |
| PullSubscriptions | `GET /reader/api/0/subscription/list?output=json` |
| subscribe | `POST /reader/api/0/subscription/quickadd` / `subscription/edit?ac=subscribe` |
| unsubscribe | `POST /reader/api/0/subscription/edit?ac=unsubscribe` |
| PullItems | `GET /reader/api/0/stream/contents/user/-/state/com.google/reading-list`（按时间增量） |
| PushStates | `POST /reader/api/0/edit-tag`（`a=user/-/state/com.google/read` / `starred`，`r=` 为移除） |

**适配目标**：
- **FreshRSS**：`{endpoint}/api/greader.php`，`/accounts/ClientLogin` 前缀需拼 `endpoint + '/api/'`（配置项含协议探测，用户填服务器根地址即可）
- **TTRSS**：需用户启用 `freshapi`（GReader 兼容）插件，端点 `{endpoint}/plugins.local/freshapi/api/greader.php`
- **Miniflux**：原生 API 优先，GReader 客户端可复用为其降级通道（不默认启用）

**FreshRSS 用户前置**：管理后台开启 API 访问 + 设置 API 专用密码。

### 5.3 Tiny Tiny RSS（扩展层，原生 JSON API）

**实现**：自写 TTRSS JSON-RPC 客户端（`internal/sync/ttrss.go`）。

| Flore 操作 | TTRSS op |
|-----------|----------|
| 登录 | `op=login` → session_id（会话式） |
| PullSubscriptions | `op=getCategories` + `op=getFeeds` |
| subscribe / unsubscribe | `op=subscribeToFeed` / `op=unsubscribeFeed` |
| PullItems | `op=getHeadlines` + `op=getArticle`（正文需二次取） |
| PushStates | `op=updateArticle`（field: 0=已读, 2=星标） |

**定位**：TTRSS 用户可选原生通道（功能更全）或 GReader 插件通道（复用 5.2 客户端，零新增代码）。默认推荐 GReader 插件通道以复用代码；原生通道作为 P2 交付。

### 5.4 WebDAV（文件后端）

**依赖**：`github.com/studio-b12/gowebdav`（轻量，MIT）

**文件路径**：`{WebDAV_URL}/flore-reader.json`。

**流程**：FetchPackage（GET，404 视为空包）→ 本地合并 → StorePackage（PUT）。

### 5.5 S3（文件后端）

**依赖**：`github.com/minio/minio-go/v7`（兼容 S3 / MinIO / R2 等对象存储）

**文件路径**：`{Bucket}/{PathPrefix}flore-reader.json`。

**流程**：与 WebDAV 一致（GetObject / PutObject）。

### 5.6 OneDrive（远期评估，P4）

走 Microsoft Graph API（OAuth 设备码授权流），涉及授权回调与 token 刷新，成本高于 WebDAV/S3。**仅评估，不承诺交付**；WebDAV/S3 覆盖国内主流网盘（坚果云、InfiniCLOUD 等均提供 WebDAV）后再决定。

---

## 6. 数据模型变更

### 6.1 Source 新增列

```go
// models/models.go — Source 新增
RemoteOrigin string `json:"remoteOrigin" gorm:"column:remoteOrigin;default:'';index"` // 提供器名（""=本地源）
RemoteID     string `json:"remoteId" gorm:"column:remoteId;default:''"`                // 远端 feed ID（GReader streamID / Miniflux feed_id）
```

- 新增列由 AutoMigrate 自动处理（无需递增 schema 版本号，遵循现有迁移注释口径）
- `RemoteOrigin != ""` 的源：scheduler 跳过本地抓取，Sync Engine 接管
- 版本号递增口径见 §11 拍板项

### 6.2 配置存储（Setting 表，`sync_` 前缀）

```
sync_api_enabled      → "true"            # API 后端总开关
sync_api_provider     → "miniflux"        # miniflux | greader | ttrss
sync_api_endpoint     → "https://..."
sync_api_username     → "..."             # greader / ttrss 需要
sync_api_password     → "..."             # greader / ttrss 需要（API 密码）
sync_api_token        → "..."             # miniflux API Key
sync_file_enabled     → "true"            # 文件后端总开关
sync_file_provider    → "webdav"          # webdav | s3
sync_file_url         → "https://..."     # webdav
sync_file_username    / sync_file_password
sync_file_endpoint / sync_file_region / sync_file_bucket
sync_file_access_key / sync_file_secret_key / sync_file_prefix
sync_window_days      → "30"              # 文件包文章窗口
sync_pull_interval    → "300"             # Pull 周期（秒）
sync_last_pull_at     → "..."             # 增量游标
```

**安全约束**：
- 密码 / token 仅存本地 SQLite，禁止写入任何日志（复用现有 sensitive 日志过滤思路）
- 出站 endpoint 经 `urlpolicy.ValidateURL` 校验，拦截内网地址（防 SSRF 跳板）
- API 后端互斥（单选 provider），文件后端与 API 后端并行

---

## 7. Sync Engine 调度

### 7.1 触发时机

| 时机 | 操作 | 说明 |
|------|------|------|
| 用户连接服务端（首次） | 订阅对齐 Pull | 远端源导入 / 本地源合并，见 §2 |
| 应用启动 | Pull（订阅 + 增量条目） | 后台执行，不阻塞首屏 |
| 定时（默认 5 分钟） | Pull 增量条目 | 替代 remoteOrigin 源的本地轮询 |
| 本地状态变更 | Push（防抖 2s 合并批量） | 标已读/星标/取消 |
| 本地订阅变更 | Push 订阅操作 | 增删源、移动、重命名 |
| 文件后端启用时 | 每日整包 StorePackage | 作为备份快照 |
| 用户手动点击 | 立即同步 | 设置页 + 侧边栏指示器 |

### 7.2 合并逻辑

```
状态合并：LWW
  key = (sourceUrl, link)
  updatedAt 大者保留；相同则任意一条

订阅合并：URL 对齐
  远端有本地无 → 导入（标记 remoteOrigin）
  本地有远端无 → 询问用户（推送远端 or 转为纯本地）
  双方都有 → title/folder 取 LWW，remoteId 以远端为准
```

### 7.3 错误处理

| 错误类型 | 行为 |
|---------|------|
| 后端不可达（网络） | 记录日志，降级离线阅读，下一周期重试 |
| 认证失败（401/403） | 前端同步指示器提示「认证失败，请检查配置」，暂停同步 |
| 速率限制（429） | 指数退避重试最多 3 次后跳过本周期 |
| 文件包合并冲突 | LWW 解决；写回前二次 Fetch 防覆盖他端写入 |

---

## 8. 前端改动

### 8.1 设置页新增「同步」Tab（`components/settings/SettingsSyncTab.tsx`）

```
┌─────────────────────────────────────┐
│ 服务端同步                           │
│                                     │
│ □ 启用 API 服务端同步                │
│   服务类型: [Miniflux ▼]             │
│   服务器地址: [________________]     │
│   API Key:    [________________]     │
│   [测试连接]                         │
│                                     │
│ □ 启用云端存储备份（WebDAV / S3）    │
│   存储类型: [WebDAV ▼]              │
│   URL / Bucket / 凭证 ...            │
│   [测试连接]                         │
│                                     │
│ 同步文章范围: [30 天 ▼]              │
│ 自动同步间隔: [5 分钟 ▼]             │
│ [立即同步]                           │
└─────────────────────────────────────┘
```

遵循用户 UI 偏好：平铺紧凑选项行、左标题右控件、toast 反馈（测试连接结果聚合提示）、无 emoji。

### 8.2 同步状态指示器（侧边栏底部）

- 同步中：轻量加载动画
- 同步完成：静默（无标记，与「健康源零标记」原则一致）
- 同步失败：轻提示（可点击查看错误详情，跳转设置 Tab）

### 8.3 API 层扩展（`utils/api.ts`）

`getSyncConfig / updateSyncConfig / testSyncConnection / triggerSync / getSyncStatus`，统一走现有 api.ts 封装与 Bearer token 体系。

---

## 9. 路由注册（后端）

```
GET    /api/sync/config          # 读同步配置（密码/密钥脱敏返回）
PUT    /api/sync/config          # 写同步配置
POST   /api/sync/test            # 测试连接（按 provider 类型）
POST   /api/sync/trigger         # 手动触发一次同步
GET    /api/sync/status          # 上次同步时间/结果/各后端健康
POST   /api/sync/align           # 首次连接时的订阅对齐（含本地源处置选项）
```

挂入 `RegisterRoutes` 的 `sensitive` 组（写操作走认证）。

---

## 10. 实施路线图

| 阶段 | 内容 | 新增依赖 | 验收 |
|------|------|---------|------|
| **P0 基础设施 + Miniflux** | Provider 接口 / 配置存储 / 设置 Tab / 引擎骨架（手动同步）/ Source 新增列 / Miniflux 原生后端 | `miniflux.app/v2/client` | 连接真实 Miniflux：订阅导入、增量拉文、状态双向同步 |
| **P1 GReader 兼容层** | greader.go 客户端 + FreshRSS 适配 | 无 | 连接 FreshRSS 完成同 P0 验收项 |
| **P2 TTRSS** | 原生 JSON API 客户端（与 GReader 插件通道二选一文档说明） | 无 | 连接 TTRSS 完成同 P0 验收项 |
| **P3 文件后端** | filesync.go + WebDAV / S3 + 整包合并 + 每日备份 | `gowebdav`、`minio-go` | WebDAV/S3 上出现 flore-reader.json，双设备整包恢复一致 |
| **P4 OneDrive 评估** | Graph API 成本评估 | 待定 | 仅评估报告，不承诺实现 |

每个阶段独立交付，含集成测试（用本地 Docker 起真实 Miniflux/FreshRSS 验证）与 tsc / go vet / gofmt 门禁。

**依赖评估备注**：三个新增依赖均轻量、MIT/Apache 许可、活跃维护；minio-go 体积中等（含 S3 全协议），若最终仅用基础 Get/Put，可评估改用裸 HTTP + SigV4 自签（P3 时再定，先按 minio-go 规划）。

---

## 11. 风险与拍板项

| 项 | 说明 | 建议 |
|----|------|------|
| **版本号递增口径** | 表结构变更（Source 新增列）按 AGENTS.md §9.2 应递增 MAJOR；但 0.x 阶段 + 纯增量列（AutoMigrate 无损）递增 MAJOR 偏重 | 倾向按 0.x 惯例递增 MINOR，需你拍板 |
| **GReader ClientLogin 弃用风险** | FreshRSS/TTRSS 插件当前均支持；协议古老但生态依赖深，短期无弃用迹象 | 锁定实现于少量端点，出问题影响面可控 |
| **文件包体积** | 30 天窗口 + 正文 HTML，重订阅用户可能达数 MB | WebDAV/S3 可承受；提供窗口调节；超限再考虑分片 |
| **订阅对齐的本地源处置** | 首次连接时本地独有源推送 or 保留本地 | 做成对齐向导内的用户选择（默认推送） |
| **密钥安全** | token/密码存本地 SQLite | 不入日志、接口脱敏返回（`****` 尾 4 位） |
| **代理兼容** | 现有代理设置是否应用于同步出站请求 | 复用 `BuildFetchHTTPClient`，与抓取同策略 |

---

## 12. 未纳入范围

| 项目 | 原因 |
|------|------|
| 稍后阅读单篇同步 | readlater-sync-design.md 另案 |
| Feedly / Inoreader 托管服务 | OAuth 流程 + 商业 API 限制，等自托管闭环后再评估 |
| 浏览器扩展收集入口 | 依赖稍后阅读方案 |
| 冲突手工解决 UI | LWW 已覆盖，复杂度无收益 |
| 本地抓取模式切换开关 | 连接即切换（远端拉取），不做双模式并存配置 |
