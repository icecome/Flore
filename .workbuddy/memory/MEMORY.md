# Flore 项目长期记忆

## 安全约定（改安全代码前必读）
- **CSRF**：后端全局 `handlers.CSRFProtection()`（CORS 之后）；非 GET/HEAD 且带 Origin 须"本地源 + `X-Requested-With: XMLHttpRequest`"，Origin:null 拒，无 Origin 的非浏览器放行。前端写请求必须经 `fetchData.ts` 的 `withTimeout`（自动注入头）。
- **CORS**：`AllowOriginFunc` 用 `handlers.IsLocalOrigin`；`AllowHeaders` 含 X-Requested-With。
- **只读代理**：`/image-proxy`、`/css-proxy`、`/favicon-proxy` 放非敏感路由组（`<img>` 无法带 Authorization，放 sensitive 组会导致桌面端 401）。上游 host 固定 `FAVICON_SERVICE_BASE`（默认 Yandex），regex 校验 domain，无 SSRF 面。
- **更新器签名**：`Asset.Signature` = 文件 SHA256（32字节）Ed25519 签名 base64，公钥内嵌 `updatePublicKeyRaw`；私钥在仓库外，发布流水线 `node scripts/sign-update.mjs` 补签；禁止 `FLORE_UPDATE_MANIFEST_URL`。
- **头像模式** `faviconMode`：`off`/`yandex`(`/favicon-proxy`)/`direct`(`/favicon-direct`)。

## 构建与运行规范
- **Wails 必须 `wails build`**，禁裸 `go build`（缺 `production` tag 会弹 MessageBox 阻塞启动）。
- **端口**：后端 `PORT=0` 系统分配，绑定后写 `FLORE_PORT_FILE`；桌面壳 `waitForPortFile` 轮询。禁 findFreePort 模式（TOCTOU）。
- **桌面壳**：`generateAPIToken` 失败必须 fail-closed（`os.Exit(1)`）；`stopBackends` 杀进程失败需二次超时兜底。

## 版本号注入（改版本号必同步三处）
1. 根 `package.json` 的 `version`（唯一真源）
2. `apps/desktop/version.go`（由 `build:desktop` 自动生成）
3. `apps/desktop/wails.json` 的 `productVersion`（由 `sync-version.mjs` 同步，读 `../../package.json`）
- 供 `-ldflags -X` 的 Go 变量必须用字符串字面量初始化（禁函数调用初始化器）；前端"关于"版本只走后端 `/api/version`。

## 备份与恢复
全量 = 配置(settings) + 订阅(subscriptions.opml) + 数据库(database.db)。粒度：全量 / 仅配置 / 仅 OPML。策略：保留 N 个 + M 天。端点：`GET /api/backups/:name/contents`、`POST /api/backups/:name/restore-config`、`POST /api/backups/:name/restore-opml`。

## 关键决策
- 不同步自建，做 Fever API 客户端（Beta，11-16 人日）；Alpha 声明无多端同步。
- 本地 SQLite + 完整备份，无云端依赖；多端同步经用户自有 Miniflux/FreshRSS。

## 已完成的修复（要点）
- pubDate 类型混用 → v2 迁移 integer 毫秒（排序修复）。
- 前端移除 Sidebar 重复 ThemeToggle；托盘回退 energye/systray v1.0.3 + RunWithExternalLoop（图标仅 ICO）。
- 导出 PDF 改原生打印；Markdown 导出优先 `SaveMarkdownFile`。
- Media RSS 完整支持（`mediaUrls` 画廊，[]string 用 []byte 手动编解码绕过 GORM SQLite 限制）。
- 抓取性能：DNS 缓存(60s) + 去 `WaitIndexChan` 阻塞 + 连接池放大。
- 窗口状态图标不一致：先 `GetWindowState()` 再 `WindowIsMaximised()`。
- 摘要模式图片：`extractThumbnailUrl()`（media:thumbnail > img src）。

## 自动抓取调度缺陷（2026-08-04 RCA，待修复）
- **症状**：后台挂机一晚无自动抓取，手动正常。
- **根因**：`adaptiveNextCheckAt`（`server/go/internal/services/source_service.go:379`）在 `newCount==0` 时用 `maxInterval`（后端 `fetchMaxInterval` 默认 **4320 分钟=3 天**），无新增源被推到 3 天后。前端 `defaultInterval`（UI"默认抓取间隔"，默认 120）仅作用于**新建源的 interval**，与后端调度 `fetchMaxInterval` 是两回事；UI 文案"按此间隔自动抓取"与实际不符。
- **修复方向**：`newCount==0` 时尊重源 interval（不再推 3 天），或大幅降低 `maxInterval` 默认值并挂钩 interval。避免引入与 `defaultInterval` 重复的新设置项。
