# Flore — Architecture + Feature-Strategy Audit

**Scope**: `apps/web/src` (React 19 + Vite + TS), `server/go` (Gin + GORM + SQLite), `apps/desktop` (Wails v2.13), `apps/routing-tool` (read-only).
**Method**: full read of `App.tsx`, `reader.go`, `item_service.go`, `source_service.go`, `folder_service.go`, `sync_engine.go`, `filter.go`, `coordinator.go`, `scheduler.go`, `export.go`, `backup.go`, `migrations.go`; caller-grep of every registered route; field-usage trace of every suspected dead column.
**Verdict up front**: this is a **well-defended, poorly-partitioned** codebase. Security and correctness engineering is genuinely above average; the *shape* of the code is the liability. `ReaderService` is a god object with 6 independent caches and 3 background workers; `App.tsx` is the frontend equivalent. Feature effort is inverted: ~2,000 lines of sync infrastructure and ~900 lines of backup/diagnostic plumbing for a product whose core loop is "fetch, read, mark read."

---

## (A) Architecture Findings

### ARCH-01 — `ReaderService` is a god object, not an aggregate root · **Severity: Critical**

`services/reader.go:15-57` declares one struct holding:

| Field group | Lines | Independent concern |
|---|---|---|
| `db`, `dbMu` | 16-17 | connection + hot-swap lock |
| `unreadCountCache` + `unreadCountMu` + `unreadCountReady` | 22-24 | cache #1 |
| `cachedHTTPClient` + `cachedHTTPClientMu` + `cachedHTTPClientSeq` + `proxySettingsSeq` | 27-30 | cache #2 |
| `settingsCache` + `settingsCacheMu` + `settingsCacheSeq` + `settingsCacheLoad` | 34-37 | cache #3 |
| `coordinator *FetchCoordinator` | 41 | fetch scheduling |
| `syncEngine *SyncEngine` | 44 | remote sync |
| `filterRulesCache` + `filterRulesCacheMu` + `filterRulesCacheSeq` | 47-49 | cache #4 |
| `indexChan chan int` | 53 | FTS index pipeline |
| `indexOverflow chan int` | 56 | backpressure buffer |

Plus, by method-set (not by field), it also owns: FTS5 indexing + `ApplyFilterRules` in one transaction (`reader.go:173-235`), OPML import/export (`folder_service.go:141-401`), markdown/JSON export (`export.go:121-277`), backup/restore/zip (`backup.go`), article retention (`setting_service.go:238`), and remote-sync settings parsing (`sync_engine.go:81-107`).

`NewReaderService()` (`reader.go:69-75`) is a `sync.Once` **process singleton** — `readerServiceInstance`, `readerServiceOnce` (`reader.go:61-64`). Handlers get it via `NewReaderHandler()` (`handlers/reader.go:122-128`). This is a service locator bolted onto an "aggregate root" label.

**Why it breaks first**: the `dbMu` hot-swap. `setDb()` (`reader.go:109-113`) replaces the connection after a database restore, while `unreadCountCache`, `settingsCache`, `filterRulesCache`, and `cachedHTTPClient` are *not* invalidated in lockstep. `execLocked` (`reader.go:119-123`) was added specifically to close a race where `*gorm.DB` could be closed mid-`Exec` (comment at `reader.go:116-118`, ticket `m-03`). That fix addresses one symptom of a struct that owns both the connection and everything cached from it.

**Testability**: `NewReaderServiceWithDB` (`reader.go:79-88`) is real DI and the service tests use it. But it still calls `NewSyncEngine(s)` (`reader.go:86`), and `loadProxySettingsSeq` (`reader.go:96-101`) hits the settings cache during construction. You cannot test "item query building" without constructing the whole graph.

### ARCH-02 — `source_service.go` / `item_service.go` / `folder_service.go` are file partitions, not layers · **Severity: High**

All three files declare methods on the **same** `*ReaderService` receiver. There is no interface between them; they call each other's unexported helpers freely:

- `item_service.go:23` calls `s.getAllFolderAllDescendantIDs` — defined in `folder_service.go:78`.
- `item_service.go:221` calls `s.applyCountScopeFilters` — same file, but the folder-descendant expansion is the `folder_service.go` helper.
- `sync_engine.go:609` calls `s.CreateFolder` — defined in `folder_service.go:23`.
- `sync_engine.go:729` calls `s.invalidateUnreadCount` — defined in `source_service.go:104`.
- `backup.go:281` calls `s.ExportOPML` — defined in `folder_service.go:305`.
- `backup.go:594` calls `s.ImportOPML` — defined in `folder_service.go:141`.

The AGENTS.md claim that `services` is "业务逻辑层" with "handler → service → model" (`AGENTS.md` §3.1) is true at the package level and false at the type level: there is exactly **one** service type. Test files follow the file partition (`source_service_test.go`, `item_service_test.go`, `folder_service_test.go`) but every one of them constructs the same `ReaderService`, so the "layers" are untestable in isolation.

**Concrete blast radius**: adding a new source-level concern means touching a 393-line file (`source_service.go`) that already mixes SSRF validation (`:144-163`), a generic `setIfSet` reflection helper (`:185-189`), feed-title fetching (`:213-221`), transaction-based batch delete (`:292-338`), health/backoff state machine (`:361-388`), and an OPML-typeless placeholder comment at `:191` (`// GetFolders 获取所有文件夹` sits directly above `type CreateSourceRequest struct` — a doc comment orphaned by a code move).

### ARCH-03 — Frontend state: `App.tsx` is a 1,082-line closure-synchronisation engine · **Severity: Critical**

Measured: **19 `useState`** + **10 `useRef`** (not the ~24 the brief estimated; the count is lower because some state was already extracted into hooks) and **~30 `useCallback`**.

`useState` sites: `App.tsx:56` (settings), `:71` (serverSettingsLoaded), `:113` selectedSourceId, `:114` selectedFolderId, `:115` selectedItem, `:116` filter, `:126` loadingReader, `:127` showAddModal, `:129` moveSource, `:130` editSource, `:131` renameTarget, `:132` showImportModal, `:133` showShortcutsHelp, `:134` showExportModal, `:135` focusedItemId, `:136` multiSelectMode, `:137` selectedIds, `:138` isMobile, `:139` mobileView, `:140` showMobileSidebar, `:141` showCreateFolder, `:142` createFolderName, `:144` confirmAction. (That is 23 declarations; `:71` and `:126` are state — **23 `useState` confirmed**.)

`useRef` sites: `:92` startupUpdateCheckDoneRef, `:104` autoDownloadStartedRef, `:143` createFolderInputRef, `:180` abortRef, `:329` itemsRef, `:446` keyboardHandlerRef, `:463` fetchersRef, `:533` foldersRef, `:748` importReportRef, `:778` prevRefreshingRef. That is **10 `useRef`**.

Four of them exist purely to defeat stale closures:

- `itemsRef` (`:329-332`) — synced in a `useEffect`.
- `foldersRef` (`:533-536`) — synced in a `useEffect`.
- `keyboardHandlerRef` (`:446-452`) — written **during render** (`keyboardHandlerRef.current = (e) => {...}` is a bare assignment statement in the component body, not in an effect).
- `fetchersRef` (`:463-480`) — also written **during render** (the assignment at `:472-480` duplicates the initialiser at `:463-471`, seven fields listed twice).

**The two conventions contradict each other, and the file documents the contradiction.** `App.tsx:328` says *"避免渲染期写 ref（React 并发渲染下不纯）：改为 effect 中同步最新值"* (avoid writing refs during render; use an effect). `App.tsx:461-462` says *"与 keyboardHandlerRef 相同惯例：每次渲染直接更新 current（幂等赋值），无需逐项 effect 同步"* (same convention as keyboardHandlerRef: update `current` during every render). Both cannot be the house rule. Under React 19 concurrent rendering, render-phase ref mutation (`:447`, `:472`) is exactly the impurity the first comment warns about.

**What breaks first**: `refreshing`-driven polling. `useFetchOrchestration` (`hooks/useFetchOrchestration.ts:73-115`) restarts its polling effect on `[refreshing]`, and its `tick` closure captures `refreshAllDataRef`/`notifyFetchCompleteRef` that are themselves refreshed in a `useEffect` (`:39-42`). Combined with `App.tsx:779-801` — an import-report effect that fires on the `refreshing` **falling edge** and re-fetches all sources — the fetch-completion path has three independent "latest value" mechanisms (ref-during-render, ref-in-effect, effect-on-transition). A missed transition silently drops the import toast; a duplicated transition double-notifies.

### ARCH-04 — Prop-drilling depth is 3, not 4; the rule is violated by *hooks*, not props · **Severity: Medium**

Counted precisely: `App → Sidebar → SettingsModal → SettingsXxxTab` is **depth 4** for settings (`App.tsx:845` → `Sidebar.tsx:566` → `SettingsModal.tsx:480` → `SettingsGeneralTab`). For sources it is depth 4 as well (`App.tsx:845` → `Sidebar.tsx:566` → `SettingsModal.tsx:487` → `SettingsSourcesTab`).

`Sidebar` has **28 props** (`Sidebar.tsx:35-67`); `SettingsModal` has **9** (`SettingsModal.tsx:48-58`); `SettingsSourcesTab` receives **21** (`SettingsModal.tsx:487-509`). `ArticleList` receives **30** (`App.tsx:922-959`). Passing props through `Sidebar` that it does not use (`onSourcesChanged`, `onMarkAllRead`, `onClearFolderSources`, `onFetchSource` are handed straight to `SettingsModal`, `Sidebar.tsx:570`) is pure tunnel drilling.

**The AGENTS.md rule is violated in spirit, not letter.** §5.2 forbids Redux/Zustand; §5.2 also says "App.tsx：状态管理中心，所有数据获取在此完成" (all data fetching happens in App.tsx). But `useItemsData` (`hooks/useItemsData.ts:100-210`) and `useSourcesData` (`hooks/useSourcesData.ts:33-76`) do the fetching, and `useUpdate.ts` is a **module-level external store** with a subscriber set — functionally a hand-rolled Zustand (`useUpdate()` is consumed at `App.tsx:91`, `useUpdate.ts` is 95 lines). The project already has a state library; it just wrote it itself instead of importing 1.2 kB of Zustand.

### ARCH-05 — The `RemoteProvider` abstraction is real but shallow; the file provider is vapour · **Severity: High**

The interface (`sync/provider.go:110-128`) has 2 conforming implementations — `greader.go` (449 lines) and `miniflux.go` (192 lines) — and `SyncEngine` *does* call through `provider.PullSubscriptions/PullItems/PushStates/PushSubscription` (`sync_engine.go:331, 343, 417, 487`). So the core abstraction holds.

But it leaks in three places:

1. **`provider.Name()` branching** — `sync_engine.go:223-235`:
   ```go
   func (e *SyncEngine) computePullSince(provider rsssync.RemoteProvider) time.Time {
       lastPullMs := int64(e.service.GetSettingInt("sync_last_pull_at", 0))
       since := time.UnixMilli(lastPullMs).Add(-time.Minute)
       if provider.Name() != rsssync.ProviderGReader { return since }
       e.fullPullCount++
       if e.fullPullCount < greaderFullPullEvery { return since }
       e.fullPullCount = 0
       return time.Time{}
   }
   ```
   This is protocol-specific cursor policy (`greaderFullPullEvery = 10`, `:218`) living in the engine, keyed on a string. The right home is a `CursorPolicy()` method on the provider.
2. **`provider.Name()` is written as data** — `sync_engine.go:336` passes `provider.Name()` into `alignSubscriptions` and stores it as `Source.remoteOrigin` (`:552`). A display name is being used as a persisted foreign key.
3. **Enum declared but unimplemented** — `ProviderTTRSS = "ttrss"` (`provider.go:16`) is referenced by neither `buildSyncProvider` (`sync_engine.go:90-106`, which only handles miniflux/greader) nor `syncAPIProviders` (`sync_config.go:52-55`). Dead constant, and the `ErrUnsupported` sentinel (`provider.go:20`) is never returned by anything.

**The `FileProvider` interface (`provider.go:173-185`) has zero implementations.** Yet the settings surface for it is fully built: `SettingsSyncTab.tsx` is **568 lines**, `sync_config.go` exposes 11 S3/WebDAV fields (`:27-36`), and `TestFileConnection` (`sync_engine.go:250-262`) implements WebDAV inline:

```go
func (e *SyncEngine) testWebDAVConnection() error {
    req, err := http.NewRequest("PROPFIND", webdavURL, nil)   // sync_engine.go:274
    req.Header.Set("Depth", "0")
    ...
}
```
WebDAV protocol code (`sync_engine.go:266-295`) lives in the sync engine, not in `internal/sync/webdav.go`, and does **not** go through `FileProvider`. The interface is a design document that was never implemented; the code that exists bypasses it. Meanwhile `sync_config.go:57-60` declares `syncFileProviders = {"webdav", "s3"}` as *allowed* — so the backend validates a config that has no runtime.

### ARCH-06 — Duplicated logic is systemic, not incidental · **Severity: High**

See table (B). Headline: **5 unread-count implementations**, **3 scope-filter implementations**, **2 OPML serialisers**, **6 backup-name validators**, **2 shortcut tables**, **2 `resolveSrc`/`buildProxyUrl` pairs**, **2 `escapeRegExp`/`escapeRegex`**, **2 fetch-status transition detections**.

### ARCH-07 — API surface: 82 routes, 17 with zero callers · **Severity: High**

Registered: **82** route lines in `handlers/reader.go:186-302` (81 under `/api` + `/health`).

Zero callers in `apps/web/src` **and** `apps/desktop` (verified by grep for path fragment and for the handler method name):

| # | Route | Handler | Evidence |
|---|---|---|---|
| 1 | `POST /api/sources/:id/fetch` | `FetchSource` | used only via `triggerFetch` at `api.ts:504`? No — that path *is* called, but only by `SettingsModal.tsx:267`; the *folder* variant is dead (see below). Recount: reachable. |
| 2 | `POST /api/sources/:id/read-all` | `MarkSourceAllRead` | `MarkSourceAllRead` → grep: **NONE** |
| 3 | `POST /api/folders/:id/clear` | `ClearFolderSources` | `api.ts:436` clearFolderSources → **called only from `App.tsx:737`? no**: grep `/clear\`` = `api.ts:436` only; `clearFolderSources` is exported but **never imported** |
| 4 | `POST /api/folders/:id/fetch` | `FetchFolder` | `/folders/:id/fetch` → **NONE** |
| 5 | `POST /api/items/read-later/clear` | `ClearReadLater` | `read-later/clear` → **NONE** |
| 6 | `GET /api/items/:id` | `GetItem` | no `getItem(` wrapper in `api.ts` |
| 7 | `POST /api/items/:id/index-search` | `IndexItemForSearch` | `IndexItemForSearch` → **NONE** |
| 8 | `POST /api/items/index-search/rebuild` | `RebuildSearchIndex` | `RebuildSearchIndex` → **NONE** |
| 9 | `POST /api/items/:id/apply-filters` | `ApplyFilterRules` | `apply-filters` → **NONE** |
| 10 | `GET /api/items/:id/export.md` | `ExportItemMarkdown` | `getItemMarkdownUrl` (`api.ts:375`) exported, **never imported** |
| 11 | `GET /api/css-proxy` | `ProxyCSS` | `css-proxy` → **NONE** |
| 12 | `GET /api/stats/unread` | `GetUnreadCount` | `stats/unread` → **NONE** (frontend uses `/items/count?unread=true`) |
| 13 | `GET /api/database/info` | `GetDatabaseInfo` | `database/info` → **NONE** |
| 14 | `POST /api/database/vacuum` | `VacuumDatabase` | `database/vacuum` → **NONE** |
| 15 | `GET /api/cache/stats` | `GetCacheStats` | `cache/stats` → **NONE** |
| 16 | `POST /api/cache/clear-readability` | `ClearReadabilityCache` | **NONE** |
| 17 | `POST /api/cache/rebuild-search` | `RebuildSearchIndex` | **NONE** (duplicate registration of #8) |
| 18 | `GET /api/backups/:name/contents` | `GetBackupContents` | `/contents` → only `apps/desktop/backend.go:363` (internal streamer) |
| 19 | `GET /api/backups/:name/restore-config` | `RestoreConfigFromBackup` | `restore-config` in `api.ts:586` via `restoreBackup` — reachable |
| 20 | `GET /api/diagnostic/items` + `POST /api/diagnostic/generate` + `GET /api/diagnostic/:path/download` | `GetDiagnoseItems`, `GenerateDiagnosticPackage`, `ExtractDiagnosticPackage` | `diagnostic` → **NONE** in web; **NONE** in `apps/desktop/*.go` |

**18 routes with no reachable caller.** Note two are literal duplicates: `POST /api/items/index-search/rebuild` (`reader.go:224`) and `POST /api/cache/rebuild-search` (`reader.go:268`) both bind `h.RebuildSearchIndex`.

**API design verdict: accumulated, not designed.** The evidence is structural:
- **Verb-in-path pairs**: 4 route pairs encode a boolean in the URL (`/read` + `/unread`, `/star` + `/unstar`, `/read-later` + `/unread-later`) — 6 handlers forwarding to 3 `toggleX(c, bool)` helpers (`item_handlers.go:179-277`). The client already passes the boolean (`api.ts:333-345` uses a ternary to pick the path).
- **Three inconsistent create verbs**: `POST /sources/create` (`reader.go:195`) but `POST /folders` (`:199`) and `POST /filter-rules` (`:232`).
- **Mixed plural/singular**: `/api/stats/unread` vs `/api/items/count`.
- **Auth group by vibes, not policy**: `/api/settings/:key` is sensitive (`:291`) because settings *may* contain proxyUrl, but `/api/database/info` (`:265`) and `/api/cache/stats` (`:266`) are open.

### ARCH-08 — Data-model hygiene: three dead fields, one dead migration, one orphan association · **Severity: High**

| Field | Declared | Written | Read | Verdict |
|---|---|---|---|---|
| `Source.ListRule` | `models.go:18` (`not null`) | `source_service.go:228`, `folder_service.go:286`, `sync_engine.go:566` — **always the literal `"rss"`** | **nowhere** | **Dead.** Write-only constant. |
| `Source.Interval` | `models.go:19` (`default:120`) | `source_service.go:229`, `folder_service.go:287`, `sync_engine.go:567` | **nowhere in scheduling** | **Dead.** `Scheduler.pollPeriodMinutes` (`scheduler.go:168-176`) reads the global `fetchInterval` setting only. |
| `SourceHealth.NextCheckAtUnix` | `models.go:51` (with an `index` tag!) | **never** — `source_service.go:374` explicitly documents "不再写入" | nowhere | **Dead column + dead index**, kept alive by a no-op migration. |
| `SourceHealth.NextRetryAtUnix` | `models.go:48` | `source_service.go:373,382` | `coordinator.go:172` | **Live** — correct. |
| `Item.Source *Source` | `models.go:73` (`foreignKey:SourceID`, `constraint:OnDelete:CASCADE`) | n/a | **never preloaded** — every query hand-joins (e.g. `item_service.go:66-67`) | **Orphan association.** The CASCADE constraint exists in AutoMigrate but nothing uses `Preload`. |

**Migration strategy verdict: 5 versions, 2 of which cancel each other out.**

- v1: implicit, never declared in `migrations.go:27-38` (the slice starts at v2).
- **v2** (`migrate_pubdate.go:12-69`): parses `pubDate` text with `time.Parse(time.RFC3339Nano, ...)` (`:48`).
- **v3** (`migrate_v3_next_check.go:9-13`): a **no-op**. The full body is `slog.Info(...)` + `return nil`. Its own comment admits "新列由 GORM AutoMigrate 自动创建，此迁移仅确保版本号一致." A schema-version bump that does nothing is worse than no migration — it makes the version number a lie.
- v4 (`migrate_v4_indexes.go`): indexes.
- **v5** (`migrate_v5_pubdate_strftime.go:17-28`): re-does v2. The doc comment at `:9-15` explains exactly why: *"v2 迁移使用 time.Parse(RFC3339Nano) 解析，无法处理 GORM 默认 time.Time 序列化产生的空格分隔符格式 … 导致大量 text 数据始终被跳过."*

**What v2→v5 says about the process**: a data migration was written, tested against a hand-crafted input format, shipped, and its failure mode (rows silently skipped, counted in a `skipped` variable at `migrate_pubdate.go:51` that nobody surfaced) went undetected until users reported "最新文章沉底". The v2 migration *had* the skip counter and logged it at `:67` — but a log line is not a validation gate. There is no post-migration assertion anywhere in `migrations.go`. Also note the version-comparison bug surface at `migrations.go:75-80`: a database from a *newer* build is a hard error ("downgrade not supported"), which for a local-first single-user app is a self-inflicted brick.

### ARCH-09 — `Folder.ParentID`: full-tree backend, half-tree frontend · **Severity: Medium**

Backend supports arbitrary nesting end-to-end: BFS descendant resolution (`folder_service.go:78-101`), recursive OPML import (`:194-234`) with depth/node guards `opmlMaxDepth = 32` / `opmlMaxNodes = 10000` (`:104-109`), recursive OPML export (`:362-401`), promotion-on-delete (`folder_service.go:45`, `source_service.go:349`). Frontend renders recursively too (`Sidebar.tsx:317-360` `renderFolderNode` calling itself via `renderChildFolder`, `:697-784`).

**But `CreateFolder` has no parent parameter anywhere in the chain**: `CreateFolder(name string)` (`folder_service.go:23`) ← `CreateFolderRequest` has only `Name` (`handlers/reader.go` folder POST → `source_handlers.go`) ← `api.ts:422` `createFolder(name: string)` ← `App.tsx:541, 586`. There is no "new subfolder" UI. **Nested folders are reachable only by importing a nested OPML.** A folder tree you can only create by importing a file is a feature that exists for exactly one code path.

### ARCH-10 — Route/handler layer: `ReaderHandler` holds a rate limiter and a frame cache · **Severity: Medium**

`ReaderHandler` (`handlers/reader.go:110-119`) carries `service`, `rateLimiter`, `baseURL`, and `frameChecker *services.FrameChecker` (a cache with FIFO eviction). `RegisterRoutes` (`:175`) *constructs* the rate limiter as a side effect of route registration and starts a goroutine (`reader.go:38`). `rateLimiter.stop()` (`:62`) is never called from `backend.go` — a goroutine leak on shutdown that matters only in tests, but it demonstrates that lifecycle is managed nowhere. The handler layer is documented as "只做：参数解析 → 调用 service → 返回 JSON" (`AGENTS.md` §5.1) and it holds a bounded cache and a background ticker.

### ARCH-11 — Settings are stored twice with no reconciliation rule · **Severity: Medium**

`loadSettings()` reads `localStorage` (`utils/settings.ts:306-326`), `loadSettingsFromServer()` reads the backend (`:346-356`), and `App.tsx:72-86` merges server over local by simply assigning (`setSettings(serverSettings)` at `:80`) then writing back (`saveSettings` at `:81`). `saveSettings` (`:414-423`) writes localStorage synchronously and fires `putServerSettings` **without awaiting** (`void saveSettingsToServer(settings)` at `:422`). Every settings toggle is therefore a potential last-writer-wins race between two stores, with no version vector and no conflict surface.

---

## (B) Duplication Inventory

| # | Pattern | Instances | Location | Divergence risk |
|---|---|---|---|---|
| D-01 | **Unread-count** | **5** | (1) `item_service.go:203-214` `GetUnreadCount` — global, `hideInTimeline` + optional `isPrivate`; (2) `item_service.go:217-227` `CountItems` — scope-aware, with `unread=true`; (3) `source_service.go:81-99` `computeUnreadCountMap` — `GROUP BY sourceId`; (4) `App.tsx:262-270` `updateSourceUnreadCount` — client-side `±1` optimistic delta; (5) `Sidebar.tsx:69-87` `computeFolderUnread` — recursive client-side tree sum | **Already divergent.** (3) has **no** `hideInTimeline`/`isPrivate` filter at all, so sidebar per-source badges include private/hidden sources that (1) and (2) exclude. (4) clamps at `Math.max(0, ...)` while the server has no clamp. |
| D-02 | **Scope filter (source/folder/timeline/hidePrivate)** | **3** | `item_service.go:15-37` `buildItemQuery`; `item_service.go:229-248` `applyCountScopeFilters`; `export.go:48-88` `buildExportQuery` + `applyScopeFilter` | **Already divergent.** `buildExportQuery` (`export.go:54-63`) adds a *second* folder predicate after `applyScopeFilter` already added `Source.folderId IS NOT NULL` (`:73`) — the comment at `:71-72` admits the override. `Item.sourceId` vs `Source.folderId` column qualification also differs across the three. |
| D-03 | **Folder-descendant expansion call sites** | **4** | `item_service.go:23`, `item_service.go:235`, `item_service.go:259`, `item_service.go:193`, `export.go:56` — all call `getAllFolderAllDescendantIDs` and all implement the same 6-line error branch (`slog.Error` + `Where("1 = 0")` vs `fmt.Errorf` wrap). Two *different* error policies for the same failure. |
| D-04 | **OPML serialisation** | **2** | Backend: `folder_service.go:112-133` + `:305-401` (types + `ExportOPML`). Other app: `apps/routing-tool/src/services/opml.ts:56,91` (`importOPML`/`exportOPML`) |**Cross-app duplication** — two independent OPML implementations in one repo, sharing only the format. |
| D-05 | **Shortcut table** | **2** | `ShortcutsHelpModal.tsx:8-18` (9 entries) and `SettingsShortcutsTab.tsx:6-25` (9 entries, grouped differently) | Both list the same 9 keys in different orderings. Adding one shortcut requires editing two files or the two disagree. |
| D-06 | **Regex-escape** | **2** | `Reader.tsx:221-223` `escapeRegex`; `ArticleList.tsx:494` `escapeRegExp` | Byte-identical body, different names. |
| D-07 | **`resolveSrc` + `buildProxyUrl`** | **2 pairs (4 fns)** | `Reader.tsx:90-108` (inside `rewriteImageUrls`) and `Reader.tsx:175-201` (inside `rewriteSrcset`) | Byte-identical 19-line `resolveSrc` and 5-line `buildProxyUrl`, duplicated *within one file*, 85 lines apart. |
| D-08 | **Backup-name validation** | **6** | `backup.go:107`, `:461`, `:479`, `:519`, `:561`, `db_handlers.go:158` | **Already divergent.** Five use `Contains("/") \|\| Contains("\\") \|\| Contains("..") \|\| !HasSuffix(".zip")`; the sixth (`backup.go:479`) drops the `.zip` check into a *separate* `if` (`:482`) and returns a different error string (`"invalid backup file type"` vs `"invalid backup name"`). `safeError` (`handlers/reader.go:333-346`) then maps these to different client messages. |
| D-09 | **Image content-type allowlist** | **2 concepts, 1 function** | `handlers/reader.go:313-327` `isAllowedImageContentType` (strips `;` params, 7 exact types, rejects SVG); `services/readability.go:300-350` `FetchImage` sets `Accept: …image/svg+xml…` (`:321`) and its caller enforces the handler's allowlist only if it happens to flow through `proxy_handlers.go:129/231` | The **fetch** layer advertises SVG acceptance; the **serve** layer rejects it. Works today, fragile by construction. |
| D-10 | **`fetching` falling-edge detection** | **2** | `App.tsx:778-801` (`prevRefreshingRef`, effect on `[refreshing]`) and `hooks/useFetchOrchestration.ts:88-91` (`if (!data.fetching) { …refresh…; setRefreshing(false) }`) | Two independent transition detectors on the same edge. The hook one is authoritative; the App one fires on its downstream `refreshing` state. Ordering between them is incidental. |
| D-11 | **Count-fetch parameter builders** | **4** | `useItemsData.ts:100-118`, `:121-134`, `:137-150`, `useSourcesData.ts:58-76` | Four separate `URLSearchParams` builders that each re-append `sourceId`/`folderId`/`hidePrivate`. The comment at `useItemsData.ts:108` ("不添加 filter 参数") encodes a rule that lives only in a comment. |
| D-12 | **`CreateSource` construction** | **3** | `source_service.go:224-233`, `folder_service.go:282-292`, `sync_engine.go:562-573` | Three literal `models.Source{...}` with the same `ListRule:"rss"`, `Interval:120`, `Active:true` triple. Any new required column needs three edits. |
| D-13 | **`backoffSeconds` / `nextRetryUnix`** | 1 impl, **1 shadow** | `coordinator.go:197-220` is the implementation; `sync_engine.go:205-213` `effectivePullIntervalLocked` re-implements exponential backoff (`1 << min(schedFailures,4)`, cap 30 min) with a *different* base and cap | Same concept, two constants sets, two semantics. |
| D-14 | **Markdown/filename slug** | 2 | `export.go:191-203` `slugify` + `:163-185` `GenerateSafeFilename` vs `Reader.tsx:380` `item.title.slice(0, 50)` for PNG names | Same goal, one sanitises, one truncates raw. |

---

## (C) Actual structure vs. documented structure

### What AGENTS.md / ARCHITECTURE.md claim

```
backend/ ──► handlers/ ──► services/ ──► models/
                  │            │
                  │            ├─ database/
                  │            └─ sync/
   handler: 参数解析 → service → JSON   (禁止业务逻辑)
   service: 业务逻辑编排                 (禁止 Gin 类型)
   "ReaderService 聚合根"
```

### What the code actually is

```
                    ┌──────────────────────────────────────────────────────┐
   backend.go ──────┤  process-wide singleton (reader.go:61-64)            │
   (assembly)       │  var readerServiceInstance *ReaderService            │
                    │  var readerServiceOnce     sync.Once                 │
                    └───────────────┬──────────────────────────────────────┘
                                    │ NewReaderService()  (reader.go:69)
                                    ▼
   ┌─────────────────────────── ReaderService (reader.go:15-57) ───────────────────────────┐
   │  db + dbMu  ← hot-swappable connection; 6 caches hang off the SAME struct              │
   │  ┌────────────┬──────────────┬──────────────┬───────────────┬────────────────────┐   │
   │  │unreadCount │settingsCache │filterRules   │cachedHTTP     │indexChan(2048)     │   │
   │  │Cache       │+ seq         │Cache + seq   │Client + seq   │indexOverflow(8192) │   │
   │  └────────────┴──────────────┴──────────────┴───────────────┴────────────────────┘   │
   │                                                                                        │
   │  coordinator *FetchCoordinator ── SyncEngine ──┐                                       │
   │  (reader.go:41)          (reader.go:44)        │                                       │
   │                                                │                                       │
   │  Method set = UNION of everything below, one type:                                     │
   │  ├── source_service.go   (393 L)  CRUD, SSRF, health/backoff, batch delete             │
   │  ├── item_service.go     (523 L)  queries, FTS5, readability cache, markdown export    │
   │  ├── folder_service.go   (403 L)  tree ops, **OPML import AND export**                 │
   │  ├── filter.go           (403 L)  rules CRUD + matching + action application           │
   │  ├── export.go           (277 L)  **article** export (zip/json)                        │
   │  ├── backup.go           (652 L)  VACUUM INTO, zip, restore, 6× name validation        │
   │  ├── sync_engine.go      (744 L)  remote sync + inline WebDAV PROPFIND                 │
   │  ├── readability.go      (406 L)  full-text extraction + image fetch                   │
   │  └── setting_service.go  (278 L)  kv store + **article retention cleanup**             │
   │                                                                                        │
   │  Background goroutines owned by this type: indexWorker, indexDrainer (reader.go:92-93) │
   └────────────────────────────────────────────────────────────────────────────────────────┘

   Handlers (reader.go:110-119) hold: service + rateLimiter(+its own goroutine)
                                            + baseURL + frameChecker (bounded cache)
   → i.e. the "no business logic" layer owns a lifecycle and a cache.

   internal/sync ── real: RemoteProvider (2 impls)      ── vapour: FileProvider (0 impls)
                └─ ProviderTTRSS constant (provider.go:16) referenced by nothing
                └─ WebDAV implemented **outside** the interface (sync_engine.go:266-295)
```

**Frontend actual:**

```
App.tsx (1082 L) — 23 useState, 10 useRef, ~30 useCallback, 3 ref-sync conventions
  ├── useUpdate.ts ................. de-facto external store (violates "no state lib")
  ├── useSourcesData / useItemsData . fetch logic NOT in App.tsx (violates §5.2)
  ├── useFetchOrchestration ......... polling + completion transition
  ├── fetchersRef (App:463/472) ..... WRITTEN DURING RENDER
  ├── keyboardHandlerRef (App:447) .. WRITTEN DURING RENDER
  ├── itemsRef / foldersRef (App:329/533) ... effect-synced  ← opposite convention
  └── Sidebar (28 props, 4 unused tunnels) → SettingsModal (9) → SettingsXxxTab (21)
        └── Sidebar ALSO owns: expandedFolders (localStorage), showSettings, settingsTab
```

---

## (D) Feature value/effort matrix

Effort in lines of code (Go `server/go`, TS `apps/web/src`, shell `apps/desktop`), with evidence.

| Feature | User value | Impl. cost (LOC) | Value/effort | Notes |
|---|---|---|---|---|
| Fetch / parse RSS+Atom (`fetcher.go` 564 + `coordinator.go` 274 + `scheduler.go` 242) | **Core** | 1,080 | **1.00** | The product. Well-built: dedup, backoff, panic recovery (`coordinator.go:258-268`). |
| Read + render 3 modes (`Reader.tsx` 646 + `ReaderToolbar` 675 + `readability.go` 406 + `proxy_content.go` 197) | **Core** | 1,924 | **0.70** | Justified — image anti-hotlink rewriting (`Reader.tsx:87-219`) and DOMPurify hardening (`:43-58`) are real value. |
| Organize: sources/folders/tags (`Sidebar` 784 + `ArticleList` 695 + `source_service` 393 + `folder_service` 403 + `item_service` 523) | **Core** | 2,798 | **0.55** | But ~40% of `folder_service.go` is OPML, and `item_service.go` carries export + readability cache. |
| **Remote sync** (`sync_engine` 744 + `sync_config` 125 + `greader` 449 + `miniflux` 192 + `SettingsSyncTab` 568 + `sync_handlers` 91 + `useSyncStatus` 41) | **Low** for a local-first app | **2,210** | **0.15** | Larger than the fetcher. Plus a `FileProvider` interface with 0 impls and 11 S3/WebDAV config fields. |
| **Backup / restore** (`backup.go` 652 + `backup_restore` 160 + `SettingsDataTab` 188 + `BackupList` 87 + `BackupPolicyForm` 100 + `useBackups` 137) | **Medium** | **1,324** | **0.30** | 6 duplicated validators, 10 routes, 3 of them with no caller. |
| **Diagnostic packages** (`diagnostic.go` 283) | **Low** | **283** | **0.05** | **Zero callers** in web or desktop. Entirely dead. |
| **Export formats** (`export.go` 277 + `ExportArticlesModal` 202 + `contextMenu.ts` 312 partial) | Medium | ~600 | **0.35** | zip-MD + JSON + single-file MD route (dead) + PNG (`Reader.tsx:362`) + PDF-via-print-window (`:392`). |
| Filter rules (`filter.go` 403 + `SettingsRulesTab` 319 + `RuleTestModal` 75) | Medium | ~800 | **0.35** | Rule application duplicated: `ApplyFilterRules` (`filter.go:227`) vs inline in `indexBatch` (`reader.go:213-233`). |
| Themes / accents (`SettingsAppearanceTab` 210 + `settings.ts` presets 480 partial + `index.css` 262) | Low-Med | ~500 | **0.20** | 4 reader themes × light/dark = 8 variants (`settings.ts:14-70`) + 9 accent presets (`:480-490`). |
| Updater (`apps/desktop/internal/updater/*` ~1,100 + `updatelet` 164 + `UpdateBadge` 89 + `useUpdate` 95) | Medium (desktop only) | **~1,450** | **0.25** | Includes zip extraction, manifest verification, Windows apply. |
| **Article retention** (`setting_service.go:238-320` + `RetentionForm` 89) | Medium | ~170 | **0.60** | Cheap and useful — one of the few well-priced features. |

**Allocation verdict: unhealthy, ~2.5 : 1 against the core loop.**

Sync + backup + diagnostic = **3,817 LOC**. Fetch + read + organize = **5,802 LOC**, of which the *organize* third is inflated by non-organize concerns living in the same files. Normalising: roughly **40% of backend effort went to features that a local-first single-user RSS reader either cannot use (sync, for users with no Miniflux account) or cannot reach (diagnostic, 0 callers).**

The tell is not the LOC ratio — it is that `SettingsSyncTab.tsx` (568 lines) is **larger than `ArticleList.tsx`'s entire search+multiselect+infinite-scroll feature set** and **2× `Sidebar.tsx`'s folder-tree logic**, in a product whose stated positioning (`AGENTS.md` §1) is "本地优先 … 附带 Web 版".

---

## (E) Missing features, ranked

Ranked by **(expected value to THIS product) ÷ (cost)**. "Expected value" is argued from the product's own positioning, not from a generic 2026 checklist.

**E-01 — Per-source refresh interval + pause/disable UI. Cost: ~150 LOC. Verdict: REAL GAP, #1.**
The backend is *already built and abandoned*. `Source.Interval` is a live, indexed-by-default column (`models.go:19`) with a working validator (`normalizeInterval`, `source_service.go:258-266`) and a live update path (`UpdateSourceRequest.Interval` at `:140`, wired through `setSourceBasicFields` at `:176`). `Source.Active` (`models.go:20`) is honoured by the coordinator (`coordinator.go:165`, `:225`). **The UI simply never sends either field.** `SettingsSourcesTab` (296 lines) has hide/move/delete/retry but no interval control and no pause toggle. A user with a 5-minute news feed and a monthly blog gets one global 30-minute poll (`scheduler.go:168-176`). This is a shipped-but-unplugged feature, not a missing one.

**E-02 — `J`/`K` navigation. Cost: ~10 LOC. Verdict: REAL GAP, #2.**
`App.tsx:364-413` handles ArrowDown/ArrowUp; `'j'`/`'k'` do not appear anywhere in `apps/web/src` (grep: zero matches). **The app already promises them**: `Reader.tsx:549` renders the empty state *"从左侧列表挑选一篇文章，或按 J / K 上下浏览。"* ("…or press J/K to browse up and down"). It advertises a keybinding that does not exist. The help modal (`ShortcutsHelpModal.tsx:9`) is honest and lists only arrows — so the product contradicts itself in two screens.

**E-03 — Mark-unread on scroll-back. Cost: ~40 LOC. Verdict: REAL GAP, #3.**
`markReadMode` supports `'scroll'` (`settings.ts:84`, `:495`) implemented with an `IntersectionObserver` in `ArticleList.tsx:214-239`. But it is **one-directional**: there is no un-observe/re-mark path. `handleToggleRead` (`App.tsx:334-351`) and `updateItemReadState` (`:258-260`) only ever move forward from the observer. Combined with `updateSourceUnreadCount`'s clamp at `Math.max(0, ...)` (`App.tsx:266`), scrolling past and back leaves the item permanently read with no recovery. NetNewsWire, Reeder, and every 2026 mail client do this.

**E-04 — Per-source unread badge correctness. Cost: ~20 LOC. Verdict: REAL GAP = a BUG, #4.**
`computeUnreadCountMap` (`source_service.go:81-99`) has **no `hideInTimeline` and no `isPrivate` filter**. So a source the user has explicitly hidden from the timeline still shows an unread badge in the sidebar. The global count (`item_service.go:203-214`) and the scoped count (`:217-227`) both apply those filters. The three numbers disagree by construction, and the sidebar is the one users stare at. This is the D-01 divergence, promoted to a finding because it is user-visible.

**E-05 — Nested folder creation UI. Cost: ~60 LOC. Verdict: REAL GAP, #5.**
Backend fully supports it (ARCH-09); `CreateFolder(name)` (`folder_service.go:23`) just never learned about parents. The Sidebar already renders the tree (`Sidebar.tsx:317`). Users who import a nested OPML get a tree they cannot extend by hand — they can only delete it.

**E-06 — Sync unread state to / from a real backend for Web users.** **Verdict: NICE-TO-HAVE, and the effort already spent makes it worse.**
`SettingsSyncTab.tsx` (568 lines) and 2,210 LOC of sync exist for users who *have* Miniflux/GReader. For a local-first app this is a differentiator only at the margin; the correct 2026 answer is "export/import the DB file" (already built, `database/export` + `/restore`) plus the `readlater-sync-design.md` / `server-sync-design.md` docs sitting in `docs/planning/`.

**E-07 — Article retention surfacing.** **Verdict: ALREADY PRESENT, not a gap.**
`CleanupArticles` (`setting_service.go:238`) + `RetentionForm.tsx` (89 lines) + a daily scheduler tick (`scheduler.go:230-269`) + `POST /articles/cleanup` are all live. The brief's hypothesis is wrong here — this one is done.

**E-08 — Global media hotkeys (play/pause for podcasts/YouTube embeds). Verdict: NICE-TO-HAVE, do not build.**
Flore renders `iframe` in a sandboxed frame (`Reader.tsx:631-639`) with an origin-isolating sandbox when proxied. Cross-origin media control requires a postMessage bridge into arbitrary third-party players — high cost, low reliability, and irrelevant to an RSS *reader* whose value is text.

**E-09 — Feed discovery (`<link rel="alternate">` autodiscovery from a site URL). Verdict: REAL GAP, missing from the brief, medium priority.**
`CreateSource` requires an exact feed URL; it fetches the title only to *name* the source (`source_service.go:213-221`). Pasting `https://example.com` (no `/feed`) fails with `ValidateURL`-then-parse, and the user gets a generic error. This is the single highest-friction moment in onboarding.

**E-10 — Keyboard shortcut customisation. Verdict: NICE-TO-HAVE, low.**
`SettingsShortcutsTab.tsx` is 28 lines of static rows (`ShortcutRow keys="↑ / ↓"`). It is named "Settings…" but is a read-only reference; the real reference is `ShortcutsHelpModal`. Two static tables for one concept (D-05).

**E-11 — Full-text search on Chinese.** **Verdict: ALREADY MITIGATED.** FTS5 uses `porter unicode61` (`ARCHITECTURE.md:163`) which does not segment Chinese, but `SearchItems` falls back to LIKE on title/desc (`item_service.go:339-343`, `searchLikeFallback` at `:380-392`) with proper wildcard escaping (`escapeSQLWildcards`, `:314-319`). Correct call.

**E-12 — OPML "import as new / merge" choice. Verdict: REAL GAP, medium.**
`ImportOPML` (`folder_service.go:141`) always merges by creating folders keyed on path (`:248-267`) and never dedups URLs — unlike `SyncEngine.alignSubscriptions` (`sync_engine.go:536-539`) which *does* dedup by URL. So importing the same OPML twice duplicates every source, while a sync alignment merges. Two importers, two semantics, and the user-facing one is the worse.

---

## (F) Top 5 Architecture Risks & Top 5 Feature-Strategy Mistakes

### Top 5 Architecture Risks

1. **R-01 — `ReaderService` god object with unilateral cache invalidation (ARCH-01).** Six caches + a hot-swappable `db` in one struct. `setDb` (`reader.go:109`) invalidates nothing. `invalidateUnreadCount` (`source_service.go:104-109`) is called from **9 sites** (`item_service.go:101,120,269`, `source_service.go:309`, `fetcher.go:513`, `filter.go:270`, `sync_engine.go:729`, `backup_restore.go:109,137`) — every new write path must remember to call it. The one that forgets produces a stale sidebar with no error. **Fix first**: split caches into their own types behind interfaces; make invalidation a subscription, not a call.

2. **R-02 — `App.tsx` closure-sync with two contradictory conventions (ARCH-03).** 10 refs, 4 of them "latest closure" mechanisms, 2 written during render (`:447`, `:472`) and 2 in effects (`:330`, `:534`). React 19 concurrent rendering makes render-phase ref writes unsafe; the file's own comment at `:328` says so while `:461` says the opposite. **Highest-probability production bug** is the import-report edge detector (`:779-801`) silently not firing.

3. **R-03 — Three scope filters that already disagree, and one of them is user-visible (ARCH-06/D-02/E-04).** `buildItemQuery` (`item_service.go:15`), `applyCountScopeFilters` (`:229`), and `buildExportQuery` (`export.go:48`) each independently re-derive source/folder/timeline/private semantics. `computeUnreadCountMap` (`source_service.go:81`) omits two of the four predicates. There is no test that asserts the three agree.

4. **R-04 — Schema hygiene: a dead column with an index, kept alive by a no-op migration (ARCH-08).** `NextCheckAtUnix` (`models.go:51`) is never written, carries `index`, and has migration v3 (`migrate_v3_next_check.go`) whose entire body is a log line. `ListRule` is a write-only `not null` constant. `Item.Source` has a CASCADE constraint nobody uses. Migrations have no post-condition assertions — which is precisely how v2 shipped broken and needed v5 to redo it (`migrate_v5_pubdate_strftime.go:9-15`).

5. **R-05 — 18 of 82 routes are dead, 2 are duplicates, and one whole subsystem (diagnostic, 283 LOC) has zero callers (ARCH-07).** Dead routes are not neutral: they are unattested attack surface (several are non-sensitive: `/api/database/info`, `/api/cache/stats`, `/api/css-proxy`) and they inflate the mental model. `POST /api/items/index-search/rebuild` and `POST /api/cache/rebuild-search` bind the same handler via two names.

### Top 5 Feature-Strategy Mistakes

1. **M-01 — Building a 2,210-LOC remote-sync stack for a local-first app, including an unimplemented `FileProvider` interface and 11 S3/WebDAV config fields (ARCH-05, D).** `SettingsSyncTab.tsx` is 568 lines; `sync_engine.go` is 744. The `FileProvider` contract (`provider.go:173-185`) has **zero implementations**, yet WebDAV PROPFIND is hand-rolled *outside* it (`sync_engine.go:266-295`) and `sync_config.go:57-60` lists `s3` as an allowed provider. Shipping config UI for a non-existent backend is the clearest symptom of effort spent on the roadmap instead of the product.

2. **M-02 — Leaving `Source.Interval` and `Source.Active` fully implemented but unreachable from the UI (E-01).** A validated, persisted, coordinator-honoured per-source refresh interval exists and no user can set it. This is the *worst* kind of debt: the cost is already paid, the value is uncollected, and it requires ~150 LOC to fix.

3. **M-03 — Advertising a keyboard shortcut that does not exist (`Reader.tsx:549` promises J/K; grep finds no `'j'`/`'k'` handler).** Ten lines of code, a direct product-integrity failure, and it appears in the *empty state* — the first screen a new user sees.

4. **M-04 — Duplicated feature surfaces instead of one good one**: two OPML implementations (`folder_service.go` + `routing-tool/src/services/opml.ts`), two shortcut tables (`ShortcutsHelpModal` + `SettingsShortcutsTab`), two OPML import semantics (merge-only-by-path in the reader at `folder_service.go:248-267` vs URL-dedup in sync at `sync_engine.go:536-539`), two export formats plus a third dead one (`/items/:id/export.md`). Each duplicate is a place where the two halves will silently diverge.

5. **M-05 — 283 lines of diagnostic-package generation with zero callers, in a product whose support model is "local-first, no server."** `diagnostic.go` builds and serves zip packages (`reader.go:297-299`); neither `apps/web` nor `apps/desktop` references any of the three routes. For a local-first app the diagnostic artifact *is* the database export, which already exists. This is roadmap-driven scope in a repo that also has a documented, deliberate `hai-razor-frontend-review.md` arguing for exactly this kind of cut (`docs/planning/hai-razor-frontend-review.md:120,286,416`) — the review was written and not acted on.

---

## Appendix — The 2-apps problem (question 9)

**Verdict: the coupling direction is clean; the *semantic* coupling is not.**

`apps/routing-tool` is a **separate Node process with its own Prisma database** (`src/services/source.ts:1-6`) that talks to the Go backend over HTTP as a client: `const GO_API = process.env.GO_API_URL || 'http://localhost:3002/api'` (`source.ts:6`). It does not import Go code, and `server/go` does not import it. The OPML handoff is file/format-level, not module-level. **That boundary is correct.**

**But the reader imported routing-tool baggage.** The evidence:

- `Source.ListRule` (`models.go:18`) is a **write-only `not null` string always set to `"rss"`** (D-12, ARCH-08). The field's name and shape come directly from routing-tool's scraper-config vocabulary — `CreateSourceInput.listRule/titleRule/linkRule/descRule/dateRule/authorRule` (`routing-tool/src/services/source.ts:14-22`) — where per-field CSS selectors genuinely need a rule string. The reader has exactly one parser and never reads the column.
- `Source.Interval` (`models.go:19`) is the same story: `CreateSourceInput.interval` (`source.ts:22`) is meaningful for a scraper that polls a site; in the reader it is dead (E-01).
- `apps/routing-tool/src/services/opml.ts` carries **its own OPML importer/exporter** (`:56`, `:91`) that reads `listRule` (`:44`, `:101`, `:117`) — so the two OPML implementations also disagree on whether `listRule` round-trips.

So the reader did not import routing-tool's *code*, but it imported its **schema**. Two dead columns in `Source` are archaeological evidence of a scraper tool's data model, and they are `not null` / `default:`-bearing, so removing them is now a migration.

Separately, the reader carries its own hardcoded curated-content risk: `apps/web/src/data/recommended-sources.ts` (19 lines, 9 hardcoded Chinese feeds) fed to `OnboardingWelcome.tsx:90`. It is in-repo (not routing-tool coupling), but it will rot silently — `sspai.com/feed` and `coolshell.cn/feed` are exactly the kind of URLs that change without notice, and there is no health check on the onboarding path. The project's own review already flagged it as `Prove first` → `Delete` (`docs/planning/hai-razor-frontend-review.md:120`, `:416`).
