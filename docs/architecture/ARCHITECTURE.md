# ARCHITECTURE.md — RSS Reader 系统架构

> 本文档定义 Reader 阅读器子项目的系统架构、模块边界和数据流向。

## 系统拓扑

```
┌─────────────────────────────────────────────────────────────────┐
│                     Wails Desktop Shell                          │
│  apps/desktop/                                                   │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  React Frontend (apps/web/)                              │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐              │    │
│  │  │ Sidebar  │→│ArticleList│→│  Reader  │              │    │
│  │  │ (源列表)  │  │ (文章列表) │  │ (阅读区)  │              │    │
│  │  └──────────┘  └──────────┘  └──────────┘              │    │
│  │         ↑              ↑              ↑                  │    │
│  │         └──────────────┼──────────────┘                  │    │
│  │                   fetch() API                            │    │
│  └───────────────────────┬─────────────────────────────────┘    │
│                          │ HTTP (127.0.0.1:{port})              │
│  ┌───────────────────────▼─────────────────────────────────┐    │
│  │  Go Backend (server/go/)                                 │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐   │    │
│  │  │  Handler  │→│  Service  │→│  Database (SQLite)    │   │    │
│  │  │ (路由/HTTP)│  │ (业务逻辑) │  │  + FTS5 全文搜索     │   │    │
│  │  └──────────┘  └──────────┘  └──────────────────────┘   │    │
│  │                      │                                    │    │
│  │  ┌───────────────────▼──────────────────────────────┐    │    │
│  │  │  Scheduler (后台调度器) → Fetcher (RSS/Atom 抓取)  │    │    │
│  │  └──────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## 模块边界

### Go 后端分层（server/go/internal/）

```
handler/ ───→ service/ ───→ model/
  │               │
  │               ├── fetcher/    (RSS/Atom 解析)
  │               ├── scheduler/  (定时抓取)
  │               ├── filter/     (过滤规则引擎)
  │               ├── export/     (文章导出)
  │               └── backup/     (数据库备份)
```

| 层 | 职责 | 禁止 |
|----|------|------|
| **handler** | HTTP 请求/响应处理，参数校验 | 直接操作数据库、调用其他 handler |
| **service** | 业务逻辑编排，数据库操作 | 处理 HTTP 请求/响应、引入 Gin 类型 |
| **model** | 数据模型定义 | 包含业务逻辑 |
| **database** | 数据库连接初始化、迁移 | 包含业务逻辑 |

### 前端组件结构（apps/web/src/）

```
App.tsx (状态管理中心)
├── TitleBar       (标题栏)
├── Sidebar        (侧边栏：源列表 + 文件夹)
│   ├── FolderItem
│   └── SourceItem
├── ArticleList    (文章列表)
│   └── ArticleItem
├── Reader         (阅读区)
├── AddSourceModal (添加订阅源)
├── EditSourceModal(编辑订阅源)
├── ImportOPMLModal(OPML 导入)
├── ExportArticlesModal(文章导出)
├── FilterRulesModal(过滤规则管理)
├── SettingsModal  (设置面板)
├── SearchBox      (搜索框)
└── ContextMenu    (右键菜单)
```

**状态管理原则**：所有状态在 `App.tsx` 中集中管理，子组件通过 props 接收数据和回调。

### 桌面壳（apps/desktop/）

```
main.go ──→ app.go
              ├── startBackends()    → 启动 Go 后端子进程
              ├── stopBackends()     → 停止子进程
              ├── findGoBackend()    → 查找后端可执行文件
              ├── findFreePort()     → 动态分配端口
              ├── waitForBackend()   → 轮询健康检查
              └── GetBackendStatus() → 提供后端状态给前端
```

## 数据流向

### 核心流程：文章阅读

```
用户操作 → Sidebar (选择源) → App.tsx 更新 selectedSourceId
       → fetch(`${GO_API}/items?sourceId=...`) → Go Handler
       → Service.GetItems() → GORM 查询 SQLite → 返回 JSON
       → ArticleList 渲染 → 用户点击文章 → Reader 渲染
```

### 后台流程：RSS 抓取

```
Scheduler (每5分钟) → 检查所有 active 源
         → 对到达间隔的源调用 FetchSourceFeed()
         → Fetcher 解析 RSS/Atom XML
         → 按 link 去重 upsert 到 Item 表
         → 应用过滤规则 → 更新 FTS5 索引 → 更新健康状态
```

## 数据库模型

```
Folder ─────── Source ─────────── Item
│              │  ├─ active       │  ├─ isRead
│  id          │  ├─ isPrivate    │  ├─ isStarred
│  name        │  ├─ hideInTimeline│  ├─ isReadLater
│  createdAt   │  ├─ interval     │  ├─ link (唯一)
│              │  ├─ folderId →Folder│  ├─ pubDate
│              │  └─ SourceHealth │  └─ author
│              │     ├─ lastFetchAt
│              │     ├─ lastSuccessAt
│              │     ├─ fetchFailCount
│              │     └─ lastError
│
│  FilterRule
│     ├─ scope: global|source|folder
│     ├─ conditions: []FilterCondition
│     └─ action: markRead|star|readLater
│
│  ReadabilityCache (文章可读性缓存)
│  ItemSearch (FTS5 全文搜索虚拟表)
```

## API 接口

所有 API 以 `/api` 为前缀，返回 JSON。Go 后端运行在 `127.0.0.1:{port}`，端口由桌面壳动态分配。

### 订阅源

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/sources` | 获取所有源（含未读数、健康状态） |
| GET | `/api/sources/:id` | 获取单个源 |
| PUT | `/api/sources/:id` | 更新源（folderId 等） |
| DELETE | `/api/sources/:id` | 删除源 |
| POST | `/api/sources/delete-batch` | 批量删除 |
| POST | `/api/sources/create` | 创建源（name + url + interval） |
| POST | `/api/sources/:id/fetch` | 立即抓取单个源 |
| POST | `/api/sources/fetch-all` | 立即抓取所有源 |
| POST | `/api/sources/:id/read-all` | 标记源下所有文章已读 |

### 文件夹

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/folders` | 获取所有文件夹 |
| POST | `/api/folders` | 创建文件夹 |
| PUT | `/api/folders/:id` | 重命名文件夹 |
| DELETE | `/api/folders/:id` | 删除文件夹 |
| POST | `/api/folders/:id/clear` | 清空文件夹 |
| POST | `/api/folders/:id/fetch` | 抓取文件夹下所有源 |

### 文章

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/items` | 文章列表（支持 sourceId/folderId 筛选） |
| GET | `/api/items/count` | 文章计数 |
| GET | `/api/items/:id` | 单篇文章详情 |
| POST | `/api/items/:id/read` | 标记已读 |
| POST | `/api/items/:id/unread` | 标记未读 |
| POST | `/api/items/:id/star` | 收藏 |
| POST | `/api/items/:id/unstar` | 取消收藏 |
| POST | `/api/items/:id/read-later` | 稍后阅读 |
| POST | `/api/items/:id/unread-later` | 取消稍后阅读 |
| POST | `/api/items/read-all` | 批量标记已读 |
| POST | `/api/items/mark-read` | 按 ID 列表批量标记 |
| GET | `/api/items/search` | 全文搜索 |
| GET | `/api/items/:id/readability` | 可读性视图 |
| POST | `/api/items/export` | 导出文章 |
| POST | `/api/items/:id/apply-filters` | 应用过滤规则 |

### 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/opml/import` | 导入 OPML |
| GET | `/api/opml/export` | 导出 OPML |
| GET | `/api/filter-rules` | 过滤规则列表 |
| POST | `/api/filter-rules` | 创建规则 |
| PUT | `/api/filter-rules/:id` | 更新规则 |
| DELETE | `/api/filter-rules/:id` | 删除规则 |
| GET | `/api/proxy/:id` | 原文代理 |
| GET | `/api/stats/unread` | 未读统计 |
| GET | `/api/database/export` | 数据库备份 |
| POST | `/api/database/restore` | 数据库恢复 |
| GET | `/health` | 健康检查 |

## 重要架构决策

1. **SQLite 单写者模式**：`SetMaxOpenConns(1)`，避免并发写入冲突
2. **WAL 模式**：启用 SQLite WAL 模式，提升并发读取性能
3. **FTS5 全文搜索**：使用 SQLite FTS5 虚拟表，标题和内容分词使用 `porter unicode61`
4. **动态端口分配**：桌面端通过 `net.Listen("127.0.0.1:0")` 分配空闲高位端口，避免端口冲突
5. **纯 Go SQLite 驱动**：使用 `github.com/glebarez/sqlite`（基于 `modernc.org/sqlite`），不依赖 CGO
6. **异步迁移**：数据库迁移（AutoMigrate + FTS5 索引构建）在 HTTP 服务启动后异步执行，降低启动延迟