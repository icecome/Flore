# Graph Report - rss  (2026-08-01)

## Corpus Check
- 110 files · ~92,870 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1058 nodes · 1624 edges · 67 communities (61 shown, 6 thin omitted)
- Extraction: 98% EXTRACTED · 2% INFERRED · 0% AMBIGUOUS · INFERRED: 27 edges (avg confidence: 0.78)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `c5d8b39c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_ReaderService|ReaderService]]
- [[_COMMUNITY_ReaderHandler|ReaderHandler]]
- [[_COMMUNITY_P1 - 重要问题|P1 - 重要问题]]
- [[_COMMUNITY_App|App]]
- [[_COMMUNITY_devDependencies|devDependencies]]
- [[_COMMUNITY_ItemWithSource|ItemWithSource]]
- [[_COMMUNITY_MilliTime|MilliTime]]
- [[_COMMUNITY_rss-fetcher.user.js|rss-fetcher.user.js]]
- [[_COMMUNITY_ArticleList.tsx|ArticleList.tsx]]
- [[_COMMUNITY_BaseFetcher|BaseFetcher]]
- [[_COMMUNITY_App.tsx|App.tsx]]
- [[_COMMUNITY_SettingsModal.tsx|SettingsModal.tsx]]
- [[_COMMUNITY_Flore — RSS Reader 设计上下文品牌分析|Flore — RSS Reader 设计上下文品牌分析]]
- [[_COMMUNITY_devDependencies|devDependencies]]
- [[_COMMUNITY_ExportArticlesModal.tsx|ExportArticlesModal.tsx]]
- [[_COMMUNITY_3. 前端 React 代码质量审查|3. 前端 React 代码质量审查]]
- [[_COMMUNITY_fetcher.go|fetcher.go]]
- [[_COMMUNITY_Reader.tsx|Reader.tsx]]
- [[_COMMUNITY_compilerOptions|compilerOptions]]
- [[_COMMUNITY_scripts|scripts]]
- [[_COMMUNITY_.ImportDatabase|.ImportDatabase]]
- [[_COMMUNITY_source.ts|source.ts]]
- [[_COMMUNITY_compilerOptions|compilerOptions]]
- [[_COMMUNITY_ARCHITECTURE.md — RSS Reader 系统架构|ARCHITECTURE.md — RSS Reader 系统架构]]
- [[_COMMUNITY_package.json|package.json]]
- [[_COMMUNITY_prisma.ts|prisma.ts]]
- [[_COMMUNITY_5. 全栈安全审查|5. 全栈安全审查]]
- [[_COMMUNITY_Go 后端规范|Go 后端规范]]
- [[_COMMUNITY_index.ts|index.ts]]
- [[_COMMUNITY_2. 后端 Go 代码质量审查|2. 后端 Go 代码质量审查]]
- [[_COMMUNITY_Contributing to Flore|Contributing to Flore]]
- [[_COMMUNITY_AGENTS.md — RSS Reader 项目 AI 行为总纲|AGENTS.md — RSS Reader 项目 AI 行为总纲]]
- [[_COMMUNITY_api.ts|api.ts]]
- [[_COMMUNITY_manifest.json|manifest.json]]
- [[_COMMUNITY_wails.json|wails.json]]
- [[_COMMUNITY_opml.ts|opml.ts]]
- [[_COMMUNITY_folder.ts|folder.ts]]
- [[_COMMUNITY_4. 桌面壳代码质量审查|4. 桌面壳代码质量审查]]
- [[_COMMUNITY_Security Policy|Security Policy]]
- [[_COMMUNITY_后续迭代提示词模板|后续迭代提示词模板]]
- [[_COMMUNITY_runtime.d.ts|runtime.d.ts]]
- [[_COMMUNITY_ErrorBoundary|ErrorBoundary]]
- [[_COMMUNITY_PROJECT.md — RSS Reader 产品意图|PROJECT.md — RSS Reader 产品意图]]
- [[_COMMUNITY_toast.ts|toast.ts]]
- [[_COMMUNITY_models.ts|models.ts]]
- [[_COMMUNITY_README|README]]
- [[_COMMUNITY_RSS Reader 全量代码审查报告|RSS Reader 全量代码审查报告]]
- [[_COMMUNITY_附录：审查文件清单|附录：审查文件清单]]
- [[_COMMUNITY_RssFeedFetcher|RssFeedFetcher]]
- [[_COMMUNITY_Flore RSS Reader — 前端主题设计提示词|Flore RSS Reader — 前端主题设计提示词]]
- [[_COMMUNITY_EventsOnMultiple|EventsOnMultiple]]
- [[_COMMUNITY_css.d.ts|css.d.ts]]
- [[_COMMUNITY_sw.js|sw.js]]
- [[_COMMUNITY_desktop|desktop]]
- [[_COMMUNITY_github.comrssgo-server|github.com/rss/go-server]]

## God Nodes (most connected - your core abstractions)
1. `ReaderHandler` - 51 edges
2. `ReaderService` - 41 edges
3. `safeError()` - 40 edges
4. `App` - 26 edges
5. `cn()` - 26 edges
6. `3. 前端 React 代码质量审查` - 21 edges
7. `compilerOptions` - 17 edges
8. `5. 全栈安全审查` - 16 edges
9. `2. 后端 Go 代码质量审查` - 15 edges
10. `compilerOptions` - 14 edges

## Surprising Connections (you probably didn't know these)
- `Reader()` --references--> `dompurify`  [EXTRACTED]
  apps/web/src/components/Reader.tsx → apps/web/package.json
- `EditSourceModal()` --calls--> `cn()`  [EXTRACTED]
  apps/web/src/components/EditSourceModal.tsx → apps/web/src/lib/cn.ts
- `ExportArticlesModal()` --calls--> `cn()`  [EXTRACTED]
  apps/web/src/components/ExportArticlesModal.tsx → apps/web/src/lib/cn.ts
- `ModalLayout()` --calls--> `cn()`  [EXTRACTED]
  apps/web/src/components/ModalLayout.tsx → apps/web/src/lib/cn.ts
- `Sidebar()` --calls--> `cn()`  [EXTRACTED]
  apps/web/src/components/Sidebar.tsx → apps/web/src/lib/cn.ts

## Import Cycles
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/generic-css.ts -> apps/routing-tool/src/scrapers/registry.ts`
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/chinawriter.ts -> apps/routing-tool/src/scrapers/registry.ts`
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/rss-feed.ts -> apps/routing-tool/src/scrapers/registry.ts`

## Communities (67 total, 6 thin omitted)

### Community 1 - "ReaderService"
Cohesion: 0.05
Nodes (24): Folder, Item, Response, FetchReadability(), ProxyOriginal(), escapeSQLWildcards(), firstNonEmpty(), DB (+16 more)

### Community 2 - "ReaderHandler"
Cohesion: 0.09
Nodes (8): Engine, HandlerFunc, ReaderHandler, authMiddleware(), Context, ReaderService, proxyErrorPage(), safeError()

### Community 3 - "P1 - 重要问题"
Cohesion: 0.04
Nodes (45): 1. 总体质量评估, 2. 代码复用决策阶梯报告, 3. 代码质量审查报告, 4. 代码安全审查报告, 5. 综合优先级排序, CRITICAL, Flore RSS Reader - 全栈审查报告, H1: 过滤规则 CRUD 双重实现 (+37 more)

### Community 4 - "App"
Cohesion: 0.07
Nodes (17): Context, Mutex, NewApp(), newLogFile(), main(), Cmd, App, BackendStatus (+9 more)

### Community 5 - "devDependencies"
Cohesion: 0.06
Nodes (32): dependencies, cheerio, cors, hono, @hono/node-server, node-cron, ofetch, rss (+24 more)

### Community 6 - "ItemWithSource"
Cohesion: 0.13
Nodes (17): FilterRule, ItemWithSource, escapeYamlString(), ReaderService, slugify(), applyRuleAction(), conditionMatches(), DB (+9 more)

### Community 7 - "MilliTime"
Cohesion: 0.10
Nodes (11): FilterRule, Folder, Item, ItemSearch, MilliTime, NullableMilliTime, ReadabilityCache, Source (+3 more)

### Community 8 - "rss-fetcher.user.js"
Cohesion: 0.15
Nodes (31): activate(), addResultItem(), autoScan(), buildConfig(), createHoverBox(), createOverlay(), createPanel(), createSourceViaApi() (+23 more)

### Community 9 - "ArticleList.tsx"
Cohesion: 0.12
Nodes (21): ArticleRow, FilterType, ContextMenuItem, MenuState, Props, IconProps, StarFilledIcon(), StarIcon() (+13 more)

### Community 10 - "BaseFetcher"
Cohesion: 0.12
Nodes (12): BaseFetcher, ScrapeResult, FetcherDefinition, ChinawriterFetcher, definition, fetcher, definition, fetcher (+4 more)

### Community 11 - "App.tsx"
Cohesion: 0.11
Nodes (20): App(), AddSourceModal(), Props, SourceFormData, ImportOPMLModal(), Props, ModalLayout(), Props (+12 more)

### Community 12 - "SettingsModal.tsx"
Cohesion: 0.10
Nodes (21): ConfirmDialog(), Props, TypePanel(), ACTION_LABELS, FIELD_LABELS, FONT_FAMILY_OPTIONS, IconBtn(), INTERVAL_OPTIONS (+13 more)

### Community 13 - "Flore — RSS Reader 设计上下文品牌分析"
Cohesion: 0.08
Nodes (25): 1. Obsidian（知识库工具风格）, 2. Apple（原生精致风格）, 3. Microsoft（Fluent Design / Office 风格）, 4.1 Notion（模块化工作空间）, 4.2 Reader by Readwise（阅读专注）, 4.3 Feedly（专业 RSS 聚合）, 4.4 Inoreader（高效信息处理）, 4.5 Reeder（macOS 经典 RSS） (+17 more)

### Community 14 - "devDependencies"
Cohesion: 0.09
Nodes (22): dependencies, dompurify, lucide-react, react, react-dom, devDependencies, autoprefixer, postcss (+14 more)

### Community 15 - "ExportArticlesModal.tsx"
Cohesion: 0.17
Nodes (17): Props, EditSourceModal(), Props, ExportArticlesModal(), ExportScope, Props, ScopeKey, Props (+9 more)

### Community 16 - "3. 前端 React 代码质量审查"
Cohesion: 0.10
Nodes (21): 3. 前端 React 代码质量审查, App.tsx, components/ArticleList.tsx, components/ContextMenu.tsx, components/ErrorBoundary.tsx, components/IconButton.tsx, components/icons.tsx, components/ModalLayout.tsx (+13 more)

### Community 17 - "fetcher.go"
Cohesion: 0.19
Nodes (16): FetchRSSFeed(), Name, ReaderService, Time, parseAtom(), parseFeedDate(), parseRSS(), atomAuthor (+8 more)

### Community 18 - "Reader.tsx"
Cohesion: 0.15
Nodes (14): EmptyState(), Props, IconButton, Props, ALL_BUTTONS, FONT_FAMILY_OPTIONS, FONT_STEPS, LETTER_SPACING_OPTIONS (+6 more)

### Community 19 - "compilerOptions"
Cohesion: 0.11
Nodes (18): compilerOptions, allowImportingTsExtensions, forceConsistentCasingInFileNames, isolatedModules, jsx, lib, module, moduleDetection (+10 more)

### Community 20 - "scripts"
Cohesion: 0.11
Nodes (18): description, devDependencies, concurrently, name, private, scripts, build, build:desktop (+10 more)

### Community 21 - ".ImportDatabase"
Cohesion: 0.15
Nodes (14): Reader, main(), AutoMigrate(), DBPath(), defaultReaderDBPath(), GetDB(), DB, Init() (+6 more)

### Community 22 - "source.ts"
Cohesion: 0.21
Nodes (12): feedRouter, getFetcher(), generateFeed(), applyFilterRules(), CreateSourceInput, deleteSource(), doFetchSource(), generateFeedPath() (+4 more)

### Community 23 - "compilerOptions"
Cohesion: 0.12
Nodes (16): compilerOptions, declaration, declarationMap, esModuleInterop, forceConsistentCasingInFileNames, module, moduleResolution, outDir (+8 more)

### Community 24 - "ARCHITECTURE.md — RSS Reader 系统架构"
Cohesion: 0.12
Nodes (16): API 接口, ARCHITECTURE.md — RSS Reader 系统架构, Go 后端分层（server/go/internal/）, 其他, 前端组件结构（apps/web/src/）, 后台流程：RSS 抓取, 数据库模型, 数据流向 (+8 more)

### Community 25 - "package.json"
Cohesion: 0.12
Nodes (15): author, bugs, url, description, homepage, keywords, license, main (+7 more)

### Community 26 - "prisma.ts"
Cohesion: 0.22
Nodes (6): FilterCondition, getConfig(), updateDefaultInterval(), startScheduler(), fetchSource(), prisma

### Community 27 - "5. 全栈安全审查"
Cohesion: 0.12
Nodes (16): 5. 全栈安全审查, CORS 与监听地址, CSP 配置, DevTools 泄露, .gitignore, iframe 安全, SQL 注入, SSRF (+8 more)

### Community 28 - "Go 后端规范"
Cohesion: 0.12
Nodes (15): Go 后端规范, Handler 层规约, Service 层规约, 代码结构, 前端规范, 命名约定, 文件组织, 构建规范 (+7 more)

### Community 29 - "index.ts"
Cohesion: 0.20
Nodes (11): app, start(), configRouter, fetcherRouter, opmlRouter, sourceRouter, FetcherParam, initRegistry() (+3 more)

### Community 30 - "2. 后端 Go 代码质量审查"
Cohesion: 0.13
Nodes (15): 2. 后端 Go 代码质量审查, cmd/main.go, go.mod, internal/database/database.go, internal/handlers/reader.go, internal/models/models.go, internal/models/time.go, internal/services/backup.go (+7 more)

### Community 31 - "Contributing to Flore"
Cohesion: 0.14
Nodes (13): Building from Source, Code of Conduct, Coding Standards, Contributing to Flore, Development Setup, Go, How to Contribute, License (+5 more)

### Community 32 - "AGENTS.md — RSS Reader 项目 AI 行为总纲"
Cohesion: 0.15
Nodes (12): AGENTS.md — RSS Reader 项目 AI 行为总纲, 代码规范建议, 关键路径, 前端（apps/web/）, 前端组件清单, 后端（server/go/）, 安全约定, 技术栈推荐 (+4 more)

### Community 33 - "api.ts"
Cohesion: 0.27
Nodes (9): TitleBar(), BackendStatus, getApi(), initApiBase(), isDesktop(), maybeWailsEnv(), openExternal(), setApiBase() (+1 more)

### Community 35 - "manifest.json"
Cohesion: 0.17
Nodes (11): background_color, description, display, icons, lang, name, orientation, scope (+3 more)

### Community 36 - "wails.json"
Cohesion: 0.18
Nodes (10): author, email, name, frontend:build, frontend:dev:serverUrl, frontend:dev:watcher, frontend:install, name (+2 more)

### Community 37 - "opml.ts"
Cohesion: 0.33
Nodes (9): buildOutlineXML(), escapeXml(), exportOPML(), importOPML(), importOutlines(), isFeed(), OPMLOutline, parseOutlines() (+1 more)

### Community 38 - "folder.ts"
Cohesion: 0.36
Nodes (7): folderRouter, createFolder(), CreateFolderInput, deleteFolder(), getAllFolders(), getFolder(), updateFolder()

### Community 39 - "4. 桌面壳代码质量审查"
Cohesion: 0.22
Nodes (9): 4. 桌面壳代码质量审查, app.go, build-desktop.ps1, build-frontend.ps1, go.mod, main.go, package-portable.ps1, test-portable.ps1 (+1 more)

### Community 40 - "Security Policy"
Cohesion: 0.22
Nodes (8): Reporting a Vulnerability, Reporting Security Issues, Response Timeline, Security Best Practices, Security Policy, Supported Versions, Thanks, Vulnerability Disclosure Policy

### Community 41 - "后续迭代提示词模板"
Cohesion: 0.22
Nodes (8): v0.dev 提示词 — Flore RSS Reader, 主提示词（复制此段）, 后续迭代提示词模板, 导出代码后, 细化 ArticleList, 细化 Reader, 细化 Sidebar, 需要上传的附件

### Community 42 - "runtime.d.ts"
Cohesion: 0.25
Nodes (7): EnvironmentInfo, NotificationAction, NotificationCategory, NotificationOptions, Position, Screen, Size

### Community 43 - "ErrorBoundary"
Cohesion: 0.25
Nodes (3): ErrorBoundary, Props, State

### Community 44 - "PROJECT.md — RSS Reader 产品意图"
Cohesion: 0.25
Nodes (7): PROJECT.md — RSS Reader 产品意图, 为谁而做, 数据策略, 明确拒绝（不做什么）, 架构原则, 核心体验, 这是什么

### Community 45 - "toast.ts"
Cohesion: 0.29
Nodes (3): ToastItem, toastStore, ToastType

### Community 47 - "README"
Cohesion: 0.40
Nodes (4): About, Building, Live Development, README

### Community 48 - "RSS Reader 全量代码审查报告"
Cohesion: 0.40
Nodes (4): 1. 优先修复建议（Top 10）, RSS Reader 全量代码审查报告, 修复进度概览, 目录

### Community 49 - "附录：审查文件清单"
Cohesion: 0.40
Nodes (5): 前端（30 个文件）, 后端（14 个文件）, 桌面壳（8 个文件）, 配置与安全, 附录：审查文件清单

### Community 51 - "Flore RSS Reader — 前端主题设计提示词"
Cohesion: 0.50
Nodes (3): Flore RSS Reader — 前端主题设计提示词, 使用说明, 提示词（英文，直接复制使用）

### Community 52 - "EventsOnMultiple"
Cohesion: 0.67
Nodes (3): EventsOn(), EventsOnce(), EventsOnMultiple()

## Knowledge Gaps
- **374 isolated node(s):** `name`, `version`, `description`, `main`, `types` (+369 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `ReaderService` connect `ReaderService` to `.ImportDatabase`, `MilliTime`?**
  _High betweenness centrality (0.037) - this node is a cross-community bridge._
- **Why does `ItemWithSource` connect `ItemWithSource` to `ReaderService`, `MilliTime`?**
  _High betweenness centrality (0.021) - this node is a cross-community bridge._
- **Why does `NewReaderService()` connect `.ImportDatabase` to `ReaderService`?**
  _High betweenness centrality (0.021) - this node is a cross-community bridge._
- **What connects `name`, `version`, `description` to the rest of the system?**
  _374 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `runtime.js` be split into smaller, more focused modules?**
  _Cohesion score 0.03076923076923077 - nodes in this community are weakly interconnected._
- **Should `ReaderService` be split into smaller, more focused modules?**
  _Cohesion score 0.052083333333333336 - nodes in this community are weakly interconnected._
- **Should `ReaderHandler` be split into smaller, more focused modules?**
  _Cohesion score 0.09335839598997493 - nodes in this community are weakly interconnected._