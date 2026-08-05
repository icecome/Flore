# Flore 项目长期记忆

## 一、安全约定（改安全相关代码前必读）
### 认证与 CSRF（handlers/security.go）
- 全局注册 `handlers.CSRFProtection()`（CORS 之后）；非 GET/HEAD 请求带 Origin 须"本地源 + X-Requested-With: XMLHttpRequest"，Origin:null 拒，无 Origin 非浏览器客户端放行。
- 前端所有写请求经 `fetchData.ts` 的 `withTimeout`（自动注入 X-Requested-With），禁止绕过。
- CORS `AllowOriginFunc` 用 `handlers.IsLocalOrigin`：**按 `url.Hostname()` 匹配** `127.0.0.1`/`localhost`/`wails.localhost`，**忽略 scheme 与端口**（2026-08-05 改，原为 scheme+端口精确匹配）。覆盖各端：Windows/Linux 生产 `http://wails.localhost`、macOS 生产 `wails://wails.localhost`、dev 模式 `http://wails.localhost:34115`；精确匹配天然挡 `wails.localhost.evil.com` 子域欺骗与外网源。
- **关键坑（2026-08-05 定位）**：Wails v2.13 macOS 生产 webview 的 Origin 是 `wails://wails.localhost`（非 `http://`），旧白名单只认 `http://wails.localhost` 精确值 → 所有 GET 被 403 → 前端"获取失败"。改 hostname 匹配后修复，且对 Win/Linux 无回归。
- 只读透传代理用 `setProxyCORS` 反射本地源，禁写 `ACAO:*`。

### 更新器签名（updater/verify.go）
- `Asset.Signature` = 文件 SHA256 摘要（32 字节）的 Ed25519 签名 base64；公钥内嵌 `updatePublicKeyRaw`。
- 私钥：`C:\Users\libing\flore-update-signing-private.key`（仓库外）；发布用 `node scripts/sign-update.mjs <update.json> <私钥>` 补签。
- `FLORE_UPDATE_MANIFEST_URL` 已移除；manifest 仅走固定 HTTPS。换密钥 = 改常量 + 重签所有资产。

### 端口分配
- `PORT=0` 由系统分配，绑定后写 `FLORE_PORT_FILE`；桌面壳 `waitForPortFile` 轮询读。禁止 findFreePort（TOCTOU）。

### 桌面壳
- `generateAPIToken` 失败须 fail-closed（`os.Exit(1)`），禁返回空 token。
- `stopBackends` 杀进程失败须有二次超时兜底。

## 二、数据架构与决策
- 全量备份 = 配置 + 订阅 OPML + 数据库。恢复粒度：全量 / 仅配置 / 仅 OPML。策略：保留 N 个 + 保留 M 天。
- 端点：`GET /api/backups/:name/contents`、`POST /api/backups/:name/restore-config`、`POST /api/backups/:name/restore-opml`。
- 同步策略：不自建同步，改做 Fever API 客户端；本地 SQLite + 完整备份，无云端依赖；多端同步借用户自有 Miniflux/FreshRSS。

## 三、构建与版本规范
- **Wails 应用必须用 `wails build`**，禁裸 `go build`（缺 `production` tag 会匹配 `app_default_windows.go` 弹模态 MessageBox 阻塞启动）。手动需 `go build -tags production`。
- **版本三处同步**：① 根 `package.json` 的 `version`（唯一真源）② `apps/desktop/version.go`（自动生成）③ `apps/desktop/wails.json` 的 `productVersion`（由 `sync-version.mjs` 同步，路径 `../../package.json`）。
- **ldflags 注入变量须字符串字面量初始化**（禁函数调用初始化器，否则 `-X` 静默失效）。后端注入：`-X github.com/rss/go-server/internal/handlers.appVersion=<ver>`；dev 模式无注入显示 `dev`，可用 `FLORE_VERSION` 覆盖。
- 前端"关于"页版本只走后端 `/api/version`，不走 Wails `GetVersion()`。

## 四、macOS 分发（核心：后端自衍生架构）
**铁律：分发包里只有一个 `Flore.app`，不再有第二个独立 `florebackend` 二进制。主程序以 `--backend` 启动自身子进程跑后端。**

- **动机**：曾把 `florebackend` 单独打包（包外或包内 `codesign --deep`），但下载带 `com.apple.quarantine` 的 `.app` 点"仍要打开"只放行主程序，**第二个独立二进制仍被 Gatekeeper 静默拦截** → 后端永不起 → 永久"获取失败"。本机无 quarantine 所以"本机正常"是假象。
- **实现**：
  - `server/go/backend/backend.go` 导出 `Start()(*Server,error)` + `Server.Stop()/RunBlocking()`，承载原 `cmd/main.go` 全部 gin/路由/DB/监听/优雅关闭（同模块 `github.com/rss/go-server`，可 import internal）。
  - `server/go/cmd/main.go` = `backend.Start().RunBlocking()`（独立 Web 版仍可用）。
  - `apps/desktop/main.go`：`main()` 开头判 `os.Args` 含 `--backend` → `runBackendMode()`（禁启 Wails GUI）。
  - `apps/desktop/backend.go` 的 `startBackends()` 用 `os.Executable()+["--backend"]` 启自身子进程（保留 `FLORE_BACKEND_PATH` 开发态覆盖）。已删 `findGoBackend`/`findBackendByExecutable`/`maxDepth` 查找逻辑。
  - `apps/desktop/go.mod`：`require github.com/rss/go-server` + `replace => ../../server/go`；`go mod tidy` 拉齐 gin/gorm/sqlite。主程序 `Flore` 含后端（~23MB）。
  - `scripts/build-desktop.mjs` 不再单独 `go build florebackend`；`package-tool` 不再拷贝/签名 florebackend，仅收 `Flore.app`。
- **数据目录**：darwin 跳过包内 `findPortableDataDir()`，直接 `userDataDirectory()` → **`~/.flore`**（始终可写）。改数据路径前必读此条。Windows/Linux 维持原便携判定。
- **验证**：`Flore --backend` 跑同二进制端口正常、`/health` → `{"status":"ok"}`、迁移完成；GUI 常驻时子进程正常存活。headless 下 `wails.Run` 立即返回→shutdown 杀子进程，看不到后端就绪属预期。
- **排查"获取失败"**：① `Flore.app/Contents/MacOS/Flore` 应 ~23MB 且 `--backend` 可独立起服务；② `~/".flore"/floredesktop.log` 应有 `spawning backend` 与 `Go backend ready`；③ 前端 `applyBackendStatus` 轮询窗口已对齐后端启动上限（75×200ms≈15s，`apps/web/src/utils/api.ts`）。
- 首次运行右键"打开"或 `xattr -d com.apple.quarantine Flore.app` 清隔离；单实例锁残留进程会致"只有 dock 图标无窗口"，先 `pkill -f Flore` 再启动。

## 五、路由代理与头像约定
- 只读透传代理（`/image-proxy`、`/css-proxy`、`/favicon-proxy`、`/favicon-direct`）注册在 `api` 组（非 `sensitive`），因 `<img>/<link>` 无法带 `Authorization`，放 sensitive 会让桌面端图片全部 401。`sensitive` 组只保护破坏性写操作。
- Favicon 代理仅允 `domain` 路径片段 + regex 校验 hostname，上游 host 由 `FAVICON_SERVICE_BASE` 固定 → 无 SSRF。
- `faviconMode`：`off`/`yandex`(`/favicon-proxy`)/`direct`(`/favicon-direct`)。头像走后端代理，禁前端直连第三方。

## 六、功能实现备忘（简述）
- **Media RSS**：`Item.MediaUrls`（JSON 序列化）`extractMediaUrls()` 支持 media:thumbnail/content/group/img；前端画廊 2 列，单张全宽。
- **摘要 RSS 图片**：`Item.ThumbnailUrl *string`，`extractThumbnailUrl()` 优先 media:thumbnail 再 img src；摘要模式标题下渲染。
- **MediaUrls 迁移坑**：GORM `serializer:json` 对 `[]string` 在 SQLite 报 `unsupported data type`；落地用 `[]byte` 手动编解码或 string 存 JSON 规避。
- **抓取性能**：DNS 缓存 60s TTL；`coordinator.go` 删 `WaitIndexChan()`；连接池 MaxIdleConns 200/PerHost 50/Idle 120s。
- **导出**：PDF 改原生打印（弃 html2canvas 截图截断）；Markdown 优先 Wails `SaveMarkdownFile` → `showSaveFilePicker` → 静默兜底。
- **窗口状态图标**：`TitleBar.tsx` 用 `GetWindowState()` 默认 + `WindowIsMaximised()` 优先，toggle 后 `await`。
- **标题栏 macOS 原生风格（2026-08-05 改）**：mac 端**不再渲染独立标题栏行**（`TitleBar.tsx` 对 `platform==='darwin'` 直接 `return null`）；侧边栏（`Sidebar.tsx`）顶到窗口顶部，用顶部 28px 行与交通灯共用——该行右对齐放**搜索按钮**（仅 mac），点击展开 `SearchBox`（`autoFocus`），再点收起；交通灯由系统在左上角渲染，按钮在右故不挤占。**非 mac 维持原样**：始终显示 `SearchBox`。`Windows/Linux` 另保留带三按钮的独立标题栏（h-[34px]），完全不受影响。
- **mac 搜索栏实现的坑**：不能给 `aside` 整体加 `pt-[28px]` 后又在顶部插 28px 行（会叠成 56px）；正确做法是顶部仅一行 28px（交通灯区）容纳按钮，正文从下一行开始。`showMacTop = isMac`（**非 `isMac || macLoading`**）：非 mac 永不渲染该行，避免平台未知期 28px 空占位行跳动；`macLoading` 仅门控非 mac 搜索框出现时机，mac 期间不渲染 mac 专属装饰。`getPlatform` 异步期间 mac 内容短暂顶到交通灯，判定后立即补 28px 行，属可接受极小闪烁。
- **mac 拖拽**：Wails `mac.TitleBarHidden()` 模式下整块 webview 默认可作拖拽区，移除前端标题栏后 mac 窗口仍可拖，Go 端无需改动。

## 七、平台怪癖
- **Windows 托盘**：`systray.go` 用 energye/systray v1.0.3 + `RunWithExternalLoop`，图标 `//go:embed build/favicon.ico` **仅支持 ICO**，PNG 不显示。
- macOS App Translocation 会把 `.app` 复制到只读临时路径；数据写包内违反规范且只读失败，故 darwin 走 `~/.flore`。
- **macOS select 控件风格统一**：`apps/web/src/index.css` `@layer base` 加 `:where(select)` 全局规则（`appearance:none` + 内联 SVG `⌄` 背景图 + `padding-right:2rem` + `background-position: right 0.75rem center`），覆盖项目全部 13 处 `<select>`（`SettingsShared.Select` + 各 Modal/Tab 裸 select），统一呈现扁平箭头、与 macOS AppKit NSPopUpButton 风格一致。`:where()` 优先级恒 0 不被 Tailwind class 压过；亮色 stroke `%2368707A`，暗色 `[data-theme="dark"]` 覆盖为 `%23A8ACB4`。跨 Win/Linux/macOS/Web 同款样式，**不再分多端规则**（用户 17:18 确认接受现状、要求保持单一全局）。
