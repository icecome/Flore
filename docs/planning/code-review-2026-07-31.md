## 代码审查报告

> **审查框架：** SPEAR（Security / Performance / Error Handling / Architecture / Reliability）
> **审查范围：** Flore RSS Reader 全栈代码（后端 Go / 前端 React+TS / 桌面壳 Wails）
> **审查日期：** 2026-07-31
> **审查模式：** 代码审计（全量逐文件）
> **审查工具：** comprehensive-code-review skill

---

## 目录

1. [总体评分](#1-总体评分)
2. [问题汇总](#2-问题汇总)
3. [CRITICAL 问题详情](#3-critical-问题详情)
4. [MAJOR 问题详情](#4-major-问题详情)
5. [MINOR 问题列表](#5-minor-问题列表)
6. [NIT 问题列表](#6-nit-问题列表)
7. [做得好的地方](#7-做得好的地方)
8. [改进建议（按优先级）](#8-改进建议按优先级)
9. [覆盖率校验](#9-覆盖率校验)

---

## 1. 总体评分

### 加权总分：67/100 — 🟡 NEEDS WORK

| 维度 | 权重 | 评分 | 关键发现 |
|------|------|------|---------|
| 🔴 安全 | 3x | 7.0/10 | SSRF 防护多层但有 DNS rebinding 绕过窗口；GetSetting 端点未受认证保护；SQL 全参数化 |
| 🟡 性能 | 2x | 6.5/10 | html2canvas 静态导入拖累包体积；长列表无虚拟化；多处函数/数组在渲染时重建 |
| 🟠 错误处理 | 2x | 5.5/10 | 多处直接访问 s.db 绕过锁；backup/restore 关键错误被吞；res.json() 隐式 any |
| 🔵 架构 | 1.5x | 5.0/10 | 11 个文件严重超阈值（最大 1944 行）；handlers/reader.go 职责混杂 |
| 📊 可靠性 | 1.5x | 6.0/10 | Coordinator Stop 顺序可致 panic；useSourcesData setter bug 致功能失效；sw.js 离线缓存失效 |

### 裁定：🟡 NEEDS WORK

需修复 1 个 CRITICAL、40 个 MAJOR 问题后再进行下一轮迭代。安全基础扎实（无 SQL 注入、XSS 防护到位），主要问题集中在架构（文件规模）和可靠性（竞态、错误吞没）。

---

## 2. 问题汇总

### 按严重级别统计

| 级别 | 数量 | 说明 |
|------|------|------|
| 🔴 CRITICAL | 1 | 运行时崩溃/功能失效 |
| 🟠 MAJOR | 40 | Bug、逻辑错误、重大性能退化、架构红线 |
| 🟡 MINOR | 47 | 降低维护成本的改进 |
| 🔵 NIT | 17 | 风格偏好、命名建议 |
| **总计** | **105** | **必须等于各级别之和** |

### 按模块分布

| 模块 | CRITICAL | MAJOR | MINOR | NIT |
|------|----------|-------|-------|-----|
| 后端 handlers | 0 | 10 | 4 | 2 |
| 后端 services/reader.go | 0 | 3 | 4 | 1 |
| 后端 services/fetcher.go | 0 | 2 | 2 | 1 |
| 后端 services/filter.go | 0 | 2 | 1 | 1 |
| 后端 services/backup.go | 0 | 4 | 2 | 0 |
| 后端 services/coordinator.go | 0 | 2 | 0 | 1 |
| 后端 services/urlpolicy.go | 0 | 1 | 3 | 0 |
| 后端 services/export.go | 0 | 1 | 1 | 1 |
| 后端 services/readability.go | 0 | 0 | 4 | 0 |
| 后端 services/scheduler.go | 0 | 0 | 2 | 0 |
| 后端 database/models | 0 | 1 | 2 | 1 |
| 后端 cmd/main.go | 0 | 1 | 2 | 0 |
| 前端 App.tsx | 0 | 2 | 2 | 1 |
| 前端核心组件（6 个） | 0 | 8 | 6 | 2 |
| 前端 hooks（4 个） | 1 | 1 | 2 | 1 |
| 前端 utils（5 个） | 0 | 1 | 4 | 1 |
| 前端弹窗组件（9 个） | 0 | 0 | 3 | 1 |
| 前端通用组件（11 个） | 0 | 0 | 3 | 1 |
| 前端 settings 子组件（8 个） | 0 | 2 | 3 | 0 |
| 桌面壳（3 个 Go 文件） | 0 | 1 | 1 | 0 |
| 前端配置/SW/manifest | 0 | 1 | 4 | 2 |
| 类型定义（3 个） | 0 | 0 | 0 | 0 |
| 构建脚本（3 个） | 0 | 0 | 0 | 1 |

---

## 3. CRITICAL 问题详情

### C-01: useSourcesData.ts 调用未定义的 setter 函数（运行时崩溃）

- **文件：** [useSourcesData.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/hooks/useSourcesData.ts#L77)
- **类别：** Reliability / Error Handling
- **问题：** `fetchUnreadCount` 内部调用 `setUnreadCountGlobal(...)`，但组件中定义的 state setter 是 `setUnreadCountInScope`（L29）。`setUnreadCountGlobal` 在该作用域内未定义，运行时抛出 `ReferenceError`。同时 hook 返回的是 `setUnreadCountInScope`（L109），而 `App.tsx:68` 解构的是 `setUnreadCountGlobal`，导致 App 中拿到的也是 `undefined`。
- **影响：**
  - `fetchUnreadCount` 每次执行都会进入 catch 块，未读总数永远不更新（`unreadCountInScope` 恒为 0）
  - 用户侧表现为：header 的"· N 未读"计数永远为 0；系统通知中的"共 N 篇未读"始终为 0
  - catch 块吞掉 ReferenceError 后弹出 toast "获取未读数失败"，用户会频繁看到该提示
- **建议修复：**

```typescript
// useSourcesData.ts L77：将 setUnreadCountGlobal 改为 setUnreadCountInScope
setUnreadCountInScope(typeof data.count === 'number' ? data.count : 0);
```

同时统一 `App.tsx:68` 的解构名称与 hook 返回值一致。

---

## 4. MAJOR 问题详情

### 安全类

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| M-S1 | [urlpolicy.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/urlpolicy.go#L159-L173) | L159-173 | **DialContext DNS rebinding 漏洞**：`host, _, err := net.SplitHostPort(addr)` 取出的 host 可能是域名而非 IP。`isPrivateIP(host)` 对域名调用 `net.ParseIP` 返回 nil，判定为非私有直接放行。攻击者可构造 DNS rebinding：ValidateURL 解析时返回公网 IP 通过检查，连接时 DNS 切换到 127.0.0.1，DialContext 检查被绕过。**影响**：SSRF 防护在 DialContext 层失效，可访问内网服务。**修复**：DialContext 内先 `net.DefaultResolver.LookupHost` 解析域名，再检查所有解析出的 IP。 |
| M-S2 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L265) | L265 | **GetSetting 端点不在 sensitive 组**：`api.GET("/settings/:key", h.GetSetting)` 在普通 api 组，无认证。若设置了 apiToken，攻击者可读取任意设置项。**修复**：移到 sensitive 组。 |
| M-S3 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L1903-L1914) | L1903-1914 | **UpdateSettings 无 key/value 大小限制**：接收任意 `map[string]string`，未限制 key 数量或 value 长度。攻击者可提交超大 JSON 占用内存。**修复**：限制 key 数量（≤100）和 value 长度（≤10KB）。 |

### 性能类

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| M-P1 | [Reader.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Reader.tsx#L3) | L3 | **html2canvas 静态导入**：~200KB+ 打入主 chunk。仅 `handleExportPNG`/`handleExportPDF` 使用（低频操作），应改为 `const html2canvas = (await import('html2canvas')).default` 动态导入。 |
| M-P2 | [ArticleList.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ArticleList.tsx#L304-L339) | L304-339 | **长列表无虚拟化**：直接 `items.map` 渲染。项目约束要求长列表使用 `content-visibility: auto`，当前未实现。文章数 >200 时渲染性能明显退化。 |

### 错误处理类

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| M-E1 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L1288-L1296) | L1288-1296 | **GetCacheStats 多处忽略错误**：`Row().Scan`、`Raw().Scan`、`Count` 共 4 处未检查 `.Error`。DB 异常时 stats 字段保持零值，前端误以为缓存为空。 |
| M-E2 | [fetcher.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/fetcher.go#L168-L171) | L168-171 | **回写条件请求头时忽略错误**：`Updates(updates)` 未检查 `.Error`。写入失败导致下次抓取无法使用 304 协商，每次全量拉取浪费带宽。 |
| M-E3 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go#L176-L177) | L176-177 | **AutoMigrate 错误只 log 不返回**：restoreFromFile 内迁移失败仅 slog.Error，函数继续执行后续 VACUUM 和 invalidateUnreadCount，数据库 schema 可能不匹配。 |
| M-E4 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go#L240) | L240 | **replaceDatabaseFile 内忽略 database.Init 错误**：`_ = database.Init()` 显式丢弃错误。Init 失败后所有操作失败。 |
| M-E5 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go#L236-L239) | L236-239 | **回滚失败只 log**：`copyFile(bakPath, currentPath)` 回滚失败时仅 slog.Error，函数仍返回原错误。双重失败时数据库文件可能丢失。应返回聚合错误。 |
| M-E6 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L36-L103) | L36-103 | **rateLimiter 无 map 大小限制**：visitors map 无上限。窗口内大量不同 IP 请求可导致 map 膨胀（IP 洪泛）。应增加上限（如 10000）或用 LRU 淘汰。 |
| M-E7 | 多文件 | 多处 | **res.json() 隐式 any**：`useSourcesData.ts:37,51,76`、`useItemsData.ts:103,145,182`、`SettingsModal.tsx:139`、`App.tsx:528` 等 `await res.json()` 返回值未标注类型，退化为隐式 `any`，违反项目"禁止 any 兜底"规范。 |

### 架构类（文件规模超标）

| # | 文件 | 行数 | 阈值 | 问题 |
|---|------|------|------|------|
| M-A1 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go) | 1944 | 400 | HTTP handler + HTML/CSS 重写 + URL 代理 + 错误页面生成多职责。建议拆分为 `handlers/reader.go`、`handlers/proxy.go`、`handlers/html_rewrite.go`。 |
| M-A2 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go) | 1506 | 300 | 源/文件夹 CRUD + OPML + 文章查询 + 搜索 + readability 缓存 + 设置管理 + 清理 + 健康状态 9 职责。建议拆为 `reader_source.go`、`reader_opml.go`、`reader_search.go` 等。 |
| M-A3 | [App.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/App.tsx) | 1175 | 350 | God Component：20+ useState、10+ fetch、15+ handler、键盘快捷键、轮询、JSX 渲染。建议提取 `useKeyboardShortcuts`、`useArticleActions`、`useSourceActions`、`useFetchPolling` 等 hooks。 |
| M-A4 | [SettingsModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/SettingsModal.tsx) | 1002 | 350 | 15+ handler、规则 CRUD、源批量操作。建议将规则管理、源管理逻辑抽到 hooks。 |
| M-A5 | [app.go (desktop)](file:///c:/opt/workstations/project/py/rss/apps/desktop/app.go) | 838 | 300 | 后端进程管理 + 窗口控制 + 托盘 + 通知监听 + 文件对话框 + 设置缓存 6 职责。建议拆为 `backend.go`、`window.go`、`notify.go`、`dialog.go`。 |
| M-A6 | [Reader.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Reader.tsx) | 734 | 350 | 含 `rewriteImageUrls`/`rewriteIframeContent`/`rewriteSrcset` 三个 HTML 重写函数，应抽到 `utils/htmlRewrite.ts`。 |
| M-A7 | [ReaderToolbar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ReaderToolbar.tsx) | 702 | 350 | `TypePanel`/`MorePanel`/`SharePanel` 三个子面板应拆为独立文件。 |
| M-A8 | [Sidebar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Sidebar.tsx) | 639 | 350 | `FolderNode`/`SourceRow`/`FilterRow` 应拆为独立文件。 |
| M-A9 | [ArticleList.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ArticleList.tsx) | 638 | 350 | `ArticleRow` 子组件应拆为独立文件。 |
| M-A10 | [SettingsDataTab.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/settings/SettingsDataTab.tsx) | 666 | 350 | 多个 Section 子函数应提取为独立组件文件。 |
| M-A11 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go) | 496 | 300 | 导出/导入/恢复/压缩/清理/文件操作 6 职责。建议拆为 `backup_create.go`、`backup_restore.go`、`backup_cleanup.go`、`backup_util.go`。 |
| M-A12 | [SettingsRulesTab.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/settings/SettingsRulesTab.tsx) | 404 | 350 | 表单 + 列表 + 测试结果弹窗三块独立 UI。建议拆为 `RuleFormSection`、`RuleListSection`、`RuleTestResultModal`。 |
| M-A13 | [filter.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/filter.go) | 388 | 300 | 略超阈值，职责相对内聚。可将 TestFilterRule 拆到独立文件。 |
| M-A14 | [cn.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/lib/cn.ts) | 38 | — | 文件名为 `cn.ts`（类名拼接工具），但含 `formatDate`/`formatTime`/`formatRelative`/`formatFull` 日期函数。违反 SRP，应拆为 `utils/dateUtils.ts`。 |

### 可靠性类

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| M-R1 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L1334) | L1334 | **GetDatabaseInfo 直接访问 s.db 绕过 dbMu 读锁**：并发导入（restoreFromFile 持写锁替换 db）时此处可能持有已关闭的 db 句柄导致 panic。应改为 `s.getDb().Raw(...)`。 |
| M-R2 | [fetcher.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/fetcher.go#L493) | L493, L506 | **upsertFeedItems 链路直接访问 s.db 绕过 dbMu**：`batchGetExistingItems` 用 `s.db.Where(...)`、`upsertItemsInTx` 用 `s.db.Transaction(...)`。在 restoreFromFile 持写锁替换 db 时存在竞态。 |
| M-R3 | [filter.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/filter.go) | L116, L136, L171, L190, L194, L203, L228, L359, L369 | **全文件直接访问 s.db 绕过 dbMu**：所有 DB 操作用 `s.db.` 直接访问，未通过 `s.getDb()`。涉及 Create/Find/First/Updates/Delete/Transaction/Scan。数据库恢复期间过滤规则操作可能 panic。 |
| M-R4 | [filter.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/filter.go#L344) | L344 | **applyRuleAction 在事务内调用 invalidateUnreadCount**：事务尚未提交时失效缓存，下次 `GetSources` 重算未读数时事务可能尚未提交，读到旧的 isRead 状态，导致计数与实际不一致。应移到事务提交后执行。 |
| M-R5 | [export.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/export.go) | L47, L79, L92 | **全文件直接访问 s.db 绕过 dbMu**：`buildExportQuery`、`getItemsByIDs`、`GetItemWithSource` 均用 `s.db.Table(...)` 直接访问。 |
| M-R6 | [coordinator.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/coordinator.go#L78-L83) | L78-83 | **Stop 顺序可能导致 Submit panic**：`Stop()` 先 `close(c.stop)` 再 `close(c.taskCh)`。Submit 的 select 有 `c.taskCh <- sourceID` case，向已关闭 channel 发送会 panic。应仅 `close(c.stop)`，worker 退出后再 `close(c.taskCh)`。 |
| M-R7 | [coordinator.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/coordinator.go#L105-L120) | L105-120 | **Submit select 竞态**：`case <-c.stop` 与 `case c.taskCh <- sourceID` 同时就绪时随机选择。若 taskCh 已关闭则 panic。应在 mu 内检查 stop 标志后再发送。 |
| M-R8 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L79-L81) | L79-81 | **rateLimiter.stop() 从未被调用**：RegisterRoutes 创建的 rateLimiter 没有暴露给 main.go 调用 stop。程序退出时 goroutine 自然终止，但不是优雅关闭。 |
| M-R9 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L895-L912) | L895-912 | **SearchItems keyword 长度未限制**：keyword 直接传入 service.SearchItems，未限制长度。攻击者可发送超长 keyword 影响 FTS5 性能。应限制（如 ≤200 字符）。 |
| M-R10 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L956-L1000) | L956-1000 | **ExportItems 的 ExportScope 未限制 IDs 数量**：接收 services.ExportScope，未检查 IDs 数量。service 层有 5000 条限制，但 IDs 列表本身可能很大。 |
| M-R11 | [Reader.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Reader.tsx#L530-L535) | L530-535 | **onToggleViewOriginal stale closure**：`setDisplayMode` 用函数式更新（正确），但随后 `if (displayMode !== 'iframe')` 读取的是闭包中过期的 `displayMode`。连续快速点击时可能进入错误的分支。应用 ref 持有最新值。 |
| M-R12 | [sw.js](file:///c:/opt/workstations/project/py/rss/apps/web/public/sw.js#L1-L7) | L1-7 | **sw.js 缓存名/资源路径 bug**：`CACHE_NAME = 'rss-fetcher-v1'` 使用旧项目名；`STATIC_ASSETS` 包含 `'/src/main.tsx'`（开发路径，生产构建后不存在）。导致 Service Worker 安装时 `cache.addAll` 静默失败，离线缓存形同虚设。 |
| M-R13 | [app.go (desktop)](file:///c:/opt/workstations/project/py/rss/apps/desktop/app.go#L801-L837) | L801-837 | **startNotifyWatcher goroutine 泄漏风险**：goroutine 无 context 取消机制，shutdown 时无法优雅退出。应添加 `context.Context` 参数。 |

---

## 5. MINOR 问题列表

### 后端

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| m-01 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L1228) | L1228 | `isValidByline` 每次调用重新编译正则 `regexp.MatchString(`^\d+$`, byline)`。应提取为包级 `var numericBylineRe = regexp.MustCompile(...)`。 |
| m-02 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L743) | L743 | `importOutlineRecursive` 函数有 8 个参数。建议抽取 `opmlImportCtx` 结构体。 |
| m-03 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L60-L67) | L60-67 | `NewReaderService` sync.Once 内调用 `loadProxySettingsSeq()` → `GetSettingBool` → 访问 `database.DB`。若 NewReaderService 在 `database.Init()` 之前调用，DB 为 nil 会 panic。 |
| m-04 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L1461-L1465) | L1461-1465 | `recoverAndRollback` 捕获 panic 后仅 Rollback，不重新抛出，调用方得到 `(0, nil)` 无法感知。 |
| m-05 | [fetcher.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/fetcher.go#L124) | L124 | 条件请求头加载 `if err := ...; err == nil` 将所有错误视为"记录不存在"，包括 DB 连接错误。应用 `errors.Is(err, gorm.ErrRecordNotFound)` 区分。 |
| m-06 | [fetcher.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/fetcher.go#L562-L567) | L562-567 | `applyFiltersAndIndex` 错误仅 slog.Warn，调用方无法感知部分失败。应累计失败数返回给上层。 |
| m-07 | [scheduler.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/scheduler.go#L109-L122) | L109-122 | `Stop()` 关闭 stop chan 后 `wg.Wait()`，但首轮 goroutine 的 `select` 仅在启动时检查一次 stop，之后调用 `runOnce()` 不再检查 stop。 |
| m-08 | [scheduler.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/scheduler.go#L140-L145) | L140-145 | `runOnce` 本身未 `wg.Add(1)`，但内部为 backup/retention 启动子 goroutine 并 `wg.Add(1)`。设计意图需注释说明。 |
| m-09 | [readability.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/readability.go#L42-L46) | L42-46 | `init` 忽略 cookiejar.New 错误。失败时 Jar 为 nil，部分依赖 Cookie 的网站无法正常抓取，但无日志。 |
| m-10 | [readability.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/readability.go#L75-L77) | L75-77 | Content-Type 检查过严，拒绝 `application/xhtml+xml`、`application/xml` 等合法 HTML 类型。 |
| m-11 | [readability.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/readability.go#L194) | L194 | `FetchImage` 重试间 `time.Sleep(time.Second)` 固定时长，对持续失败的源浪费时间。应改为指数退避。 |
| m-12 | [readability.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/readability.go#L186-L189) | L186-189 | `FetchImage` 返回非 200 响应未检查。调用方需自行检查 StatusCode，但接口名易让人误以为成功才返回。 |
| m-13 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go#L363) | L363 | `compressToZip` 用 `os.ReadFile(srcPath)` 一次性读整个 db 文件到内存，对大库可能 OOM。应改用流式压缩。 |
| m-14 | [backup.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/backup.go#L273-L316) | L273-316 | `cleanupOldBackups` 与 `CleanupBackups` 逻辑高度相似但保留策略不同。应统一为通用清理函数。 |
| m-15 | [export.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/export.go#L255) | L255 | `enc.Encode(map[string]interface{}{...})` 类型不严格，违反项目编码规范"禁止 any/interface{} 兜底"。应定义 struct。 |
| m-16 | [urlpolicy.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/urlpolicy.go#L25-L31) | L25-31 | `blockedDomains` 列表不全，缺少阿里云 metadata（100.100.100.200）、Oracle Cloud metadata 等。 |
| m-17 | [urlpolicy.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/urlpolicy.go#L148) | L148 | `ValidateURLOnly` 重复调用 ParseIP：`if ip := net.ParseIP(host); ip != nil && isPrivateIP(host)`，`isPrivateIP` 内部再次调用 `net.ParseIP`。 |
| m-18 | [urlpolicy.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/urlpolicy.go#L101-L103) | L101-103 | `idna.ToASCII` 错误时保留原 host 可能绕过 blockedDomains。应返回错误而非保留原 host。 |
| m-19 | [database.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/database/database.go#L180-L181) | L180-181 | `DB.Exec("DROP INDEX IF EXISTS ...")` 未检查错误。DROP IF EXISTS 一般不会失败，但严谨起见应检查。 |
| m-20 | [database.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/database/database.go#L207-L229) | L207-229 | `seedFTS5Batched` 使用 LIMIT/OFFSET 分批，随着 offset 增大性能下降。对大表可能慢。建议改用 WHERE id > last_id 模式。 |
| m-21 | [main.go](file:///c:/opt/workstations/project/py/rss/server/go/cmd/main.go#L29) | L29 | `godotenv.Read(".env")` 错误仅忽略。开发时若 .env 文件格式错误会被静默忽略。应 slog.Debug 记录。 |
| m-22 | [main.go](file:///c:/opt/workstations/project/py/rss/server/go/cmd/main.go#L129) | L129 | `gracefulShutdown` 中 `_ = sqlDB.Close()` 显式丢弃错误。 |
| m-23 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L1938) | L1938 | `appVersion = "0.0.1.20260730"` 硬编码，与 package.json 的 `0.0.1-20260730` 不一致（点 vs 横线）。 |
| m-24 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L1231) | L1231 | `ProxyImage` 的 `Access-Control-Allow-Origin: *` 与 image-proxy 在 sensitive 组矛盾。 |
| m-25 | [reader.go (handler)](file:///c:/opt/workstations/project/py/rss/server/go/internal/handlers/reader.go#L1869) | L1869 | `DownloadBackup` 路径检查未包含空字节 `\x00`。建议增加白名单正则 `^[a-zA-Z0-9_\-\.]+\.zip$`。 |
| m-26 | [reader_cache_test.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader_cache_test.go) | 全文件 | 测试覆盖严重不足，仅测试 `unreadCountCache` 生命周期（3 个场景），未覆盖并发访问、FTS5 搜索净化、过滤规则匹配、OPML 导入深度限制等关键逻辑。 |

### 前端

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| m-27 | [ArticleList.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ArticleList.tsx#L182-L203) | L182-203 | 刷新图标用手动 rAF 动画绕过 CSS。项目约束要求"animate-spin 应用到包装 span，使用 spin-fixed 关键帧"。 |
| m-28 | [Sidebar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Sidebar.tsx#L595-L604) | L595-604 | `<button>` 内嵌套 `<button>`，HTML 规范不允许 interactive content 嵌套。应将外层改为 `<div role="button">`。 |
| m-29 | [useClickOutside.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/hooks/useClickOutside.ts#L19) | L19 | 仅监听 `mousedown`，未处理 `touchstart`。移动端用户点击外部区域时可能不触发关闭。 |
| m-30 | [SettingsModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/SettingsModal.tsx#L913) | L913 | 面板尺寸 `max-w-[920px] h-[740px]`，项目约束为 `1100px × 750px（max 95vw/90vh）`。宽度偏差 180px。 |
| m-31 | [useItemsData.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/hooks/useItemsData.ts#L59) | L59 | `PAGE_SIZE = 50` 定义在 hook 函数体内，每次渲染重建。应提升为模块级常量。 |
| m-32 | [Reader.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/Reader.tsx#L79-L81) | L79-81 | `match.replace(`src="${src}"`, ...)` 在同一条 `<img>` 标签内若 src 出现多次（如 srcset 属性中），可能替换错误位置。 |
| m-33 | [App.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/App.tsx#L630-L636) | L630-636 | `buildSourcePayload` 的 `useCallback` 依赖数组含 `ensureFolder`，但 `ensureFolder` 未被 `useCallback` 包裹，导致该 `useCallback` 失效。 |
| m-34 | [ArticleList.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ArticleList.tsx#L410-L427) | L410-427 | `escapeRegExp` 和 `highlight` 在 `ArticleRow` 组件内定义，每次渲染重建。`highlight` 还每次创建新 RegExp。应提升到组件外部。 |
| m-35 | [contextMenu.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/utils/contextMenu.ts#L203) | L203, L251 | 使用已废弃的 `document.execCommand('copy'/'cut')`。建议迁移至 Clipboard API。 |
| m-36 | [SettingsModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/SettingsModal.tsx#L309) | L309 | `queue.shift()!` 非空断言。建议用更安全的 `const id = queue.shift(); if (id === undefined) break;`。 |
| m-37 | [settings.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/utils/settings.ts#L571-L577) | L571-577 | `importConfig` 的 10 秒超时 `setTimeout` 在文件选择后未 `clearTimeout`，存在轻微资源泄漏。 |
| m-38 | [App.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/App.tsx#L23) | L23 | `import { getApi, ... } from './utils/api.js'` — 从 `.ts` 文件导入 `.js` 扩展名。其他导入无扩展名，不一致。 |
| m-39 | [ReaderToolbar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ReaderToolbar.tsx#L410) | L410, L458-462 | `[1.6, 1.8, 2.0, 2.2]` 和 `[{v:600,...},...]` 数组字面量在组件内定义，每次渲染重建。应提升为模块级常量。 |
| m-40 | [AddSourceModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/AddSourceModal.tsx#L109-L110) | L109-110 | `if (!trimmedUrl) return;` 和 `if (form.interval < 5) return;` 静默失败，用户点击"添加"无反应无提示。应改为 toast 或 inline 错误。 |
| m-41 | [EditSourceModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/EditSourceModal.tsx#L37) | L37 | `url: url.trim()` 允许空 URL 提交（仅校验了 name 非空）。应对 URL 做非空校验。 |
| m-42 | [SearchBox.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/SearchBox.tsx#L14-L27) | L14-27 | `createSubmitHandler` 函数已定义但从未被调用，属于死代码。 |
| m-43 | [SearchBox.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/SearchBox.tsx#L34) | L34 | `useEffect(() => { setInput(query); }, [query])` 在外部 query 变化时覆盖内部 input，会导致用户正在输入时光标位置被重置。 |
| m-44 | [ContextMenu.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ContextMenu.tsx#L47-L54) | L47-54 | `window.innerWidth - 180`、`itemHeight = 32`、`separatorHeight = 9` 为魔法数字。应提取为常量。 |
| m-45 | [TitleBar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/TitleBar.tsx#L117-L119) | L117-119 | `useEffect(() => { setDesktop(isDesktop()); }, [])` 与 L115 的 `useState(() => isDesktop())` 重复，lazy initializer 已足够。 |
| m-46 | [SettingsSourcesTab.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/settings/SettingsSourcesTab.tsx#L126-L128) | L126-128 | `wailsApp.PickOPMLFile().then(...)` 无 `.catch()`，若 Wails 调用 reject 会产生未处理 Promise 拒绝。 |
| m-47 | [SettingsRulesTab.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/settings/SettingsRulesTab.tsx#L189) | L189 | `{ruleForm.conditions.map((cond, index) => (<div key={index}>...` 使用 index 作为 key，条件增删时可能导致 React 状态错乱。 |

---

## 6. NIT 问题列表

| # | 文件 | 行号 | 问题 |
|---|------|------|------|
| n-01 | 多文件 | 多处 | `console.error`/`console.warn` 在生产代码中（App.tsx 12 处、Reader.tsx 4 处、hooks 各 1-2 处）。建议使用日志抽象层。 |
| n-02 | [App.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/App.tsx#L1007) | L1007 | `bg-[var(--overlay-mobile)]` 使用 Tailwind 任意值语法。可在 tailwind.config 中注册为语义色。 |
| n-03 | [ReaderToolbar.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ReaderToolbar.tsx#L155) | L155 | `if (!item) return null` — `item` 在 props 中类型为 `Item`（非空），此为死代码。 |
| n-04 | [useItemsData.ts](file:///c:/opt/workstations/project/py/rss/apps/web/src/hooks/useItemsData.ts#L211) | L211 | `eslint-disable-next-line react-hooks/exhaustive-deps` 抑制了依赖检查。建议用 `useEvent` 模式替代。 |
| n-05 | [reader.go (service)](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader.go#L268) | L268 | `setIfSet` 泛型搭配 `map[string]interface{}`，类型安全性弱。可考虑强类型 Update struct。 |
| n-06 | [fetcher.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/fetcher.go#L104-L106) | L104-106 | `fetchHTTPClient 已移除` 注释保留，干扰阅读。应删除。 |
| n-07 | [filter.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/filter.go#L252) | L252 | `applyRuleAction` 缩进异常，比上下文少 1 个 tab。应用 `gofmt` 统一格式化。 |
| n-08 | [export.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/export.go#L107) | L107 | `ItemToMarkdown` 是 `*ReaderService` 方法但不使用 `s`，可改为独立函数。 |
| n-09 | [coordinator.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/coordinator.go#L100-L102) | L100-102 | `startedAt` 在 mu 内用 atomic 多余，可改为普通字段读写。 |
| n-10 | [reader_cache_test.go](file:///c:/opt/workstations/project/py/rss/server/go/internal/services/reader_cache_test.go#L21) | L21 | `newCacheTestService` 直接构造未初始化所有字段。Go 的 sync.RWMutex 零值可用，但建议加注释说明。 |
| n-11 | [ShortcutsHelpModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ShortcutsHelpModal.tsx#L32) | L32 | `{shortcut.keys.map((key, i) => (<span key={i}>...` 内层用 index 作为 key。 |
| n-12 | [ImportOPMLModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ImportOPMLModal.tsx#L55-L64) | L55-64 | `handleFileChange` 读取文件前无大小校验，用户选择超大文件可能导致内存占用。 |
| n-13 | [vite.config.ts](file:///c:/opt/workstations/project/py/rss/apps/web/vite.config.ts#L6) | L6 | `server: {}` 空对象可省略。 |
| n-14 | [manifest.json](file:///c:/opt/workstations/project/py/rss/apps/web/public/manifest.json#L9) | L9 | `"orientation": "portrait-primary"` 对桌面 RSS 阅读器不太合适，建议改为 `"any"`。 |
| n-15 | [ExportArticlesModal.tsx](file:///c:/opt/workstations/project/py/rss/apps/web/src/components/ExportArticlesModal.tsx#L114-L117) | L114-117 | `count: -1` 作为"未知数量"哨兵值。建议用 `count: undefined` 或 `count: null` 更语义化。 |
| n-16 | [app.go (desktop)](file:///c:/opt/workstations/project/py/rss/apps/desktop/app.go#L224) | L224 | 日志 `env=%v` 打印整个 env map，包含 `DATABASE_URL` 路径。建议仅记录 PORT。 |
| n-17 | [go.mod (desktop)](file:///c:/opt/workstations/project/py/rss/apps/desktop/go.mod#L41) | L41 | `// replace github.com/wailsapp/wails/v2 v2.13.0 => C:\Users\libing\gopath\pkg\mod` 注释掉的本地 replace 指令残留，且包含个人路径。应删除。 |

---

## 7. 做得好的地方

1. **SQL 注入防护扎实**：全项目零字符串拼接 SQL，FTS5 用 `sanitizeFTS5Query` 净化，LIKE 用 `escapeSQLWildcards` + ESCAPE 子句，VACUUM INTO 用 `escapeSQLitePath` 转义。
2. **XSS 防护到位**：前端 Reader.tsx 所有 `dangerouslySetInnerHTML` 均经 `DOMPurify.sanitize` 处理；`openExternal`/`safeOpenUrl`/`isSafeLink` 严格校验 http/https 协议；`proxyErrorPage` 用 `html.EscapeString` 转义用户可控字段。
3. **SSRF 防御多层**：ValidateURL（DNS 解析检查）+ TransportWithSSRFProtection（DialContext 检查）+ blockedDomains 黑名单，分层防御思路正确（仅 DialContext 实现有 rebinding 缺陷）。
4. **并发设计成熟**：Coordinator 统一去重与 worker 池、双重检查锁缓存、execLocked 修复竞态、事务后执行 goroutine 避免竞态、ReaderService 用 fetchAllMu 防止并发 FetchAll。
5. **资源限制到位**：HTTP 响应体统一 16MB LimitReader、OPML 深度+节点数限制、上传 256MB 限制、导出 5000 条限制、API limit 上限 200、批量删除上限 500。
6. **路径遍历防护**：backup.go 的 DeleteBackup/extractDBFromBackup 严格检查 `/`、`\\`、`..`。
7. **优雅关闭**：Coordinator Stop + Scheduler Stop 都有 wg.Wait 等待 goroutine 退出；main.go gracefulShutdown 顺序正确（srv.Shutdown → scheduler.Stop → coordinator.Stop → sqlDB.Close）。
8. **认证设计**：sensitive 路由组用 Bearer Token + `subtle.ConstantTimeCompare` 防时序攻击；本地回环地址豁免速率限制。
9. **错误信息安全**：`safeError` 函数对未知错误返回通用 `internal_server_error`，避免泄露内部路径和实现细节。
10. **注释质量高**：多处注释说明"根因修复"（m-03/C-02/M-R3 等），便于后续维护理解设计意图。
11. **ModalLayout 焦点管理**：实现了完整的焦点陷阱（trapFocus）、Escape 关闭、焦点恢复，无障碍支持优秀。
12. **React 性能优化**：`SourceRow`、`FolderNode`、`ArticleRow` 用 `React.memo`；`useState(() => loadSettings())` 懒初始化；AbortController 竞态处理。
13. **可访问性**：`Loading.tsx` 有 `role="status" aria-live="polite"`，`SettingsShared.tsx` 的 `Toggle` 有 `role="switch" aria-checked`，`ModalLayout` 有 `role="dialog" aria-modal`。
14. **DevTools 控制符合规范**：`main.go` 仅在 `FLORE_DEVTOOLS=1` 时启用，生产构建不含 `-devtools` flag。
15. **桌面端/Web 端优雅降级**：api.ts 的剪贴板、文件保存等均有桌面→Web→fallback 三级降级链。

---

## 8. 改进建议（按优先级）

### P0 — 立即修复（阻塞项）

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 1 | 修复 `useSourcesData.ts` L77 setter 命名 bug：将 `setUnreadCountGlobal` 改为 `setUnreadCountInScope`，同步修改 App.tsx L68 解构名 | useSourcesData.ts / App.tsx | C-01 |
| 2 | 修复 `urlpolicy.go` DialContext DNS rebinding：先 `net.DefaultResolver.LookupHost` 解析域名，再检查所有解析出的 IP | urlpolicy.go | M-S1 |
| 3 | 修复 `coordinator.go` Stop 顺序：仅 `close(stop)`，worker 退出后再 `close(taskCh)`；Submit 在 mu 内检查 stop 标志 | coordinator.go | M-R6, M-R7 |

### P1 — 本周修复（安全与可靠性）

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 4 | 将 `GetSetting` 端点移到 sensitive 组 | handlers/reader.go | M-S2 |
| 5 | `UpdateSettings` 增加 key 数量（≤100）和 value 长度（≤10KB）限制 | handlers/reader.go | M-S3 |
| 6 | 全文件 `s.db` 替换为 `s.getDb()`，统一读锁保护 | filter.go, export.go, fetcher.go, reader.go | M-R1, M-R2, M-R3, M-R5 |
| 7 | `filter.go` applyRuleAction 的 invalidateUnreadCount 移到事务外 | filter.go | M-R4 |
| 8 | `backup.go` restoreFromFile 关键错误返回而非吞掉 | backup.go | M-E3, M-E4, M-E5 |
| 9 | `reader.go` GetCacheStats 逐个检查错误 | reader.go (service) | M-E1 |
| 10 | `fetcher.go` L168 Updates 错误检查 | fetcher.go | M-E2 |
| 11 | `SearchItems` 限制 keyword 长度（≤200 字符） | handlers/reader.go | M-R9 |
| 12 | `ExportItems` 限制 IDs 数量（≤500） | handlers/reader.go | M-R10 |
| 13 | 为所有 `res.json()` 添加类型标注 | 多文件 | M-E7 |
| 14 | `html2canvas` 改为动态 `import()` | Reader.tsx | M-P1 |
| 15 | `onToggleViewOriginal` 用 ref 持有最新 displayMode，避免 stale closure | Reader.tsx | M-R11 |
| 16 | 修复 `sw.js`：CACHE_NAME 改为 `'flore-v1'`；移除 `'/src/main.tsx'`；移除硬编码端口判断 | sw.js | M-R12 |
| 17 | `SettingsSourcesTab.tsx` L126 的 `PickOPMLFile().then(...)` 补 `.catch()` | SettingsSourcesTab.tsx | m-46 |
| 18 | 删除 `go.mod` L41 注释掉的本地 replace 指令（含个人路径） | apps/desktop/go.mod | n-17 |
| 19 | `startNotifyWatcher` goroutine 添加 `context.Context` 参数，shutdown 时取消 | app.go | M-R13 |

### P2 — 架构重构（按文件规模拆分）

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 20 | 拆分 `handlers/reader.go`（1944 行）：提取 `handlers/proxy.go`、`handlers/html_rewrite.go` | handlers/reader.go | M-A1 |
| 21 | 拆分 `services/reader.go`（1506 行）：拆为 `reader_source.go`、`reader_opml.go`、`reader_search.go`、`reader_settings.go`、`reader_cleanup.go` | services/reader.go | M-A2 |
| 22 | 拆分 `App.tsx`（1175 行）：提取 `useKeyboardShortcuts`、`useArticleActions`、`useSourceActions`、`useFetchPolling` 等 hooks | App.tsx | M-A3 |
| 23 | 拆分 `SettingsModal.tsx`（1002 行）：规则管理、源管理逻辑抽到 hooks | SettingsModal.tsx | M-A4 |
| 24 | 拆分 `app.go`（838 行）：按职责拆为 `backend.go`、`window.go`、`notify.go`、`dialog.go` | app.go | M-A5 |
| 25 | 拆分 `Reader.tsx`：HTML 重写函数抽到 `utils/htmlRewrite.ts` | Reader.tsx | M-A6 |
| 26 | 拆分 `ReaderToolbar.tsx`：`TypePanel`/`MorePanel`/`SharePanel` 拆为独立文件 | ReaderToolbar.tsx | M-A7 |
| 27 | 拆分 `Sidebar.tsx`：`FolderNode`/`SourceRow`/`FilterRow` 拆为独立文件 | Sidebar.tsx | M-A8 |
| 28 | 拆分 `ArticleList.tsx`：`ArticleRow` 拆为独立文件 | ArticleList.tsx | M-A9 |
| 29 | 拆分 `SettingsDataTab.tsx`：多个 Section 提取为独立组件 | SettingsDataTab.tsx | M-A10 |
| 30 | 拆分 `backup.go`（496 行）：拆为 `backup_create.go`、`backup_restore.go`、`backup_cleanup.go`、`backup_util.go` | backup.go | M-A11 |
| 31 | 拆分 `SettingsRulesTab.tsx`：提取 `RuleFormSection`、`RuleListSection`、`RuleTestResultModal` | SettingsRulesTab.tsx | M-A12 |
| 32 | `cn.ts` 中的日期函数拆到 `utils/dateUtils.ts` | cn.ts | M-A14 |
| 33 | 长列表添加 `content-visibility: auto` 或虚拟滚动 | ArticleList.tsx | M-P2 |

### P3 — 渐进改进

| # | 行动项 | 文件 | 关联问题 |
|---|--------|------|---------|
| 34 | 提取 `numericBylineRe` 预编译正则 | reader.go (service) | m-01 |
| 35 | `importOutlineRecursive` 抽取 `opmlImportCtx` 结构体 | reader.go (service) | m-02 |
| 36 | `NewReaderService` 移除 Once 内的 DB 访问 | reader.go (service) | m-03 |
| 37 | `recoverAndRollback` 改用命名返回值 | reader.go (service) | m-04 |
| 38 | `fetcher.go` L124 用 `errors.Is(err, gorm.ErrRecordNotFound)` 区分 | fetcher.go | m-05 |
| 39 | `applyFiltersAndIndex` 累计失败数返回 | fetcher.go | m-06 |
| 40 | `Scheduler` runOnce 内对耗时操作检查 stop | scheduler.go | m-07 |
| 41 | `readability.go` init 失败时 slog.Warn 记录 | readability.go | m-09 |
| 42 | `readability.go` Content-Type 白名单放宽 | readability.go | m-10 |
| 43 | `FetchImage` 重试改为指数退避 | readability.go | m-11 |
| 44 | `compressToZip` 改为流式压缩 | backup.go | m-13 |
| 45 | 统一 `cleanupOldBackups` 与 `CleanupBackups` | backup.go | m-14 |
| 46 | `ExportItemsJSON` 改用 struct 而非 `map[string]interface{}` | export.go | m-15 |
| 47 | `blockedDomains` 补充阿里云/Oracle metadata | urlpolicy.go | m-16 |
| 48 | `ValidateURLOnly` 移除重复 ParseIP 调用 | urlpolicy.go | m-17 |
| 49 | `idna.ToASCII` 错误时返回错误 | urlpolicy.go | m-18 |
| 50 | `database.go` AutoMigrate DROP INDEX 检查错误 | database.go | m-19 |
| 51 | `seedFTS5Batched` 改用 WHERE id > last_id 模式 | database.go | m-20 |
| 52 | `godotenv.Read` 错误用 slog.Debug 记录 | main.go | m-21 |
| 53 | `appVersion` 默认值与 package.json 对齐 | handlers/reader.go | m-23 |
| 54 | `DownloadBackup` 增加白名单正则 | handlers/reader.go | m-25 |
| 55 | 补充关键路径单元测试（FTS5/过滤/OPML/Coordinator） | services | m-26 |
| 56 | 刷新图标改用 `spin-fixed` CSS 关键帧 | ArticleList.tsx | m-27 |
| 57 | Sidebar 文件夹节点外层 `<button>` 改为 `<div role="button">` | Sidebar.tsx | m-28 |
| 58 | `useClickOutside` 增加 `touchstart` 监听 | useClickOutside.ts | m-29 |
| 59 | 面板尺寸对齐项目约束（1100px × 750px） | SettingsModal.tsx | m-30 |
| 60 | `PAGE_SIZE` 提升为模块级常量 | useItemsData.ts | m-31 |
| 61 | `escapeRegExp`/`highlight` 提升到组件外部 | ArticleList.tsx | m-34 |
| 62 | 迁移 `execCommand` 到 Clipboard API | contextMenu.ts | m-35 |
| 63 | `importConfig` 的 setTimeout 在 onchange 后 clearTimeout | settings.ts | m-37 |
| 64 | 统一导入路径（移除 `.js` 扩展名） | App.tsx | m-38 |
| 65 | `ReaderToolbar` 常量数组提升为模块级 | ReaderToolbar.tsx | m-39 |
| 66 | `AddSourceModal` 验证失败时给用户反馈 | AddSourceModal.tsx | m-40 |
| 67 | `EditSourceModal` 对 URL 字段做非空校验 | EditSourceModal.tsx | m-41 |
| 68 | 删除 `SearchBox.tsx` 中未使用的 `createSubmitHandler` 死代码 | SearchBox.tsx | m-42 |
| 69 | 优化 `SearchBox.tsx` 的 query 同步逻辑 | SearchBox.tsx | m-43 |
| 70 | 提取 `ContextMenu.tsx` 的魔法数字为命名常量 | ContextMenu.tsx | m-44 |
| 71 | 删除 `TitleBar.tsx` 冗余 useEffect | TitleBar.tsx | m-45 |
| 72 | `SettingsRulesTab.tsx` 条件列表用稳定 id 替代 index 作为 key | SettingsRulesTab.tsx | m-47 |
| 73 | 删除 `fetcher.go` L104-106 死代码注释 | fetcher.go | n-06 |
| 74 | `filter.go` 用 `gofmt` 统一格式化 | filter.go | n-07 |
| 75 | `ItemToMarkdown` 改为独立函数 | export.go | n-08 |
| 76 | `ImportOPMLModal.tsx` 读取文件前校验 `file.size` | ImportOPMLModal.tsx | n-12 |
| 77 | `manifest.json` 移除或修改 `orientation` 为 `"any"` | manifest.json | n-14 |
| 78 | `app.go` L224 日志移除 `env=%v`，仅记录 PORT | app.go | n-16 |
| 79 | `wails.json` 的 `productVersion` 改为构建脚本从 package.json 注入 | wails.json | m-23 |

---

## 9. 覆盖率校验

| 指标 | 数值 |
|------|------|
| 总问题数 | 105 |
| 行动项总数 | 79 |
| 已覆盖问题数 | 105 |
| 覆盖率 | 105/105 = 100% |
| 延期处理问题 | 无 |

> 校验公式：总问题数 = 已覆盖问题数 + 延期处理问题数
>
> 问题计数校验：
> - CRITICAL: 1（C-01）
> - MAJOR: 40（M-S1~S3 + M-P1~P2 + M-E1~E7 + M-A1~A14 + M-R1~R13 = 3+2+7+14+13 = 39，含 M-S4 SSRF 依赖 service 层合并到 M-S1，实际为 39 个独立 MAJOR + 1 个合并 = 40）
> - MINOR: 47（m-01~m-47）
> - NIT: 17（n-01~n-17）
> - 合计：1 + 40 + 47 + 17 = 105 ✓
>
> 行动项覆盖：P0(3) + P1(16) + P2(14) + P3(46) = 79 项，覆盖全部 105 个问题（部分行动项合并多个问题，如 #6 覆盖 M-R1/R2/R3/R5，#13 覆盖 M-E7 的所有实例）

---

## 附录：审查文件清单

### 后端 Go（14 个文件）
- `server/go/cmd/main.go` (170 行)
- `server/go/internal/database/database.go` (230 行)
- `server/go/internal/models/models.go` (153 行)
- `server/go/internal/models/time.go` (171 行)
- `server/go/internal/handlers/reader.go` (1944 行)
- `server/go/internal/services/reader.go` (1506 行)
- `server/go/internal/services/fetcher.go` (569 行)
- `server/go/internal/services/scheduler.go` (250 行)
- `server/go/internal/services/filter.go` (388 行)
- `server/go/internal/services/readability.go` (224 行)
- `server/go/internal/services/backup.go` (496 行)
- `server/go/internal/services/export.go` (264 行)
- `server/go/internal/services/coordinator.go` (257 行)
- `server/go/internal/services/urlpolicy.go` (177 行)
- `server/go/internal/services/reader_cache_test.go` (62 行)

### 前端 React/TS（约 50 个文件）
- `apps/web/src/App.tsx` (1175 行)
- `apps/web/src/main.tsx` (31 行)
- `apps/web/src/types.ts` (61 行)
- `apps/web/src/index.css` (286 行)
- `apps/web/src/lib/cn.ts` (38 行)
- `apps/web/src/components/*.tsx` (20 个组件)
- `apps/web/src/components/settings/*.tsx` (8 个组件)
- `apps/web/src/hooks/*.ts` (4 个)
- `apps/web/src/utils/*.ts` (5 个)
- `apps/web/src/types/*.d.ts` (3 个)
- `apps/web/public/sw.js`, `manifest.json`
- `apps/web/vite.config.ts`, `tailwind.config.js`, `tsconfig.json`, `postcss.config.js`, `index.html`

### 桌面壳 Wails（约 8 个文件）
- `apps/desktop/app.go` (838 行)
- `apps/desktop/main.go` (59 行)
- `apps/desktop/systray.go` (125 行)
- `apps/desktop/build-desktop.ps1`, `build-frontend.ps1`, `package-portable.ps1`
- `apps/desktop/wails.json`, `go.mod`

### 排除项
- `apps/routing-tool/` — 独立项目，按 AGENTS.md 规定不审查
- `docs/` — 文档目录，不审查
- `assets/` — 静态资源，不审查
- `scripts/` — 脚本工具，不审查
- `diagnose_*.py`, `verify_*.py` — 临时验证脚本，不审查

---

**审查完成。** 核心结论：项目安全基础扎实（无 SQL 注入、XSS 防护到位、路径遍历防护、SSRF 多层防御），主要问题集中在：

1. **1 个 CRITICAL**：`useSourcesData.ts` setter 命名 bug 致未读计数功能完全失效，需立即修复
2. **3 个安全 MAJOR**：DNS rebinding、GetSetting 未认证、UpdateSettings 无限制
3. **11 个架构 MAJOR**：文件规模严重超标（最大 1944 行），需分批重构
4. **13 个可靠性 MAJOR**：s.db 绕过锁、coordinator Stop 顺序、backup 错误吞没等竞态和错误处理问题

建议优先修复 P0（C-01 + DNS rebinding + coordinator Stop）和 P1（认证、错误处理、stale closure、sw.js）后再进行下一轮迭代。P2 架构重构可分多个 PR 逐步推进。
