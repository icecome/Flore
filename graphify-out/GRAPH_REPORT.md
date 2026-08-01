# Graph Report - rss  (2026-08-01)

## Corpus Check
- 127 files · ~107,789 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1360 nodes · 2640 edges · 79 communities (70 shown, 9 thin omitted)
- Extraction: 98% EXTRACTED · 2% INFERRED · 0% AMBIGUOUS · INFERRED: 53 edges (avg confidence: 0.78)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `3cd6f9b8`
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
- [[_COMMUNITY_backend.go|backend.go]]
- [[_COMMUNITY_test-portable.ps1|test-portable.ps1]]
- [[_COMMUNITY_desktop|desktop]]
- [[_COMMUNITY_github.comrssgo-server|github.com/rss/go-server]]
- [[_COMMUNITY_main|main]]
- [[_COMMUNITY_App|App]]
- [[_COMMUNITY_ReadabilityResult|ReadabilityResult]]
- [[_COMMUNITY_PROJECT.md — RSS Reader 产品意图|PROJECT.md — RSS Reader 产品意图]]
- [[_COMMUNITY_AI 自定义指令|AI 自定义指令]]
- [[_COMMUNITY_RunMigrations|RunMigrations]]
- [[_COMMUNITY_newCacheTestService|newCacheTestService]]
- [[_COMMUNITY_saveUploadToTempFile|saveUploadToTempFile]]
- [[_COMMUNITY_TestCoordinatorNewItemsAccumulate|TestCoordinatorNewItemsAccumulate]]

## God Nodes (most connected - your core abstractions)
1. `ReaderService` - 84 edges
2. `ReaderHandler` - 73 edges
3. `App` - 53 edges
4. `safeError()` - 50 edges
5. `getApi()` - 40 edges
6. `showToast()` - 37 edges
7. `cn()` - 35 edges
8. `Folder` - 23 edges
9. `Source` - 23 edges
10. `SettingsDataTab()` - 21 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `NewApp()`  [INFERRED]
  apps/desktop/main.go → apps/desktop/app.go
- `Reader()` --references--> `dompurify`  [EXTRACTED]
  apps/web/src/components/Reader.tsx → apps/web/package.json
- `ArticleList()` --references--> `react`  [EXTRACTED]
  apps/web/src/components/ArticleList.tsx → apps/web/package.json
- `Props` --references--> `Folder`  [EXTRACTED]
  apps/web/src/components/AddSourceModal.tsx → apps/web/src/types.ts
- `ContextMenu()` --calls--> `cn()`  [EXTRACTED]
  apps/web/src/components/ContextMenu.tsx → apps/web/src/lib/cn.ts

## Import Cycles
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/generic-css.ts -> apps/routing-tool/src/scrapers/registry.ts`
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/chinawriter.ts -> apps/routing-tool/src/scrapers/registry.ts`
- 2-file cycle: `apps/routing-tool/src/scrapers/registry.ts -> apps/routing-tool/src/scrapers/sites/rss-feed.ts -> apps/routing-tool/src/scrapers/registry.ts`

## Communities (79 total, 9 thin omitted)

### Community 1 - "ReaderService"
Cohesion: 0.11
Nodes (3): RWMutex, Client, ReaderService

### Community 2 - "ReaderHandler"
Cohesion: 0.09
Nodes (5): ReaderHandler, Map, Context, ReaderService, safeError()

### Community 3 - "P1 - 重要问题"
Cohesion: 0.06
Nodes (32): 1. 现网案例调研, 2. 本项目可行性分析, 3.1 后端, 3.2 前端, 3.3 Bookmarklet（轻量级收集方案）, 3.4 浏览器扩展（进阶方案，后续阶段）, 3. 设计方案, 4. 实施路线图 (+24 more)

### Community 4 - "App"
Cohesion: 0.06
Nodes (13): Context, App, Mutex, NewApp(), newLogFile(), main(), Bool, CancelFunc (+5 more)

### Community 5 - "devDependencies"
Cohesion: 0.06
Nodes (32): dependencies, cheerio, cors, hono, @hono/node-server, node-cron, ofetch, rss (+24 more)

### Community 6 - "ItemWithSource"
Cohesion: 0.12
Nodes (21): FilterRule, ItemWithSource, applyScopeFilter(), escapeYamlString(), DB, ReaderService, slugify(), compareValues() (+13 more)

### Community 7 - "MilliTime"
Cohesion: 0.10
Nodes (11): FilterRule, Folder, Item, ItemSearch, MilliTime, NullableMilliTime, ReadabilityCache, Setting (+3 more)

### Community 8 - "rss-fetcher.user.js"
Cohesion: 0.18
Nodes (27): BACKUP_INTERVAL_OPTIONS, BackupEntry, CacheStats, DatabaseInfo, doCleanupArticles(), doCleanupExpiredBackups(), doClearReadability(), doCreateBackup() (+19 more)

### Community 9 - "ArticleList.tsx"
Cohesion: 0.06
Nodes (51): ArticleList(), ArticleRow, FilterType, getArticleListTitle(), ContextMenu(), ContextMenuItem, MenuState, Props (+43 more)

### Community 10 - "BaseFetcher"
Cohesion: 0.12
Nodes (12): BaseFetcher, ScrapeResult, FetcherDefinition, ChinawriterFetcher, definition, fetcher, definition, fetcher (+4 more)

### Community 11 - "App.tsx"
Cohesion: 0.07
Nodes (36): App(), AddSourceModal(), getInputClasses(), getSourceFormFolders(), Props, SourceForm(), SourceFormData, Props (+28 more)

### Community 12 - "SettingsModal.tsx"
Cohesion: 0.07
Nodes (50): SettingsAboutTab(), CLOSE_BEHAVIOR_OPTIONS, MINIMIZE_BEHAVIOR_OPTIONS, Props, SettingsNotifyTab(), NumberInput(), Row(), Section() (+42 more)

### Community 13 - "Flore — RSS Reader 设计上下文品牌分析"
Cohesion: 0.16
Nodes (25): Node, Request, Response, attrVal(), FetchCSS(), fetchHTMLWithClient(), FetchImage(), FetchReadability() (+17 more)

### Community 14 - "devDependencies"
Cohesion: 0.09
Nodes (22): dependencies, dompurify, html2canvas, lucide-react, react, react-dom, devDependencies, autoprefixer (+14 more)

### Community 15 - "ExportArticlesModal.tsx"
Cohesion: 0.14
Nodes (25): Props, EditSourceParams, Props, buildCurrentViewScope(), buildScopeByKey(), ExportArticlesModal(), getCurrentViewLabel(), Props (+17 more)

### Community 16 - "3. 前端 React 代码质量审查"
Cohesion: 0.13
Nodes (20): FetchStatusResponse, frameCheckResult, Regexp, fallbackToCSSProxy(), Client, URL, inlineCSSImports(), inlineExternalCSS() (+12 more)

### Community 17 - "fetcher.go"
Cohesion: 0.13
Nodes (24): FetchFeedTitle(), FetchRSSFeedWithClient(), filterValidItems(), Client, Context, DB, Item, Name (+16 more)

### Community 18 - "Reader.tsx"
Cohesion: 0.22
Nodes (14): EmptyState(), Props, escapeRegex(), fetchReadability(), PURIFY_CONFIG, Reader(), rewriteIframeContent(), rewriteImageUrls() (+6 more)

### Community 19 - "compilerOptions"
Cohesion: 0.11
Nodes (18): compilerOptions, allowImportingTsExtensions, forceConsistentCasingInFileNames, isolatedModules, jsx, lib, module, moduleDetection (+10 more)

### Community 20 - "scripts"
Cohesion: 0.11
Nodes (18): description, devDependencies, concurrently, name, private, scripts, build, build:desktop (+10 more)

### Community 21 - ".ImportDatabase"
Cohesion: 0.33
Nodes (10): AutoMigrate(), configureConnectionPool(), defaultReaderDBPath(), DB, Init(), resolveAbsolutePath(), resolveDatabasePath(), seedFTS5Batched() (+2 more)

### Community 22 - "source.ts"
Cohesion: 0.21
Nodes (12): feedRouter, getFetcher(), generateFeed(), applyFilterRules(), CreateSourceInput, deleteSource(), doFetchSource(), generateFeedPath() (+4 more)

### Community 23 - "compilerOptions"
Cohesion: 0.12
Nodes (16): compilerOptions, declaration, declarationMap, esModuleInterop, forceConsistentCasingInFileNames, module, moduleResolution, outDir (+8 more)

### Community 24 - "ARCHITECTURE.md — RSS Reader 系统架构"
Cohesion: 0.15
Nodes (17): appendPrefixStarsToWords(), buildUpdateMap(), cleanFTS5Keyword(), escapeSQLWildcards(), T, isFTS5SafeChar(), isLetterOrDigitOrUnicode(), normalizeInterval() (+9 more)

### Community 25 - "package.json"
Cohesion: 0.12
Nodes (15): author, bugs, url, description, homepage, keywords, license, main (+7 more)

### Community 26 - "prisma.ts"
Cohesion: 0.22
Nodes (6): FilterCondition, getConfig(), updateDefaultInterval(), startScheduler(), fetchSource(), prisma

### Community 27 - "5. 全栈安全审查"
Cohesion: 0.15
Nodes (3): applyItemStatusFilters(), applyTimelineFilters(), DB

### Community 28 - "Go 后端规范"
Cohesion: 0.12
Nodes (15): Go 后端规范, Handler 层规约, Service 层规约, 代码结构, 前端规范, 命名约定, 文件组织, 构建规范 (+7 more)

### Community 29 - "index.ts"
Cohesion: 0.20
Nodes (11): app, start(), configRouter, fetcherRouter, opmlRouter, sourceRouter, FetcherParam, initRegistry() (+3 more)

### Community 30 - "2. 后端 Go 代码质量审查"
Cohesion: 0.19
Nodes (10): filterSourcesToFetch(), Duration, Mutex, ReaderService, Source, Time, WaitGroup, NewScheduler() (+2 more)

### Community 31 - "Contributing to Flore"
Cohesion: 0.12
Nodes (16): API 接口, ARCHITECTURE.md — RSS Reader 系统架构, Go 后端分层（server/go/internal/）, 其他, 前端组件结构（apps/web/src/）, 后台流程：RSS 抓取, 数据库模型, 数据流向 (+8 more)

### Community 32 - "AGENTS.md — RSS Reader 项目 AI 行为总纲"
Cohesion: 0.09
Nodes (22): AGENTS.md — RSS Reader 项目 AI 行为总纲, 代码规范建议, 关键路径, 前端（apps/web/）, 前端组件清单, 各目录职责与约束, 后端（server/go/）, 安全约定 (+14 more)

### Community 33 - "api.ts"
Cohesion: 0.12
Nodes (27): SharePanel(), Runtime, TitleBar(), useRuntime(), useWindowDrag(), useWindowState(), useNotification(), UseNotificationParams (+19 more)

### Community 35 - "manifest.json"
Cohesion: 0.17
Nodes (11): background_color, description, display, icons, lang, name, orientation, scope (+3 more)

### Community 36 - "wails.json"
Cohesion: 0.12
Nodes (16): author, email, name, frontend:build, frontend:dev:serverUrl, frontend:dev:watcher, frontend:install, info (+8 more)

### Community 37 - "opml.ts"
Cohesion: 0.33
Nodes (9): buildOutlineXML(), escapeXml(), exportOPML(), importOPML(), importOutlines(), isFeed(), OPMLOutline, parseOutlines() (+1 more)

### Community 38 - "folder.ts"
Cohesion: 0.36
Nodes (7): folderRouter, createFolder(), CreateFolderInput, deleteFolder(), getAllFolders(), getFolder(), updateFolder()

### Community 39 - "4. 桌面壳代码质量审查"
Cohesion: 0.16
Nodes (13): ALL_BUTTONS, FONT_FAMILY_OPTIONS, FONT_STEPS, LETTER_SPACING_OPTIONS, ReaderToolbar(), ReaderToolbarProps, ToolbarButtonDef, ToolbarButtonId (+5 more)

### Community 40 - "Security Policy"
Cohesion: 0.18
Nodes (8): buildOPMLPath(), firstNonEmpty(), Name, resolveOPMLFolderID(), opmlBody, opmlDocument, opmlHead, opmlOutline

### Community 41 - "后续迭代提示词模板"
Cohesion: 0.17
Nodes (14): ACTION_LABELS, FIELD_LABELS, FIELD_OPTIONS, isSafeLink(), OPERATOR_LABELS, OPERATOR_OPTIONS, Props, RuleCondition (+6 more)

### Community 42 - "runtime.d.ts"
Cohesion: 0.25
Nodes (7): EnvironmentInfo, NotificationAction, NotificationCategory, NotificationOptions, Position, Screen, Size

### Community 43 - "ErrorBoundary"
Cohesion: 0.22
Nodes (3): ErrorBoundary, Props, State

### Community 44 - "PROJECT.md — RSS Reader 产品意图"
Cohesion: 0.17
Nodes (4): Int64, Mutex, WaitGroup, FetchCoordinator

### Community 45 - "toast.ts"
Cohesion: 0.18
Nodes (15): EditSourceModal(), IconBtn(), Props, SettingsSourcesTab(), SOURCE_FILTER_OPTIONS, ThemeSelector(), ToastContainer(), typeConfig (+7 more)

### Community 47 - "README"
Cohesion: 0.40
Nodes (4): About, Building, Live Development, README

### Community 48 - "RSS Reader 全量代码审查报告"
Cohesion: 0.26
Nodes (6): DBPath(), escapeSQLitePath(), ReaderService, getBackupDir(), BackupEntry, DatabaseInfo

### Community 49 - "附录：审查文件清单"
Cohesion: 0.20
Nodes (10): Engine, HandlerFunc, rateEntry, rateLimiter, authMiddleware(), Duration, Mutex, Time (+2 more)

### Community 51 - "Flore RSS Reader — 前端主题设计提示词"
Cohesion: 0.24
Nodes (8): Folder, SourceHealth, buildFolderTree(), buildOPMLOutlines(), Source, groupSourcesByFolder(), mergeSourceData(), sourceIDs()

### Community 52 - "EventsOnMultiple"
Cohesion: 0.67
Nodes (3): EventsOn(), EventsOnce(), EventsOnMultiple()

### Community 55 - "backend.go"
Cohesion: 0.26
Nodes (9): GetDB(), cleanupOldBackups(), compressToZip(), copyFile(), closeAndReleaseLocks(), DB, ReaderService, readMaxKeepFromDB() (+1 more)

### Community 59 - "test-portable.ps1"
Cohesion: 0.17
Nodes (5): FileSystemCreateWritableOptions, FileSystemFileHandle, FileSystemWritableFileStream, SaveFilePickerOptions, Window

### Community 67 - "main"
Cohesion: 0.23
Nodes (10): Config, Server, buildCORSConfig(), gracefulShutdown(), main(), resolvePort(), backoffSeconds(), ReaderService (+2 more)

### Community 69 - "ReadabilityResult"
Cohesion: 0.25
Nodes (5): Item, Time, isReadabilityCacheExpired(), isValidByline(), ReadabilityResult

### Community 70 - "PROJECT.md — RSS Reader 产品意图"
Cohesion: 0.29
Nodes (6): PROJECT.md — RSS Reader 产品意图, 为谁而做, 数据策略, 架构原则, 核心体验, 这是什么

### Community 71 - "AI 自定义指令"
Cohesion: 0.29
Nodes (6): AI 自定义指令, 一、沟通协议, 三、工程准则, 二、工作原则, 五、后置审查, 四、编码规范

### Community 72 - "RunMigrations"
Cohesion: 0.60
Nodes (4): Migration, getSchemaVersion(), RunMigrations(), setSchemaVersion()

### Community 73 - "newCacheTestService"
Cohesion: 0.60
Nodes (4): ReaderService, T, newCacheTestService(), TestUnreadCountCache_Lifecycle()

## Knowledge Gaps
- **311 isolated node(s):** `name`, `version`, `description`, `main`, `types` (+306 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **9 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `ReaderService` connect `ReaderService` to `ReadabilityResult`, `ItemWithSource`, `Security Policy`, `PROJECT.md — RSS Reader 产品意图`, `RSS Reader 全量代码审查报告`, `3. 前端 React 代码质量审查`, `Flore RSS Reader — 前端主题设计提示词`, `ARCHITECTURE.md — RSS Reader 系统架构`, `5. 全栈安全审查`?**
  _High betweenness centrality (0.042) - this node is a cross-community bridge._
- **Why does `NewReaderHandler()` connect `3. 前端 React 代码质量审查` to `ReaderHandler`, `main`?**
  _High betweenness centrality (0.023) - this node is a cross-community bridge._
- **Why does `NewReaderService()` connect `3. 前端 React 代码质量审查` to `ARCHITECTURE.md — RSS Reader 系统架构`, `ReaderService`, `main`?**
  _High betweenness centrality (0.023) - this node is a cross-community bridge._
- **What connects `name`, `version`, `description` to the rest of the system?**
  _311 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `runtime.js` be split into smaller, more focused modules?**
  _Cohesion score 0.03076923076923077 - nodes in this community are weakly interconnected._
- **Should `ReaderService` be split into smaller, more focused modules?**
  _Cohesion score 0.1111111111111111 - nodes in this community are weakly interconnected._
- **Should `ReaderHandler` be split into smaller, more focused modules?**
  _Cohesion score 0.0855094726062468 - nodes in this community are weakly interconnected._