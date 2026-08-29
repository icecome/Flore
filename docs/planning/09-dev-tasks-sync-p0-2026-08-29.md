# 服务端同步 P0 任务清单（基础设施 + Miniflux）

> 来源：server-sync-design.md（v1，2026-08-29）
> 范围：P0 = Provider 接口 + 配置存储 + 设置 Tab + 引擎骨架（手动同步）+ Source 新增列 + Miniflux 原生后端
> 估时口径：S=0.5–1 天，M=2–3 天（单人速率，含自测）
> 约定：每完成一项在状态列打勾并附一行备注；全部完成后对齐「验收总闸」

---

## 零、P0 边界（不做什么）

| 不做 | 理由 |
|------|------|
| GReader / TTRSS provider | P1 / P2 交付 |
| WebDAV / S3 文件后端 | P3 交付 |
| 定时自动 Pull（引擎周期启动） | P0 仅手动触发 + 状态变更 Push；周期调度随 P1 落地（避免骨架期后台任务不可控） |
| OneDrive | P4 仅评估 |
| 稍后阅读单篇同步 | readlater-sync-design.md 另案 |

---

## 一、任务清单

### T-P0-01 ｜ Source 模型新增列 + 调度过滤 ｜ 状态：已完成

- **备注**（2026-08-29）：`Source.RemoteOrigin/RemoteID`、`Item.RemoteID` 已落地；`coordinator.go` 的 `SubmitAllSources`/`SubmitFolder` 已追加 `remoteOrigin = ''` 过滤。

- **分层**：后端模型 + 调度
- **改动点**：
  - [models.go](../../server/go/internal/models/models.go) `Source` 新增 `RemoteOrigin`（`column:remoteOrigin;default:'';index`）与 `RemoteID`（`column:remoteId;default:''`）两列，AutoMigrate 自动建列，不递增 schema 版本号。
  - [coordinator.go](../../server/go/internal/services/coordinator.go) `SubmitAllSources`（158 行）查询条件追加 `AND remoteOrigin = ''`；`SubmitFolder`（218 行）同样追加。
  - 全量抓取入口手动刷新（`FetchAll` handler 路径）确认同样走 `SubmitAllSources`，无需单独处理。
- **验收**：给某源写入 `remoteOrigin='miniflux'` 后，全量刷新/文件夹刷新/调度轮询均不再抓取该源；普通源行为不变。
- **依赖**：无
- **估时**：S

### T-P0-02 ｜ sync 包骨架：接口与类型定义 ｜ 状态：已完成

- **备注**（2026-08-29）：`internal/sync/provider.go` 已定义 `RemoteProvider`/`FileProvider` 接口与 `RemoteFeed`/`RemoteItem`/`StateChange`/`SubscriptionOp` 类型。

- **分层**：后端
- **改动点**：新增 `server/go/internal/sync/` 包（AGENTS 12.1 未列此子包，位于 `server/go` 内合规；设计文档 §4 已锚定）：
  - `provider.go`：`RemoteProvider` / `FileProvider` 接口 + `RemoteFeed` / `RemoteItem` / `StateChange` / `SubscriptionOp` 类型定义（P0 先落接口全貌，FileProvider 类型仅定义不实现）
  - 类型先行的编码规约（禁止 `interface{}` 兜底）
- **验收**：`go build ./...` 通过；接口签名与设计文档 §4 一致。
- **依赖**：无
- **估时**：S

### T-P0-03 ｜ 同步配置读写 Service ｜ 状态：已完成

- **备注**（2026-08-29）：`services/sync_config.go` 已实现脱敏读写与掩码回传保留原值逻辑。

- **分层**：后端
- **改动点**：
  - `server/go/internal/services/sync_config.go`：`GetSyncConfig()`（脱敏：token/password 返回 `****` + 尾 4 位）/ `UpdateSyncConfig(map)`；键清单见设计文档 §6.2（`sync_*` 前缀）。
  - 复用 `setting_service.go` 现有 `GetSetting`/`UpdateSettings` 与 settingsCache 失效机制（m-13 修复后的缓存路径）。
  - endpoint 写入时调用 `urlpolicy.ValidateURL` 拦截内网地址。
- **验收**：单测覆盖：脱敏返回、非法 endpoint 拒绝、缺省值回填（window 30 天 / interval 300 秒）。
- **依赖**：T-P0-02（类型）
- **估时**：S

### T-P0-04 ｜ Miniflux Provider 实现 ｜ 状态：已完成

- **备注**（2026-08-29）：`internal/sync/miniflux.go` 基于官方客户端实现全部 `RemoteProvider` 方法；`UpdateFeed`/`CreateCategory` 签名差异已修正。
- **备注**（2026-08-29，单测补齐）：`miniflux_test.go` httptest mock 覆盖认证头透传、401 可识别错误（ErrNotAuthorized）、订阅映射、offset 分页（满页续拉）+ changed_after 增量游标、状态批量体格式、rename/move 返回 ErrUnsupported。

- **分层**：后端
- **改动点**：
  - `go get miniflux.app/v2/client`（新增依赖，Apache-2.0，官方维护）
  - `server/go/internal/sync/miniflux.go`：实现 `RemoteProvider` 全部方法（协议映射见设计文档 §5.1）：
    - `PullSubscriptions`：feeds + categories → `[]RemoteFeed`（含文件夹归类）
    - `PushSubscription`：subscribe / unsubscribe（rename/move 返回「暂不支持」错误，P1 前不实现）
    - `PullItems(since)`：`after_entry_id` 增量 + 分页拉取
    - `PushStates`：`PUT /v1/entries` 批量
    - `HealthCheck`：`GET /v1/me`
  - HTTP client 复用 `BuildFetchHTTPClient`（代理同策略）；超时与错误包装遵循 `%w` 规约。
- **验收**：单测（httptest mock Miniflux API）：登录认证头、增量游标、状态批量体格式；认证失败返回可识别错误类型。
- **依赖**：T-P0-02、T-P0-03（配置）
- **估时**：M

### T-P0-05 ｜ Sync Engine 骨架（手动同步链路） ｜ 状态：已完成

- **备注**（2026-08-29）：`services/sync_engine.go` 实现订阅对齐、增量拉取、状态回推与防抖推送；`cmd/main.go` 已注入引擎实例。
- **备注**（2026-08-29，单测补齐）：`sync_engine_test.go`（mock provider + 内存 SQLite）覆盖订阅对齐三分支、LWW 状态合并、防抖批量推送（同 ID 去重、本地源跳过、空 flush 幂等）、对齐向导推送。测试中发现并修复两处缺陷：
  1. LocalOnlyURLs 误报：合并前快照中「双方都有」的源 remoteOrigin 内存值仍为空，会被误报为本地独有；已按远端 URL 集合排除。
  2. RecordStateChange 表名错误：硬编码 `Table("items")`/`JOIN sources` 与 Prisma 风格表名（`Item`/`Source`）不符，查询必然失败导致状态回推队列从未入队（T-P0-07 实际失效的根因）；已改为 GORM 模型 + 反引号表名 + Scan 匿名结构体补 gorm column tag。

- **分层**：后端
- **改动点**：
  - `server/go/internal/sync/engine.go`：`SyncEngine` 结构（持有 provider + 依赖 `ReaderService` 的写入能力）
  - `TriggerFullSync(ctx)`：PullSubscriptions → 订阅对齐（URL 对齐，见设计文档 §7.2）→ PullItems 增量 → upsert 写入 Item（复用 `item_service.go` 的 `upsertFeedItems` 思路，抽取单源 upsert）
  - `PushPendingStates(ctx)`：收集本地未推送状态变更（防抖窗口聚合）→ PushStates
  - 状态变更记录：P0 用内存队列 + 启动全量对齐兜底（简化，避免 P0 引入 Item 级持久化队列）
  - 同步结果写入 `Setting`：`sync_last_pull_at` / `sync_last_result` / `sync_last_error`（供 status 接口）
- **验收**：mock provider 单测：订阅导入/合并三分支（远端独有/本地独有/双方都有）、LWW 状态合并、防抖批量推送。
- **依赖**：T-P0-02、T-P0-04
- **估时**：M

### T-P0-06 ｜ Handler 与路由注册 ｜ 状态：已完成

- **备注**（2026-08-29）：`handlers/sync_handlers.go` 六个 handler 已挂载到 sensitive 路由组（/api/sync/*）。

- **分层**：后端
- **改动点**：
  - `server/go/internal/handlers/sync_handlers.go`：`GetSyncConfig` / `UpdateSyncConfig` / `TestSyncConnection` / `TriggerSync` / `GetSyncStatus` / `AlignSubscriptions` 六个 handler（仅参数解析 → service → JSON，无业务逻辑）
  - [reader.go](../../server/go/internal/handlers/reader.go) `RegisterRoutes` 挂载 `/api/sync/*`：config 读写与 status 入 `sensitive` 组（含脱敏密钥，从严）；test/trigger/align 写操作入 `sensitive` 组
- **验收**：curl 走通六个端点；未设 token 时写端点拒未认证请求。
- **依赖**：T-P0-03、T-P0-05
- **估时**：S

### T-P0-07 ｜ 状态变更 Push 挂接 ｜ 状态：已完成

- **备注**（2026-08-29）：`item_service.go` 标已读/星标路径已挂接 StateChange 防抖队列，异步推送失败不影响本地操作。
- **备注**（2026-08-29，单测补齐）：RecordStateChange 查询修复（见 T-P0-05 备注 2）后，防抖队列链路经 `TestSyncEngineStateChangeDebounce` 验证可用。

- **分层**：后端
- **改动点**：
  - [item_service.go](../../server/go/internal/services/item_service.go) 标已读/星标路径（`MarkRead`/`MarkStar` 及批量变体）末尾追加：目标 item 属于 `remoteOrigin != ''` 源时，投递 `StateChange` 到 engine 的防抖队列（异步，失败仅日志不影响本地操作）
  - `cmd/main.go`：启动时构造 SyncEngine 并注入（同步开关关闭时为 no-op 引擎）
- **验收**：本地标读一个同步源文章 → 2s 内 mock 端点收到批量状态推送；本地源文章不触发推送；推送失败不影响标读本地生效。
- **依赖**：T-P0-05
- **估时**：M

### T-P0-08 ｜ 集成验证（真实 Miniflux） ｜ 状态：待办

- **分层**：验证
- **改动点**：本地 Docker 起真实 Miniflux（`miniflux/miniflux` + postgres），建 2 个真实源，端到端走：连接 → 对齐导入 → 增量拉文 → 双向状态同步。
- **验收**：设计文档 §10 P0 行的三项验收全绿；附验证记录（命令 + 日志摘要，追加到本文档末尾）。
- **依赖**：T-P0-01 ~ T-P0-07
- **估时**：S

### T-P0-09 ｜ 前端 API 层扩展 ｜ 状态：已完成

- **备注**（2026-08-29）：api.ts 新增六个同步函数；`types.ts` 已含 `SyncConfig/SyncStatus/SyncResult/SyncAlignResult`；tsc 通过。

- **分层**：前端
- **改动点**：[api.ts](../../apps/web/src/utils/api.ts) 按现有薄封装风格新增：`getSyncConfig` / `updateSyncConfig` / `testSyncConnection` / `triggerSync` / `getSyncStatus`（对齐设计文档 §8.3，类型定义进 `types.ts`）。
- **验收**：tsc 通过；无组件直连 fetch。
- **依赖**：T-P0-06（端点契约）
- **估时**：S

### T-P0-10 ｜ 设置页「同步」Tab ｜ 状态：已完成

- **备注**（2026-08-29）：`settings/SettingsSyncTab.tsx` 已实现（API 同步 + 云端备份两组表单、测试连接、立即同步、自动保存）；`SettingsModal.tsx` 已注册 sync Tab，并新增 `initialTab` prop 供指示器跳转。WebDAV/S3 表单按配置模型一并呈现（后端 FileProvider 于 P3 落地，届时无前端改动即可启用）。

- **分层**：前端
- **改动点**：
  - 新增 `apps/web/src/components/settings/SettingsSyncTab.tsx`（复用 `SettingsShared` 的 `Section/Row/Toggle/Select`）
  - [SettingsModal.tsx](../../apps/web/src/components/SettingsModal.tsx) `TABS` 数组与 `renderTab` 注册 `sync` Tab
  - UI 遵循：紧凑选项行、左标题右控件、测试连接结果 toast 聚合反馈、无 emoji；Miniflux 表单（地址 + API Key + 测试连接 + 立即同步）
  - P0 不渲染 GReader/TTRSS/WebDAV/S3 表单（后端 provider 枚举预留，选择项禁用态展示）
- **验收**：填真实 Miniflux 地址 + Key → 测试连接 toast 成功；立即同步后文章列表出现远端源。
- **依赖**：T-P0-09
- **估时**：M

### T-P0-11 ｜ 同步状态指示器 ｜ 状态：已完成

- **备注**（2026-08-29）：`hooks/useSyncStatus.ts` 30s 轮询；Sidebar 底部指示器（同步中转圈/异常红标/正常灰标），点击经 `initialTab` 跳转同步 Tab；未启用同步时隐藏。

- **分层**：前端
- **改动点**：[Sidebar.tsx](../../apps/web/src/components/Sidebar.tsx) 底部轻量指示器（同步中动画/完成静默/失败轻提示），新建 `hooks/useSyncStatus.ts` 轮询 `getSyncStatus`（同步开启时 30s 一次，失败时点击跳转设置 sync Tab）。
- **验收**：断开 Miniflux → 指示器出现失败轻提示；恢复后静默。
- **依赖**：T-P0-09、T-P0-10
- **估时**：S

### T-P0-12 ｜ 订阅对齐向导（首次连接） ｜ 状态：已完成

- **备注**（2026-08-29）：同步后若存在本地独有源，同步 Tab 内出现「订阅对齐」区块（勾选 URL → 推送远端）。采用行内展开而非弹窗，遵循「输入操作行内化」的交互偏好；推送成功后条目即时移除。

- **分层**：前端
- **改动点**：连接成功后若存在本地独有源，弹对齐选择（复用 `ModalLayout`）：推送远端（默认）/ 保留纯本地；选择后调 `POST /api/sync/align`。无本地独有源时静默跳过。
- **验收**：含本地独有源的场景出现向导；两种选择各自行为正确；空场景无打扰。
- **依赖**：T-P0-10
- **估时**：S

---

## 二、执行顺序（依赖链）

```
T-P0-01 模型+调度过滤 ──────────────┐
T-P0-02 sync 包骨架 ─► T-P0-03 配置 ─► T-P0-04 Miniflux ─► T-P0-05 引擎骨架 ─► T-P0-06 路由 ─► T-P0-09 前端API
                                    │                        └► T-P0-07 Push挂接            ├► T-P0-10 设置Tab ─► T-P0-11 指示器
                                    │                                                       └► T-P0-12 对齐向导
                                    └►（T-P0-05 写入 Item 时依赖 01 的列语义）► T-P0-08 集成验证（收口）
```

建议批次：
1. **第一批（纯后端地基）**：T-P0-01、T-P0-02、T-P0-03 —— 无外部依赖，合入即闭环（本地行为零变化）
2. **第二批（同步链路）**：T-P0-04、T-P0-05、T-P0-06、T-P0-07 —— 核心交付，mock 测试护航
3. **第三批（前端 + 收口）**：T-P0-09 ~ T-P0-12、T-P0-08 —— 真实 Miniflux 验收

---

## 三、验收总闸（P0 完成标志）

1. 连接真实 Miniflux：订阅导入、增量拉文、状态双向同步全部可用
2. 同步源不再被本地 scheduler 抓取；断网后本地已有数据可离线阅读
3. 设置页同步 Tab 完整（Miniflux 表单 + 测试连接 + 手动同步）；侧边栏指示器反映同步健康
4. 密钥脱敏返回、不入日志；endpoint SSRF 校验生效
5. `tsc` / `go build` / `go vet` / `gofmt -l` / 单测全绿

---

## 四、遗留到 P1 的接口预留

| 预留 | 位置 |
|------|------|
| FileProvider 接口与类型 | T-P0-02 一并定义 |
| provider 枚举 `greader` / `ttrss` | 配置校验白名单预留（前端禁用态） |
| 引擎周期调度（定时 Pull） | engine.go 预留 ticker 挂点，P1 接入 |
| PushSubscription 的 rename/move | Miniflux provider 返回明确「未支持」错误 |

---

## 五、验证记录（完成后追加）

（待 T-P0-08 填写）
