# ARCHITECTURE.md — Flore (RSS Reader) 系统架构

> 本文档定义 Flore 阅读器的系统架构、模块边界和数据流向，与代码现状保持同步。
> 最后更新：2026-09-17（架构审查修复后重写）。

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
│  │                   fetch() API（utils/api.ts 统一入口）      │    │
│  └───────────────────────┬─────────────────────────────────┘    │
│                          │ HTTP (127.0.0.1:{port})              │
│  ┌───────────────────────▼─────────────────────────────────┐    │
│  │  Go Backend (server/go/)                                 │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────────┐   │    │
│  │  │  Handler  │→│  Service  │→│  Database (SQLite)    │   │    │
│  │  │ (路由/HTTP)│  │ (业务逻辑) │  │  + FTS5 全文搜索     │   │    │
│  │  └──────────┘  └────┬─────┘  └──────────────────────┘   │    │
│  │                     │                                     │    │
│  │  ┌──────────────────▼──────────────────────────────┐    │    │
│  │  │ Scheduler → FetchCoordinator → Fetcher (RSS/Atom) │    │    │
│  │  │ SyncEngine → internal/sync (GReader/Miniflux)     │    │    │
│  │  └──────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## 模块边界

### Go 后端分层（server/go/）

```
backend/ ──── 启动组装层：日志、数据库初始化、gin 引擎、服务编排
  │
  ▼
internal/
  ├── handlers/  ──→ services/ ──→ models/
  │                    │              ▲
  │                    ├── database/ ─┘（连接 + 版本化迁移）
  │                    └── sync/（远端同步协议客户端，端口适配器）
  └── logging/   日志轮转 + 敏感信息脱敏（被 backend 使用）
```

依赖方向严格单向，无循环：`backend → handlers → services → {models, database, sync}`；`sync` 仅依赖标准库与第三方协议库，不反向依赖任何内部包。

| 层 | 职责 | 禁止 |
|----|------|------|
| **backend** | 启动组装：.env 加载、日志初始化、迁移、路由注册、优雅关闭 | 业务逻辑 |
| **handler** | HTTP 请求/响应处理，参数校验 | 直接操作数据库、调用其他 handler、业务逻辑 |
| **service** | 业务逻辑编排，数据库操作（含代理内容处理引擎） | 处理 HTTP 请求/响应、引入 Gin 类型 |
| **model** | 数据模型定义 | 包含业务逻辑 |
| **database** | 连接初始化、AutoMigrate + 版本化迁移 | 业务逻辑 |
| **sync** | GReader/Miniflux 协议客户端（RemoteProvider 接口） | 依赖内部业务包 |

#### handlers 文件清单

| 文件 | 职责 |
|------|------|
| `reader.go` | 路由注册总表、Bearer 认证中间件（constant-time 比较）、IP 限流 |
| `source_handlers.go` | 订阅源 CRUD |
| `item_handlers.go` | 文章列表/状态标记/导出 |
| `proxy_handlers.go` | 原文/图片/CSS/favicon 代理（内容处理调 `services/proxy_content.go`） |
| `db_handlers.go` | 数据库导出/导入/备份管理 |
| `sync_handlers.go` | 远端同步配置与触发 |
| `security.go` | CSRF 防护、CORS 来源校验、代理跨域头 |
| `diagnostic.go` | 诊断包生成与下载 |

#### services 文件清单

| 文件 | 职责 |
|------|------|
| `reader.go` | ReaderService 聚合根：注入式构造（`NewReaderServiceWithDB`）、FTS5 批量索引 worker |
| `source_service.go` / `item_service.go` / `folder_service.go` | 源/文章/文件夹 CRUD 与查询 |
| `filter.go` | 过滤规则引擎（scope 匹配 + 条件匹配 + 动作执行） |
| `fetcher.go` | RSS/Atom 抓取与解析、增量 304 协商 |
| `scheduler.go` | 固定周期全量轮询调度 |
| `coordinator.go` | 抓取协调器：所有抓取入口的唯一调度权威（去重 + 并发控制 + 退避） |
| `readability.go` | 全文提取 + 原文/图片/CSS 抓取 |
| `proxy_content.go` | 代理内容处理引擎：CSP meta 剥离、base 注入、CSS url() 重写、frame 预检缓存 |
| `export.go` | 文章导出 |
| `backup.go` / `backup_restore.go` | 压缩备份与恢复 |
| `setting_service.go` | Setting 表键值读写与缓存 |
| `sync_engine.go` / `sync_config.go` | 同步引擎（订阅对齐/增量拉取/状态回推）与配置 |
| `urlpolicy.go` | SSRF 防护 URL 校验 |

#### internal/sync 文件清单

| 文件 | 职责 |
|------|------|
| `provider.go` | `RemoteProvider` 接口定义 |
| `greader.go` | Google Reader API 协议客户端 |
| `miniflux.go` | Miniflux API 协议客户端 |

### 前端组件结构（apps/web/src/）

```
App.tsx (状态管理中心 + 单一 fetchersRef 数据刷新)
├── TitleBar / UpdateBadge / ToastContainer
├── Sidebar (源列表 + 文件夹)
├── ArticleList (文章列表，无限滚动/多选/搜索)
├── Reader + ReaderToolbar (阅读区，RSS/Readability/iframe 三模式)
├── 各功能弹窗 (AddSource/EditSource/MoveSource/Rename/ConfirmDialog...)
└── SettingsModal (纯 tab 分发)
    └── settings/ 子目录（10 个 tab 组件 + 共享组件 + 备份/规则表单）
```

**状态管理原则**：所有状态在 `App.tsx` 集中管理，通过 props 向下传递；组件禁止直接调用 API，统一经 `src/utils/api.ts`；数据获取逻辑封装在 `src/hooks/`（useSourcesData / useItemsData / useFetchOrchestration 等 9 个 hook）。

### 桌面壳（apps/desktop/）

```
main.go ──→ app.go（Wails 绑定：API 代理、托盘、窗口）
         ──→ backend.go（子进程生命周期）
              ├── 启动：FLORE_BACKEND_PATH 或自身二进制 --backend、PORT=0 动态端口、端口文件、API token
              ├── 健康检查：轮询 /health、超时/提前退出处理、Job Object 防孤儿进程
              └── 停止：/api/shutdown 优雅关闭 → 超时 Kill
```

## 数据流向

### 核心流程：文章阅读

```
用户操作 → Sidebar (选择源) → App.tsx 更新 selectedSourceId
       → listItems() → Go Handler → Service.GetItems() → GORM → SQLite
       → ArticleList 渲染 → 用户点击文章 → Reader 渲染
```

### 后台流程：RSS 抓取

```
Scheduler (5 分钟周期) → FetchCoordinator.SubmitAllSources
  → 跳过退避期僵尸源 → Fetcher 解析 RSS/Atom（304 协商增量）
  → 按 link+sourceId 去重 upsert → indexChan 批量 FTS5 索引
  → 应用过滤规则 → 更新健康状态与未读数缓存
```

### 同步流程：远端对齐

```
SyncEngine (周期 Pull) → internal/sync provider (GReader/Miniflux)
  → 订阅对齐（align）→ 增量拉取文章 → 本地状态回推（read/star，去抖合并）
```

## 数据库模型

```
Folder 1→N Source 1→N Item
Source 1→1 SourceHealth（抓取健康 + 304 缓存头 + 退避）
FilterRule（scope: global|source|folder，conditions JSON，action: markRead|star|readLater）
Setting（键值对：schema_version、同步配置、代理配置等）
ReadabilityCache（全文提取缓存）
ItemSearch（FTS5 虚拟表，porter unicode61 分词）
```

- `Item.link + sourceId` 复合唯一索引；`pubDate`、`isRead` 等常用查询字段有索引
- 版本化迁移：`Setting.schema_version` 记录版本，迁移幂等（v5 用 `typeof(pubDate)='text'` 条件）

## API 接口

所有 API 以 `/api` 为前缀，返回 JSON。**标注 🔒 的路由属敏感组**：`FLORE_API_TOKEN` 非空时需 Bearer Token（constant-time 比较）；token 为空（本地默认）时全开放。全局限流：每 IP 每 60 秒 120 次（回环地址豁免）。

### 订阅源

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/sources` | 所有源（含未读数、健康状态） |
| GET | `/api/sources/:id` | 单个源（不加载健康数据） |
| POST | `/api/sources/create` | 创建源 |
| PUT | `/api/sources/:id` 🔒 | 更新源（部分字段） |
| DELETE | `/api/sources/:id` 🔒 | 删除源（清理空文件夹） |
| DELETE | `/api/sources/delete-batch` 🔒 | 批量删除 |
| POST | `/api/sources/:id/fetch` | 立即抓取单源 |
| POST | `/api/sources/fetch-all` | 抓取全部源 |
| GET | `/api/sources/fetch-status` | 抓取进度状态 |
| POST | `/api/sources/:id/read-all` | 源下文章全部已读 |

### 文件夹

| 方法 | 路径 | 说明 |
|------|------|------|
| GET / POST | `/api/folders` | 列表 / 创建 |
| PUT | `/api/folders/:id` 🔒 | 重命名 |
| DELETE | `/api/folders/:id` 🔒 | 删除（源移出至根、子文件夹提升） |
| POST | `/api/folders/:id/clear` | 清空（源移出并删除空文件夹） |
| POST | `/api/folders/:id/fetch` | 抓取文件夹下所有源 |

### 文章

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/items` | 列表（sourceId/folderId/未读/收藏/稍后读/私密筛选 + newest/oldest 排序 + 分页） |
| GET | `/api/items/count` | 计数 |
| GET | `/api/items/:id` | 详情 |
| POST | `/api/items/:id/read` `/unread` `/star` `/unstar` `/read-later` `/unread-later` | 状态标记 |
| POST | `/api/items/read-all` | 全部已读（可按源/文件夹限定） |
| POST | `/api/items/mark-read` | 按 ID 列表批量标记 |
| POST | `/api/items/read-later/clear` | 清空稍后读（可限定范围） |
| GET | `/api/items/search` | FTS5 全文搜索 |
| POST | `/api/items/:id/index-search` | 单篇重建索引 |
| POST | `/api/items/index-search/rebuild` | 全量重建索引 |
| GET | `/api/items/:id/readability` | 全文模式内容 |
| POST | `/api/items/export` | 导出文章 |
| GET | `/api/items/:id/export.md` | 导出单篇 Markdown |
| POST | `/api/items/:id/apply-filters` | 手动应用过滤规则 |

### 过滤规则

| 方法 | 路径 | 说明 |
|------|------|------|
| GET / POST | `/api/filter-rules` | 列表 / 创建 |
| PUT / DELETE | `/api/filter-rules/:id` 🔒 | 更新 / 删除 |
| POST | `/api/filter-rules/:id/test` | 用最近 200 篇文章试跑规则 |

### 远端同步（全部 🔒）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET / PUT | `/api/sync/config` | 同步配置（含密钥，读写均敏感组） |
| POST | `/api/sync/test` / `/api/sync/test-file` | 连接测试 |
| POST | `/api/sync/trigger` | 手动触发同步 |
| GET | `/api/sync/status` | 同步状态 |
| POST | `/api/sync/align` | 订阅对齐 |

### 原文代理（只读透传，非敏感组）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/proxy/check/:id` | iframe 嵌入预检（XFO / CSP frame-ancestors，结果缓存） |
| GET | `/api/proxy/:id` | 原文最小代理（剥 CSP meta + 注入 base） |
| GET | `/api/image-proxy` | 图片代理（Referer 绕防盗链，类型白名单拒绝 SVG） |
| GET | `/api/css-proxy` | CSS 代理（url() 重写为图片代理地址） |
| GET | `/api/favicon-proxy` / `/api/favicon-direct` | 第三方图标服务 / 源站直抓 favicon |

### 数据管理（除 info 外全部 🔒）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/database/export` 🔒 | 数据库备份导出 |
| POST | `/api/database/restore` 🔒 | 数据库恢复 |
| GET | `/api/database/info` | 数据库文件信息 |
| POST | `/api/database/vacuum` 🔒 | VACUUM 压缩 |
| GET/POST/DELETE | `/api/backups*` 🔒 | 备份列表/创建/删除/清理/下载/内容/恢复/局部恢复/导入校验（10 条子路由） |
| GET / POST | `/api/cache/stats` / `/api/cache/clear-readability` | 可读性缓存统计/清理 |
| POST | `/api/cache/rebuild-search` | 重建搜索索引 |
| POST | `/api/articles/cleanup` | 文章留存清理 |

### 设置 / 诊断 / 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| GET / PUT | `/api/settings` 🔒 | 全量键值读写（含敏感明文） |
| GET | `/api/settings/:key` 🔒 | 单 key 读取 |
| GET | `/api/diagnostic/items` | 诊断数据 |
| POST | `/api/diagnostic/generate` 🔒 | 生成诊断包 |
| GET | `/api/diagnostic/:path/download` 🔒 | 下载诊断包 |
| GET | `/api/version` | 版本号（构建时注入） |
| GET | `/api/stats/unread` | 未读统计 |
| POST | `/api/opml/import` 🔒 / GET `/api/opml/export` | OPML 导入导出 |
| POST | `/api/shutdown` 🔒 | 优雅关闭（桌面壳退出用） |
| GET | `/health` | 健康检查（无前缀） |

## 重要架构决策

1. **SQLite 单写者模式**：`SetMaxOpenConns(1)`，避免并发写入冲突；WAL 模式提升并发读
2. **FTS5 全文搜索**：原生 SQL 虚拟表，`porter unicode61` 分词，批量索引事务合并写入
3. **动态端口分配**：`net.Listen("127.0.0.1:0")` 消除 TOCTOU，端口写入 `FLORE_PORT_FILE`；默认端口常量 `handlers.DefaultPort`（3002）单点定义
4. **纯 Go SQLite 驱动**：`github.com/glebarez/sqlite`，无 CGO
5. **异步迁移**：AutoMigrate + FTS5 构建在 HTTP 服务启动后异步执行；版本化迁移记录于 `Setting.schema_version`
6. **抓取协调器**：所有抓取入口（手动/调度/托盘）统一走 `FetchCoordinator.Submit`，去重、并发上限、失败退避集中控制
7. **同步端口适配器**：`internal/sync` 实现 `RemoteProvider` 接口（GReader/Miniflux），SyncEngine 每次同步重建 provider，配置变更即时生效
8. **纵深安全**：Bearer Token（constant-time）敏感路由组、CSRF 拦截、CORS 本地来源校验、SSRF URL 校验（urlpolicy）、代理类型白名单 + 尺寸上限 + 域名正则防注入、IP 限流（回环豁免）、日志脱敏（internal/logging）
9. **注入式测试构造**：`NewReaderServiceWithDB(db)` 支持注入内存/临时 SQLite 且不启动后台 goroutine；services 层表驱动测试覆盖 filter/item/source/folder 核心路径
10. **版本注入链路**：根 `package.json` version → `sync-version.mjs` 注入 `version.go` + `wails.json` → `/api/version` → 前端关于页
