# Flore 项目长期记忆

## 安全约定（2026-08-03 审查修复后确立，改安全相关代码前必读）

### 认证与 CSRF（handlers/security.go）
- **CSRF 防护**：后端全局注册 `handlers.CSRFProtection()`（在 CORS 之后）；非 GET/HEAD 请求若带 Origin 必须满足"本地源 + `X-Requested-With: XMLHttpRequest`"，Origin:null 一律拒绝；无 Origin 的非浏览器客户端放行
- **前端约定**：所有写请求必须经 `fetchData.ts` 的 `withTimeout`（自动注入 X-Requested-With）；禁止绕过它直接 fetch
- **CORS**：`AllowOriginFunc` 使用 `handlers.IsLocalOrigin`（拒绝 null/外网源）；`AllowHeaders` 含 X-Requested-With
- **代理端点**：只读透传（image/css/favicon）用 `setProxyCORS` 反射本地源，禁止再写 `ACAO:*`
- **新增只读接口注意**：经 `<img>/<link>` 消费的接口不得放入 sensitive 组（无 Authorization 头）；代理端点必须用 setProxyCORS

### 更新器签名（updater/verify.go）
- **Asset.Signature** = 对文件 SHA256 摘要（32 字节原始字节）的 Ed25519 签名，base64；公钥内嵌 `updatePublicKeyRaw`
- **私钥**：`C:\Users\libing\flore-update-signing-private.key`（PKCS8 PEM，仓库外）；发布流水线用 `node scripts/sign-update.mjs <update.json> <私钥>` 补签
- **禁止**：`FLORE_UPDATE_MANIFEST_URL` 已被移除（曾用于劫持 manifest）；manifest 仅从固定 HTTPS 通道拉取
- 换密钥 = 改 verify.go 常量 + 重新补签所有资产

### 端口分配（D-C4 修复）
- 后端 `PORT=0` 由系统分配端口，绑定后写 `FLORE_PORT_FILE` 指定文件；桌面壳 `waitForPortFile` 轮询读取
- **禁止**再实现"探测空闲端口再交给后端绑定"的 findFreePort 模式（TOCTOU）

### 桌面壳
- `generateAPIToken` 失败必须 fail-closed（os.Exit(1)），禁止返回空 token
- `stopBackends` 杀进程失败后必须有二次超时兜底，禁止 `<-exited` 无超时等待

## 备份恢复设计（2026-08-01）

**全量备份** = 配置（settings.json）+ 订阅源（subscriptions.opml）+ 数据库（database.db）

**恢复粒度**：
- 全量恢复：替换数据库 + 设置 + 订阅
- 仅恢复配置：仅写入 Settings 表，不影响文章
- 仅恢复订阅源：仅导入 OPML，不影响数据库和设置

**API 端点**：
- `GET /api/backups/:name/contents` — 获取备份内容清单
- `POST /api/backups/:name/restore-config` — 仅恢复配置
- `POST /api/backups/:name/restore-opml` — 仅恢复订阅源

**备份策略**：保留 N 个 + 保留 M 天，自动清理过期备份

## 项目关键决策

### 同步策略
- **不自建同步**，改为做 Fever API 客户端
- Alpha 版显式声明"无多端同步"
- Beta 阶段实现 Fever API 客户端（11-16 人日）

### 数据主权
- 本地 SQLite + 完整备份体系
- 无云端依赖
- 多端同步通过用户自己的 Miniflux/FreshRSS 服务器实现

## pubDate 类型混用修复（2026-08-02）

**问题**：SQLite 混合类型排序导致新抓取的 integer 格式文章被排到 text 格式文章之后，用户需要滚动才能看到最新文章。

**根因**：历史导入数据的 pubDate 为 ISO 8601 text 格式，新项目代码写入 integer 毫秒时间戳，SQLite DESC 排序将 text 类型始终排在 integer 类型之前。

**修复状态**：
- 迁移脚本已添加到 `migrations.go`（v2），后续启动会自动迁移
- 便携版数据库已迁移：`F:/Program Files/Flore-portable/data/reader.db`（1970 条 text → integer）
- 备份：`reader.db.bak`

**关键文件**：
- `server/go/internal/database/migrate_pubdate.go`：迁移实现
- `server/go/internal/database/migrations.go`：注册 v2 迁移
- 顶部：备份与恢复 → 立即备份、备份管理、刷新
- 备份管理弹窗：恢复备份、导入备份、导出备份、删除备份
- 备份列表：每行有下载、全量恢复、仅恢复设置、仅恢复订阅、删除按钮
- 备份策略：保留数量/天数、自动备份开关和间隔
## 前端样式清理（2026-08-02）
- **问题**：侧边栏 `Sidebar.tsx` 中存在 `ThemeToggle` 组件，与设置菜单外观切换重复
- **修复**：移除 `Sidebar.tsx` 中的 `ThemeToggle` import 和渲染，保留设置菜单中的主题切换

## 托盘组件修复（2026-08-02）
- **问题**：energye/systray 长时间运行后托盘点击无响应
- **初步方案**：迁移到 gogpu/systray（避免 LockOSThread 线程竞争）
- **新问题**：gogpu/systray 使用 HWND_MESSAGE，SetForegroundWindow/TrackPopupMenu 失败
- **最终方案A**：回退到 energye/systray v1.0.3 + RunWithExternalLoop
  - `RunWithExternalLoop` 返回 start/end 函数，由调用方控制消息循环
  - 避免 LockOSThread 与 Wails WebView2 COM 初始化的线程竞争
  - energye/systray 使用普通窗口（WS_OVERLAPPEDWINDOW），TrackPopupMenu 可正常工作
- **文件**：`apps/desktop/systray.go` 完全重写，`go.mod` 换回 energye/systray
- **图标**：`//go:embed build/favicon.ico`；Windows 下 energye/systray 使用 `LoadImageW(IMAGE_ICON)` 加载，**仅支持 ICO 格式，PNG 会导致图标不显示**（此问题在 M8 修复时已修正但未更新 MEMORY）

## 构建规范（重要！2026-08-02）
- **Wails 应用必须用 `wails build` 构建，禁止裸 `go build`！**
- 裸 `go build` 缺少 `production` build tag，Wails 匹配 `app_default_windows.go` 错误实现，弹出模态 MessageBox 阻塞启动（表现为：托盘不显示、startup 不触发、WebView2 不启动）
- 手动构建需加：`go build -tags production`
- 完整打包走 `build-desktop.ps1`（会依次构建后端、wails build、打便携包）

## 备份相关文件
- 后端服务：`server/go/internal/services/backup.go`
- 后端路由：`server/go/internal/handlers/reader.go`（备份相关 handler）
- 前端组件：`apps/web/src/components/settings/SettingsDataTab.tsx`（2026-08-02 完全重写）
- 图标：`apps/web/src/components/icons.tsx`（Cog、ArchiveRestore）

## 导出功能修复（2026-08-03）

### PDF 导出被动修复
- **问题**：`handleExportPDF` 使用 `html2canvas` 截图方式，导致长文章截断、图片异常
- **根因**：方案错误，截图方式不适合文本文档
- **修复**：改为浏览器原生打印，直接在新窗口渲染 HTML 内容，保留矢量文本和图片
- **文件**：`apps/web/src/components/Reader.tsx`

### Markdown 导出调整
- **问题**：右键"导出为 Markdown"使用 `a.click()` 静默下载，未弹出文件保存对话框
- **修复**：`downloadItemMarkdown` 改为 async 函数，优先调用 Wails `SaveMarkdownFile`，其次用 `showSaveFilePicker` 弹出系统对话框，最后回退到静默下载
- **文件**：`apps/web/src/utils/contextMenu.ts`、`apps/web/src/utils/api.ts`

## Media RSS 完整支持（方案B，2026-08-03）

**实现**：从单张缩略图升级为完整 Media RSS 支持，存储多张图片并在前端展示画廊。

**后端改动**：
- `models.go`：Item 新增 `MediaUrls []string` 字段（JSON 序列化）
- `fetcher.go`：新增 `extractMediaUrls()` 支持 media:thumbnail、media:content、media:group、img src
- `upsertSingleItem()`：序列化 mediaUrls 为 JSON 存储

**前端改动**：
- `types.ts`：Item 接口新增 `mediaUrls: string[] | null`
- `Reader.tsx`：多张图片展示画廊（2列网格），单张图片保持全宽

**数据库**：`mediaUrls` 列由 GORM AutoMigrate 自动创建，现有文章返回 null，前端兼容显示

## 抓取性能优化（2026-08-02）

**问题**：同一网络/系统下，Flore 每次抓取需 30-60 秒，竞品（Folo、FluentReader）只需几秒。

**三处改动**：

1. **DNS 缓存**（`urlpolicy.go`）：新增 `dnsCache` map + 60s TTL，`lookupHostWithCache()` 封装，`DialContext` 改调缓存版，避免每源重复同步 DNS lookup
2. **移除索引阻塞**（`coordinator.go`）：`worker()` 删除 `WaitIndexChan()` 调用，FTS5 索引已走异步 channel，worker 无需串行等待
3. **连接池放大**（`urlpolicy.go`）：`MaxIdleConns` 100→200，`MaxIdleConnsPerHost` 10→50，`IdleConnTimeout` 90s→120s

## 窗口状态图标不一致修复（2026-08-02）

- **问题**：启动时窗口右上角最大化/还原图标与窗口实际状态相反
- **根因**：Wails runtime 在 `Frameless: true` 模式下启动初期 `WindowIsMaximised()` 返回不稳定值；持久化状态文件与实际启动状态可能不同步
- **修复**：`useEffect` 改为 async 内部函数，先尝试 `await GetWindowState()`（持久化状态作默认值），再 `await WindowIsMaximised()`（运行时实际状态作优先值）；`WindowToggleMaximise` 后改为 `await` 调用
- **变更**：`app.go` 新增 `GetWindowState()`；`TitleBar.tsx` 重写 `useEffect`；`api.ts` 更新 `Promise` 类型
- **状态**：Go build ✓，TypeScript ✓，测试 ✓，前端已重新构建并复制到 `desktop/frontend/dist/`

## 路由鉴权与代理约定（2026-08-02，M-13 修复确立）

- **只读透传代理必须放在非敏感路由组**：`/image-proxy`、`/css-proxy`、`/favicon-proxy` 都注册在 `api` 组（非 `sensitive`）。
- **根因**：`<img>`/`<link>` 无法携带 `Authorization` 头；而 `sensitive` 组在 `FLORE_API_TOKEN` 非空时要求 Bearer → 桌面端经 `<img>` 加载的图片会 401。把只读代理放敏感组既无解安全收益（它们已有全局速率限制 + 类型白名单 + 尺寸上限），又会让桌面端图片全部失效。
- **`sensitive` 组的本意**：仅保护破坏性写操作（DB 导入/导出、OPML 导入、源/文件夹/过滤规则删除等）。任何经 `<img>` 消费的只读接口都不应放入。
- **Favicon 代理安全约束**：`/favicon-proxy?domain=` 只允许 `domain` 作路径片段、regex 校验 hostname 字符，上游 host 由后台 `FAVICON_SERVICE_BASE` 固定（默认 `https://favicon.yandex.net/favicon`）→ 无 SSRF 面。上游失败返回 502，前端 `SourceAvatar` 的 `onError` 回退字母头像。
- **头像代理多模式**（2026-08-03）：`faviconMode` 设置项：`off`（字母头像）/ `yandex`（Yandex 图标服务 `/favicon-proxy`）/ `direct`（后端从源站直接抓 favicon.ico `/favicon-direct`）。`api.iowen.cn` 已下线；Google 服务被墙不可用。
- **不要重复造轮子**：头像走后端代理，不前端直接请求第三方。

## 摘要模式 RSS 图片不显示（2026-08-03）

**问题**：某订阅源抓取到的摘要模式 RSS 信息不显示图片，其他 RSS 支持。

**根因**：Go 抓取层用标准 XML 解析 `<description>`/`<content:encoded>`，未提取 Media RSS `<media:thumbnail url="..."/>` 和图片 `<img src>`。

**修复（方案A）**：
- `server/go/internal/services/fetcher.go`：新增 `extractThumbnailUrl()`（优先 media:thumbnail，其次 img src），在 `parseRSS`/`parseAtom` 中调用，写入 `FeedItem.ThumbnailUrl`
- `server/go/internal/models/models.go`：`Item` 和 `ItemWithSource` 新增 `ThumbnailUrl *string`
- `server/go/internal/services/fetcher.go`：`upsertSingleItem` 保存 thumbnailUrl
- `apps/web/src/types.ts`：`Item` 接口添加 `thumbnailUrl: string | null`
- `apps/web/src/components/Reader.tsx`：摘要模式标题下方渲染缩略图（maxHeight 400px，失败时隐藏）
- GORM AutoMigrate 自动新增 thumbnailUrl 列

**验证**：`go build` ✓，`go test` ✓，`tsc --noEmit` ✓

## 版本号注入约定（2026-08-03 确立，踩坑后固化）

- **供 `-ldflags -X` 注入的 Go 字符串变量，必须用字符串字面量初始化**，禁止函数调用初始化器。
  - 错误：`var appVersion = resolveAppVersion()` → 包 init 覆盖链接器写入值，`-X` 静默失效（`go tool nm` 仍能看到 `.str` 符号，具有极强迷惑性）。
  - 正确：`var appVersion = "dev"`，兜底逻辑放到运行时函数里判断。
- **版本共三处，改版本号时必须同步**：
  1. 根 `package.json` 的 `version`（唯一真源，构建脚本从此读取）
  2. `apps/desktop/version.go`（由 `build:desktop` 自动生成，供 updater 版本比较）
  3. `apps/desktop/wails.json` 的 `productVersion`（由 `apps/desktop/sync-version.mjs` 自动同步，影响 exe 文件属性）

  - `sync-version.mjs` 读取根 `package.json` 的路径是 `../../package.json`（`apps/desktop` → `apps` → 项目根），不是 `../package.json`（那会指向 `apps/package.json` 导致 ENOENT）。
- **前端"关于"页版本只来自后端 HTTP `/api/version`**，不走 Wails `GetVersion()`。排查版本显示问题应优先查后端 ldflags 链路，而非桌面壳。
- 后端注入命令：`-X github.com/rss/go-server/internal/handlers.appVersion=<version>`；dev 模式（`go run`）无注入会显示 `dev`，可用 `FLORE_VERSION` 环境变量覆盖。

## SQLite 数据库迁移（2026-08-03）

**问题**：`Item.MediaUrls []string` 字段在 SQLite 上 `gorm AutoMigrate` 时报 `unsupported data type: &[]`。

**根因**：GORM 的 `serializer:json` 对 `[]string` 切片类型在 SQLite 上没有反序列化成 `interface{}` 的注册转换，导致类型不匹配。

**修复方案**：
- **方案 A（推荐）**：改用 `[]byte` 手动编解码，保持 SQLite 不变。
- **方案 B**：改用 `string` 存 JSON 字符串，读写时手动 `json.Marshal`/`json.Unmarshal`。
- **方案 C**：迁至 PostgreSQL，其 `jsonb` 原生支持 `[]string`。

**相关文件**：`server/go/internal/models/models.go`（`Item.MediaUrls` 定义）、`server/go/internal/database/migrate.go`（AutoMigrate 调用点）。
