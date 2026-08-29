# 服务端同步 P1 任务清单（GReader 兼容层 + FreshRSS + 定时 Pull）

> 来源：server-sync-design.md §5.2 / §7（v1，2026-08-29）
> 范围：P1 = GReader 协议客户端（自写，零新增依赖）+ FreshRSS 适配 + 引擎周期调度（P0 遗留）
> 估时口径：S=0.5–1 天，M=2–3 天（单人速率，含自测）
> 约定：每完成一项在状态列打勾并附一行备注；全部完成后对齐「验收总闸」

---

## 零、P1 边界（不做什么）

| 不做 | 理由 |
|------|------|
| TTRSS 原生 JSON API 客户端 | P2 交付 |
| TTRSS freshapi 插件通道验证 | 与 GReader 客户端同一套代码，仅端点前缀不同；实测推迟到 P2 |
| WebDAV / S3 文件后端 | P3 交付 |
| OneDrive | P4 仅评估 |
| 稍后阅读单篇同步 | readlater-sync-design.md 另案 |

---

## 一、任务清单

### T-P1-01 ｜ GReader 客户端：ClientLogin 认证 ｜ 状态：完成（2026-08-29）

- **分层**：后端
- **改动点**：
  - `server/go/internal/sync/greader.go`：GReader 协议 HTTP 客户端（无第三方依赖）：
    - `ClientLogin`：`POST {api}/accounts/ClientLogin`（`Email`/`Passwd` 表单），响应 `Auth=xxx` 行提取 token
    - token 内存缓存，请求头 `Authorization: GoogleLogin auth=<token>`；遇 401 时重新登录一次再重试
    - httpClient 复用 `BuildFetchHTTPClient`（代理同策略）
- **验收**：单测 mock ClientLogin 成功/失败（401 → ErrNotAuthorized）、token 提取。
- **依赖**：T-P0-02（接口类型）
- **估时**：S
- **备注**：token 缓存带互斥锁保护；401 重登录重试已实现并有单测覆盖（TestGReader401ReloginRetry）。

### T-P1-02 ｜ GReader Provider：全方法实现 + FreshRSS 适配 ｜ 状态：完成（2026-08-29）

- **分层**：后端
- **改动点**：`greader.go` 实现 `RemoteProvider` 接口（协议映射见设计文档 §5.2）：
  - `PullSubscriptions`：`GET /reader/api/0/subscription/list?output=json` → `[]RemoteFeed`（`categories[0].label` 为文件夹名）
  - `PushSubscription`：
    - subscribe：`POST /reader/api/0/subscription/quickadd`（`quickadd=feed/<url>`）→ 需要时 `subscription/edit?ac=subscribe` 带 `a=user/-/label/<folder>` 与 `t=` 标题
    - unsubscribe：`POST /reader/api/0/subscription/edit?ac=unsubscribe`（`s=feed/<url>`）
    - rename / move：`subscription/edit?ac=edit`（`t=` / `a=user/-/label/<folder>`）——GReader 协议原生支持，本阶段实现（Miniflux 的 ErrUnsupported 差异保留）
  - `PullItems`：`GET /reader/api/0/stream/contents/user/-/state/com.google/reading-list`（`ot=` 起始时间戳 + `n=` 分页 1000 + `c=` continuation 续拉）→ `[]RemoteItem`
  - `PushStates`：`POST /reader/api/0/edit-tag`（`i=` 条目 id 批量；`a=user/-/state/com.google/read|starred` 添加，`r=` 移除）
  - `HealthCheck`：`GET /reader/api/0/user-info`（200 且 UserId 非空）
  - **FreshRSS 适配**：endpoint 填服务器根地址（如 `https://freshrss.example.com`），客户端拼接 `{root}/api/greader.php/reader/api/0/...` 与 `{root}/api/greader.php/accounts/ClientLogin`
- **验收**：单测（httptest mock GReader API）：登录流、订阅映射、stream 分页（continuation）、edit-tag 状态体格式；认证失败返回 ErrNotAuthorized。
- **依赖**：T-P1-01
- **估时**：M
- **备注**：endpoint 归一化兼容根地址 / `…/api` / 完整 greader.php 三种填法（TestGReaderEndpointNormalization）；PullItems 需保留原始 streamId 故内部走 listSubscriptions。

### T-P1-03 ｜ 引擎接入：buildSyncProvider greader 分支 ｜ 状态：完成（2026-08-29）

- **分层**：后端
- **改动点**：
  - [sync_engine.go](../../server/go/internal/services/sync_engine.go) `buildSyncProvider`：`case rsssync.ProviderGReader` → `NewGReaderProvider(endpoint, username, password, httpClient)`；username/password 为空时报错
  - `sync_api_username` / `sync_api_password` 配置键复用（T-P0-03 已预置）
- **验收**：配置 greader 后 TestConnection 走 GReader HealthCheck；错误信息可读。
- **依赖**：T-P1-02、T-P0-03
- **估时**：S

### T-P1-04 ｜ 定时 Pull 周期调度 ｜ 状态：完成（2026-08-29）

- **分层**：后端
- **改动点**：
  - `SyncEngine` 增加周期调度：
    - 启动时后台执行一次初始 Pull（不阻塞启动）
    - `time.Ticker` 周期检查（复用 `sync_pull_interval` 配置，默认 300 秒），到期执行增量 Pull（订阅对齐 + PullItems，与手动全量同一链路）
    - 同步失败退避：连续失败后间隔翻倍（上限 30 分钟），成功后恢复
    - 已在同步中（手动触发）时跳过本轮
    - `sync_api_enabled` 关闭时挂起（ticker 保留，每轮先检查开关）
  - 引擎启动/停止挂接到 `ReaderService` 生命周期（随 scheduler 关闭）
- **验收**：单测：周期到期触发、间隔内不重复触发、失败退避、关闭开关挂起。
- **依赖**：T-P0-05、T-P1-03
- **估时**：M
- **备注**：backend.go 启动时 StartPeriodicPull、关闭时 StopPeriodicPull；`sync_pull_interval <= 0` 表示仅手动（间隔取极大值永不到期）。

### T-P1-05 ｜ 前端设置 Tab 支持 GReader ｜ 状态：完成（2026-08-29）

- **分层**：前端
- **改动点**：
  - [SettingsSyncTab.tsx](../../apps/web/src/components/settings/SettingsSyncTab.tsx)：Provider 下拉增加 `{ value: 'greader', label: 'FreshRSS / GReader' }`
  - 选中 greader 时：endpoint 提示文案改为「FreshRSS 服务器根地址」；凭据区显示「用户名 + API 专用密码」（隐藏 Token 输入框）；Miniflux 时反之（Token 输入框，隐藏密码）
  - 说明文案：FreshRSS 用户需在管理后台开启 API 访问并设置 API 专用密码
- **验收**：切换 provider 时表单字段联动正确；保存后 TestConnection 正常。
- **依赖**：T-P1-03、T-P0-10
- **估时**：S
- **备注**：联动采用「用户名/密码恒显（Miniflux 时用户名标注 Token 认证可留空）+ Token 输入框仅 Miniflux 显示」方案，避免切换 provider 时清空已填凭据。顺带修复 P0 遗留：buildPayload 键名由 camelCase 改为后端实际识别的 `sync_*`（此前保存不生效）；pullInterval 前后端单位统一（表单分钟 × 60 存秒）；后端校验放宽允许 0 值（windowDays=全部历史、pullInterval=仅手动）。

### T-P1-06 ｜ 单测补齐（GReader + 定时调度） ｜ 状态：完成（2026-08-29）

- **分层**：后端
- **改动点**：
  - `internal/sync/greader_test.go`：httptest mock GReader API
  - `sync_engine_test.go`：周期调度用例（可注入短间隔）
- **验收**：`go test ./internal/sync/ ./internal/services/ -count=1` 全绿。
- **依赖**：T-P1-02、T-P1-04
- **估时**：M
- **备注**：9 个 GReader 用例（登录、401 重试、endpoint 归一化、订阅/条目/状态/增退订映射）+ 周期调度 3 用例（节流、退避、开关挂起）。

### T-P1-07 ｜ 集成验证（真实 FreshRSS） ｜ 状态：待办

- **分层**：联调
- **改动点**：无代码改动；本地 Docker 启动 FreshRSS + SQLite：
  - 连接配置 → TestConnection
  - 订阅同步（远端导入 / 本地合并 / 本地独有推送）
  - 增量拉文（30 天窗口）
  - 状态双向同步（本地标读 → FreshRSS 网页端可见；网页端标读 → 本地 Pull 可见）
- **验收**：同 P0 验收项，目标服务端为 FreshRSS。
- **依赖**：T-P1-05、T-P1-06
- **估时**：M

---

## 二、验收总闸

| 项 | 判据 |
|----|------|
| 单测全绿 | `go test ./...` 无失败 |
| tsc / vet / gofmt | `tsc --noEmit`、`go vet ./...`、`gofmt -l` 无输出 |
| FreshRSS 端到端 | T-P1-07 四步全过 |
| P0 回归 | Miniflux 手动同步 + 状态推送行为不变 |
