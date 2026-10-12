# Flore — Product Professor Full Audit

**Date**: 2026-09-01
**Repo**: `C:\opt\workstations\project\web\Flore` · 307 tracked files · version `0.3.0-20260901` · **two releases published** (`v0.1.0-20260806`, `v0.3.0-20260901`)
**Mode**: `full`
**Letter grade: C−**

> **Scope note**: Medium repo (~307 files). Full audit run across all categories. Coverage: near-complete on `apps/web/src` (69 source files read), high on `server/go` critical paths and `apps/desktop` lifecycle. **Not audited**: `apps/routing-tool/` (read-only by project rule), live runtime of the Wails shell on macOS, third-party CVE scan. Findings are labelled `needs confirmation` where evidence was indirect. **Three of my own initial findings were self-corrected during the evidence gate** — recorded transparently in §19.

---

## 1. Executive verdict

**Flore is a well-engineered product with an unfinished user experience and no working safety net.**

> **Correction issued during this audit.** My first pass reported "CI is red on `main`". That was **wrong**, and the correction matters. I verified against a clean `git worktree` of HEAD (`a268fbf`): **`go vet` and `go test` both exit 0 on HEAD.** The failing test file is **untracked**. So CI is green for what is pushed — and red for the working tree everyone develops in. The exposure is latent, not active: the moment those four untracked test files are committed, `go vet` fails, and because it is CI's second step, the frontend typecheck and build stop running on every push. Corrected throughout as P0-001.

The core RSS loop works and works correctly. Data integrity is handled more rigorously than in most shipped desktop apps — the restore path does a WAL checkpoint, `VACUUM INTO`, SQLite validation, atomic-ish file swap, and rollback on failure. The security boundary is real, not decorative: SSRF defence closes the DNS-rebinding TOCTOU window and is applied on both the direct and proxy transports; CSRF combines origin allow-listing with a required custom header; auth uses constant-time comparison.

None of that is what a user experiences.

What a user experiences is: **an app that keeps promising things it does not deliver.** On cold start it shows the first-run onboarding screen to a returning user with 200 subscriptions, then fires seven failure toasts that overwrite each other. A settings toggle labelled "显示托盘图标" does nothing when toggled. The empty state instructs the user to press J and K, which are not bound. The sidebar says a feed has 23 unread; the list behind it has 18. S3 is offered as a sync backend; the server answers `not supported yet`. Search that finds nothing says "没有更多文章了".

Every one of these is a **declaration that outlives its implementation** — and that single pattern accounts for 11 of the 14 top complaints across all five user personas.

**And its safety net is one typo away from not existing.** The four test files that carry all coverage for `filter`, `folder_service`, and `source_service` are **untracked**, and one of them does not compile:

```
$ go vet ./...          # working tree
internal\services\item_service_test.go:116:2: declared and not used: normal

$ go vet ./...          # clean HEAD (git worktree add --detach HEAD)
exit 0
```

Commit them as they stand and `go vet` fails — CI's second step — so the frontend typecheck and build stop running on every push. Meanwhile `urlpolicy.go` — the 258-line SSRF boundary the audit rates as this project's single most valuable asset — has **no tests whatsoever**.

---

## 2. What this product appears to be

A **local-first RSS reader** shipped as a Windows/macOS desktop app via Wails v2.13, with a browser build of the same React frontend. The Go backend runs as a managed child process on a dynamic loopback port, writing its port to a file. All user data lives in a local SQLite database (`reader.db`) — subscriptions, folders, articles, read/star/read-later state, filter rules, settings, and a readability cache.

Stated positioning (`docs/architecture/PROJECT.md`): long-term RSS users, privacy-conscious users who want data autonomy, users who want a fast lightweight desktop reader. Explicit non-goals: no hosted service, no account.

The repository contains a **second, independent application** — `apps/routing-tool/` (Hono + Prisma), a feed-scraping tool that shares subscriptions with the reader via OPML files. Project rules forbid modifying it. I verified the coupling direction is clean: routing-tool talks to the Go backend over HTTP as a client (`routing-tool/src/services/source.ts:6`) with no module imports either way.

**Observation**: the reader inherited routing-tool's *schema*. `Source.ListRule` and `Source.Interval` (`models.go:18-19`) are write-only/dead in the reader but meaningful in routing-tool's `CreateSourceInput`. Two dead `not null` columns are the archaeological trace of a scraper's data model in a reader's database.

---

## 3. Stack detected

| Layer | Technology | Evidence |
|---|---|---|
| Frontend | React 19 + Vite 6 + TypeScript 5 (strict) + Tailwind CSS 3 | `apps/web/package.json` |
| Frontend routing | **None** — view state via ~10 `useState` in `App.tsx` | no router in deps; `App.tsx:113-144` |
| Frontend state | **No library** (project rule) — but `useUpdate.ts` is a hand-rolled module-level external store | `useUpdate.ts:21-61` |
| UI components | **No library** — hand-built + `lucide-react` | deps confirmed |
| Runtime deps | `dompurify`, `html2canvas`, `lucide-react`, `react`, `react-dom` — **5 total** | `apps/web/package.json` |
| Backend | Go 1.26 + Gin v1.10 + GORM v1.25 | `server/go/go.mod` |
| Database | SQLite via `github.com/glebarez/sqlite` (pure Go, no CGO), WAL mode, `SetMaxOpenConns(1)` | `database/database.go:68,160-161` |
| Search | FTS5 virtual table, `porter unicode61`, with LIKE fallback for CJK | `database.go:179-186`, `item_service.go:339-343` |
| Desktop shell | Wails v2.13 (Go), embedded frontend, system tray, native notifications | `apps/desktop/wails.json` |
| Self-updater | Custom: manifest + signature verification + zip-slip guard | `apps/desktop/internal/updater/` |
| CI/CD | GitHub Actions ×3; `gofmt` + `vet` + `test` + `tsc` + build gates; R2-hosted release + `update.json` | `.github/workflows/` |
| Distribution | Windows NSIS + portable zip; macOS dmg + portable zip (**unsigned, unnotarised**) | `release.yml` header |

**Stack assessment**: coherent and honestly minimal on the frontend. The "极简依赖" claim in `PROJECT.md` is upheld — five runtime dependencies is genuinely lean. The absence of a router is a legitimate choice for a three-pane desktop app; the absence of a state library is not, and is already violated in spirit.

---

## 4. Scores summary

| Metric | Score |
|---|---:|
| Overall Product Professor Score | **62 / 100** |
| Launch Readiness Score | **38 / 100** |
| True Score | **61 / 100** |
| AI-Generated Feel Badness | **4.5 / 10** |
| Expected User Complaint Severity | **7 / 10** |

| Category | Weight | Score |
|---|---:|---:|
| Product clarity | 10% | 72 |
| Core functionality | 15% | 74 |
| UX and user flow | 15% | **45** |
| Visual polish | 10% | 78 |
| Architecture | 12% | 52 |
| Security and privacy | 12% | **76** |
| Performance | 8% | 70 |
| Reliability and tests | 8% | **40** |
| Feature completeness | 6% | **62** |
| Business / launch readiness | 4% | **30** |

Full weighting and evidence: [`SCORECARD.md`](SCORECARD.md).

---

## 5. Launch readiness verdict

### **NO — do not ship as-is.**

**Correction to an assumption I made at the start of this audit**: I opened by assuming Flore was pre-release. **That was wrong.** `git tag` shows two releases (`v0.1.0-20260806`, `v0.3.0-20260901`), HEAD equals the version tag, the release workflow publishes to both GitHub Releases and an R2 CDN, and `README.md:7` says *"Flore 是一个使用 Vibe Coding 方式制作的 RSS 阅读器，提供桌面端安装包和便携包。不保证使用和维护，随缘更新。"* — i.e. **it is already released and distributed**. The distribution engineering is real, and better than the product surface.

That makes the verdict sharper, not softer. Users already have this installed.

Five hard blockers:

| # | Blocker | Evidence |
|---|---|---|
| **1** | **The working tree's test build is broken; committing the new tests turns CI red** | `item_service_test.go:116` (untracked). 4 untracked test files. `ci.yml` gates on `go vet` at step 2. |
| **2** | **First-run is wrong for every non-first-time user** | `App.tsx:909`; backend up to 15s (`backend.go:74-79`); `loadingSources` never rendered. |
| **3** | **Error and success are visually identical** | 122 `showToast` calls, zero pass a type; branches at `ToastContainer.tsx:6-10` unreachable. |
| **4** | **Two user-visible settings do nothing** | `trayEnabled` (shell never reads it) and `faviconMode` (frozen by `useState` snapshot). Shipped to users who can see both toggles. |
| **5** | **Auto-backup ships off by default in a data-sovereignty product** | `settings.ts:217` `backupAutoEnabled: false`. A week-one user with no backups has no protection at all, and the app never prompts. |

**Also blocking**: the S3 sync option collects Access/Secret keys for a backend that returns `not supported yet`; macOS builds are unsigned and unnotarised, so Gatekeeper blocks them for every user on every install.

**What is genuinely ready**: the core read/fetch/organise loop, backup and restore, the security boundary, the Ed25519-signed update pipeline, and the visual system.

---

## 6. Top 10 fixes (ordered by impact)

| # | Fix | Effort | Why it ranks here |
|---|---|---|---|
| 1 | Remove the unused `normal` binding at `item_service_test.go:116` | **1 line** | Turns CI green and re-enables ≈1,900 lines of tests. Nothing else can be safely changed until this is done. |
| 2 | Add `urlpolicy.go` tests (private-IP predicates + the rebinding dialer) | ~4h | The highest-value correctness asset has zero coverage. |
| 3 | Introduce a global `connecting / ready / offline` backend state | ~1 day | Fixes the single worst first impression; suppresses the toast flood. |
| 4 | Pass `'error'`/`'success'` at all 122 `showToast` sites | ~3h | The rendering layer already exists and is correct. App-wide payoff. |
| 5 | Bind `j`/`k` (or fix the copy) | **10 min** | The app currently lies to new users. |
| 6 | Add a pause/enable toggle + per-source interval to `EditSourceModal` | ~4h | Backend is complete and tested; only the UI is missing. |
| 7 | Unify the three unread-count queries behind one builder | ~4h | The sidebar badge already disagrees with the list. User-visible wrongness. |
| 8 | Fix mobile: hover-gated controls, 920px settings modal, search drawer, orientation reset | ~2 days | Mobile is currently a squeezed desktop layout. |
| 9 | Remove the S3 option + 6 S3-only credential fields (or implement it) | ~1h to remove | Collecting keys for an unimplemented backend is worse than having nothing. |
| 10 | Route mark-all-read through `ConfirmDialog` | ~1h | Restores risk symmetry with delete operations. |

---

## 7. Findings by priority

### P0 — blocks launch (6)

| ID | Finding | Evidence |
|---|---|---|
| **P0-001** | **Latent CI breaker: 4 untracked test files, one of which does not compile.** HEAD is green (verified via clean worktree); committing them as-is fails `go vet` — CI's 2nd of 7 steps — silencing the frontend typecheck and build on every push | `item_service_test.go:116:2: declared and not used: normal`; `git status` shows `??` for `item_service_test.go`, `filter_test.go`, `folder_service_test.go`, `source_service_test.go`, `updateinfo_test.go` |
| **P0-005** | **Auto-backup ships OFF by default** in a product whose premise is data sovereignty — a released build where week-one users have zero protection | `settings.ts:217` `backupAutoEnabled: false`; `scheduler.go:186-188` skips when false. The backup machinery itself is sound. |
| **P0-006** | **No corruption detection or recovery path** — a corrupt DB bricks the app silently, with no `integrity_check`, no recovery UI, and backups off | `grep integrity_check` → 0 hits; `cmd/main.go:10-13` exits 1; the restore endpoints exist only behind a working app. |
| **P0-002** | **Cold start presents first-run onboarding to returning users + 7-toast flood; no global connection state** | `App.tsx:909`; `backend.go:74-79` (15s); `loadingSources` destructured at `App.tsx:121`, never rendered; 7 catch-blocks each `showToast` against `MAX_TOASTS = 5` |
| **P0-003** | **Success and error toasts are visually identical** | `toast.ts:69` default `'info'`; 122 call sites, zero pass a type; unreachable branches at `ToastContainer.tsx:6-10` |

### P1 — ship-blocking quality (20)

| ID | Finding | Evidence |
|---|---|---|
| **P1-001** | `Source.active` (pause feed) has no UI; only recourse is deleting the feed and its history | `models.go:20`; consumed at `coordinator.go:165,225`; absent from `EditSourceModal.tsx` |
| **P1-002** | `Source.Interval` written by the client, never read by the scheduler — two controls for one concept | `models.go:19`; `AddSourceModal.tsx:134`; scheduler uses global `defaultInterval` |
| **P1-003** | Sidebar unread badge disagrees with the article list; ignores `hideInTimeline`/`isPrivate` | `source_service.go:81-99` (no join, no filters) vs `item_service.go:203-214` and `:217-227` |
| **P1-004** | `trayEnabled` toggle does nothing; shell calls `startTray()` unconditionally | `SettingsNotifyTab.tsx:50-51`; zero `trayEnabled` hits in `apps/desktop/*.go`; `app.go:255` |
| **P1-005** | S3 sync backend is selectable, collects Access/Secret keys, returns `not supported yet`; WebDAV only probes and never transfers | `SettingsSyncTab.tsx:23-26,478-496`; `sync_engine.go:250-295`; `FileProvider` has zero implementations |
| **P1-006** | Mobile: hover-only list actions unreachable by touch | `ArticleList.tsx:733` `opacity-0 group-hover:opacity-100`, no `focus-within` |
| **P1-007** | Settings modal fixed at 920px — unusable on a phone | `SettingsModal.tsx:574`; no responsive variant |
| **P1-008** | Search that finds nothing says "没有更多文章了"; search failure renders the same; search caps at 50 with no pagination | `ArticleList.tsx:428-432`; `useItemsData.ts:197,204-207` |
| **P1-009** | Empty state advertises J/K navigation; no `j`/`k` handler exists | `Reader.tsx:549` vs `App.tsx:364-412` |
| **P1-010** | "Mark all read" — an irreversible bulk action — has no confirmation, unlike delete operations | `App.tsx:669-677`; contrast `:607-623`, `:649-666`, `:730-745` |
| **P1-011** | `faviconMode` changes do not take effect until reload (read once into `useState`) | `SourceAvatar.tsx:27-29` |
| **P1-012** | **Auto-backup off by default** — no protection for a first-week user; app never prompts | `settings.ts:217`; `scheduler.go:186` |
| **P1-013** | **`appTheme` stored on the server is never re-applied at startup** — only `localStorage.theme` is read by the pre-paint script, so a fresh machine ignores the synced dark preference | `index.html:28` reads `localStorage.getItem('theme')`; `settings.ts:346-356` loads server settings but nothing re-applies the attribute. Also: no `matchMedia` change listener, so OS theme flips are ignored until restart. |
| **P1-014** | **No crash reporting and no in-app log access** — a shipped crash is invisible to the developer and unactionable for the user | grep for Sentry/telemetry/unhandledrejection: 0 matches. `ErrorBoundary.tsx:27-28` only `console.error`s. Logs exist on disk but nothing links to them; the diagnostic endpoint has no caller. |
| **P1-015** | **Uninstall silently orphans user data** — the NSIS uninstaller removes the install dir, but `reader.db` + backups live elsewhere and are left behind | `installer.nsi:81-93` vs `paths.go:11,19` (`%LOCALAPPDATA%\Flore`) and `setting_service.go:105-111` (`<root>/backups`) |
| **P1-016** | **`sw.js` is shipped but never registered** — 2KB dead weight; `manifest.json` promises installability the service worker cannot back | grep `serviceWorker` in `apps/web/src`: 0 hits; `public/sw.js` exists; `index.html:20` links the manifest |
| **P1-017** | **The auth boundary is route placement, not a capability rule** — `POST /items/export` returns the whole reading history from outside `sensitive`, reachable even with the token set; `/articles/cleanup` and `/database/vacuum` likewise. Remote-exploitable under the documented `BIND_ADDR=0.0.0.0` | `reader.go:226,294,269` vs the deliberate protection at `:271-272`; CSRF passes header-less requests by design (`security.go:48-53`) |
| **P1-018** | **CORS trusts every loopback origin while reads are token-free** — any local web page can silently bulk-exfiltrate the history and FTS index, even with the token set | `security.go:16-29` (any port); reads at `reader.go:186,210,222,265` outside `sensitive`; `backend.go:253-262` |
| **P1-019** | **No corruption detection anywhere** — `integrity_check`/`quick_check` 0 hits; a corrupt DB makes the backend exit with no user-reachable recovery path | `database.go:52-87`, `backend.go:92-108`, `cmd/main.go:10-13` |
| **P1-020** | **Restore is not crash-atomic** — `copyFile` calls `os.Create(dst)`, truncating the live DB before the copy | `backup.go:175-197` (`:182`), `backup_restore.go:60-75` |

### P2 — quality, should fix (9)

| ID | Finding | Evidence |
|---|---|---|
| P2-001 | Mobile search leaves the drawer open over the results | `App.tsx:208-214` omits `setShowMobileSidebar(false)`, unlike `:859,867,873` |
| P2-002 | Orientation change force-returns to the list, dropping the article being read | `App.tsx:160-163` |
| P2-003 | Long operations (export ≤120s, sync ≤300s) have no progress; export closes its modal immediately | `ExportArticlesModal.tsx:138`; `api.ts:363,479` |
| P2-004 | Add-source and OPML import have divergent feedback (one toasts + fetches, the other does neither) | `App.tsx:555-573` vs `:769-774` |
| P2-005 | Add-source silent validation dead-end; the Add button does nothing with no message | `AddSourceModal.tsx:146-147` |
| P2-006 | "全部标为已读" fires per-article POSTs in scroll mode with no batching | `ArticleList.tsx:215-239`; `App.tsx:334-351` |
| P2-007 | `<button>` nested in `<button>` — invalid HTML, undefined screen-reader behaviour | `Sidebar.tsx:732-749` |
| P2-008 | ~30 toggles have `role="switch"` but no accessible name; zero `htmlFor` in the entire codebase | `SettingsShared.tsx:95-115` |
| P2-009 | OPML import has no dedup; importing twice duplicates everything (remote sync *does* dedup) | `folder_service.go:141` vs `sync_engine.go:536-539` |

### P3 — hygiene (7)

| ID | Finding |
|---|---|
| P3-001 | `icons.tsx:2` header says "Typestream 图标系统"; `:87` references "原项目" — a predecessor not in this repo |
| P3-002 | Ticket-coded comments span ~6 ID schemes (`T-v03-02`, `m-03`, `M-R2`, `n-09`, `P0-1`, `B-08`, `M-A3`) across 4 planning docs, no index; `B-08` and `M-A3` have **zero referents anywhere** |
| P3-003 | `ThemeToggle.tsx` is an entire dead file; **28** dead icon aliases (scripted grep of all 38); `usePlatform` returns two unused values |
| P3-004 | **16** routes with no reachable caller; **2** literal duplicates binding the same handler; **5** boolean toggle routes where the client always sends the affirmative path with a flag |
| P3-005 | 3 dead model fields; migration v3 is a no-op; migration v5 rewrites v2; **45** settings keys presented flat with **16 tautological `desc` strings** in the General tab alone |
| P3-006 | `diagnostic.go:173` `readDesensitizedLog` performs no desensitisation |
| P3-007 | **Five product identities in one tree**: `package.json` "flore" / `apps/web/package.json` "rss-fetcher-client" / Go module `github.com/rss/go-server` / repo `github.com/icecome/Flore` / `PROJECT.md` data path `%LOCALAPPDATA%\RSSReader\` vs actual `paths.go` `%LOCALAPPDATA%\Flore` |

---

## 8. Bugs and broken flows

**Confirmed broken (verified this session):**

1. **First-run screen shown to returning users** — `App.tsx:909`. The single worst user-facing defect.
2. **Toast type parameter is unreachable** — 122 calls, zero pass a type.
3. **`trayEnabled` has no effect** — verified: zero hits in `apps/desktop/*.go`.
4. **J/K advertised, not bound** — verified: no `case 'j'` / `case 'k'`.
5. **`faviconMode` frozen at mount** — `SourceAvatar.tsx:27`.
6. **S3 selectable, unimplemented** — `sync_engine.go:256`.
7. **Sidebar unread count disagrees with the list** — `source_service.go:81` omits filters the other two paths apply.
8. **Search empty state uses the wrong copy for four different situations.**
9. **Diagnostic log tail is not desensitised** despite the function name.
10. **Mobile search leaves the drawer covering results.**
11. **Server-synced `appTheme` never applied on a fresh machine** — `index.html:28` reads only `localStorage`.
12. **OS theme changes ignored until restart** — no `matchMedia` change listener anywhere.
13. **PDF export injects the font *enum name*** (`serif`) as a CSS `font-family` instead of the resolved stack (`Reader.tsx:421` vs `:347`).
14. **PNG export forces a white background** regardless of reader theme (`Reader.tsx:375`) — dark/sepia/green users get white images.
15. **Uninstall leaves all user data silently** (`installer.nsi:81-93` vs `paths.go:19`).
16. **`sw.js` shipped, never registered** — PWA claim without a service worker.
17. **`getDomainFolderName` failure creates a real folder named "未分类"** (`App.tsx:522-529`), colliding conceptually with `folderId = null`.

**Dead ends / half-flows:**

11. Onboarding card 3 is a `<div>` styled as a button (`OnboardingWelcome.tsx:75-81`).
12. Add-source: no toast, no fetch trigger, and silent rejection when `interval < 5`.
13. OPML import with `imported === 0` closes silently (`App.tsx:769`).
14. iframe mode has no load timeout — the spinner can spin forever (`Reader.tsx:629-638`).
15. Export's "current view" count shows the loaded page (≤50), not the true scope total (`ExportArticlesModal.tsx:104`).
16. "测试连接" in sync settings writes the half-filled config to the database (`SettingsSyncTab.tsx:181-183`).
17. ReaderToolbar button customisation does not persist (`toolbarVisible` is `useState`).
18. `getDomainFolderName` returns '未分类' on parse failure, **creating a real folder by that name** that collides conceptually with `folderId = null` (`App.tsx:522-529`).

**`needs confirmation`:** iframe `onLoad` behaviour under cross-origin error conditions in the Wails WebView2 surface — reasoned from code, not executed.

---

## 9. Architecture problems

### 9.1 `ReaderService` is a god object, not an aggregate root

`services/reader.go:15-57` — one struct holding the `db` plus a mutex guarding it, **six independent caches** (unread counts, HTTP client, settings, filter rules, with **four separate invalidation sequence numbers**), the fetch coordinator, the sync engine, two channel-based index buffers, and two background goroutines. `NewReaderService()` (`:69-75`) is a `sync.Once` **process singleton**.

The three "service layers" (`source_service.go`, `item_service.go`, `folder_service.go`) are **file partitions of one type**, not layers. Proof: they declare methods on the same `*ReaderService` receiver, and they call each other's unexported methods freely (`item_service.go:23` → `folder_service.go:78`; `sync_engine.go:609` → `folder_service.go:23`).

`setDb` (`:109`) replaces the live connection and invalidates nothing. `execLocked` (`:119`) exists solely to patch the race that hot-swappable connection creates.

**Why it matters**: `invalidateUnreadCount` is called from **9 sites**. The path that forgets produces a silently stale sidebar with no error — which is exactly the class of bug already present (P1-003).

**Recommendation**: not a rewrite. Collapse the four sequence counters into one monotonic `dbVersion`; move the caches to the layer that owns the DB lifecycle; keep the injectable `NewReaderServiceWithDB` constructor (tests depend on it). **Do not introduce a DI framework** — construction injection is already test-friendly.

### 9.2 `App.tsx` is 1,082 lines with two contradictory conventions

23 `useState`, 10 `useRef`, ~30 `useCallback`. Four refs exist to defeat stale closures under **two mutually exclusive house rules documented in the same file**:

- `:328` — "避免渲染期写 ref（React 并发渲染下不纯）" → effect-synced (`itemsRef:330-332`, `foldersRef:534-536`)
- `:461-462` — the opposite convention → render-time bare assignment (`keyboardHandlerRef:447`, `fetchersRef:472-480`)

`fetchersRef` additionally duplicates its seven fields (initialised `:463-471`, reassigned `:472-480`). The `refreshing` falling-edge is detected twice (`App.tsx:778-801` and `useFetchOrchestration.ts:88-91`), so the import-report toast can silently fail to fire.

### 9.3 Abstraction without implementation

`internal/sync/provider.go` declares `RemoteProvider` — **real**, with 2 implementations and 5 provider-agnostic call sites in the engine. Beside it sits `FileProvider` (`:173-185`) and `ReaderPackage` (`:131-170`) with **zero implementations**, while WebDAV is hand-rolled *outside* the interface at `sync_engine.go:266-295`. `ProviderTTRSS` (`:16`) is unreferenced; `ErrUnsupported` (`:20`) has one producer and one test consumer, with no engine-side capability check.

`RemoteProvider` also leaks: the engine branches on `provider.Name() != ProviderGReader` for a GReader-specific cadence (`sync_engine.go:218,226`), and persists that display name as a foreign key in `Source.remoteOrigin`.

### 9.4 Duplication (14 instances)

Most consequential:

| Pattern | Count | Consequence |
|---|---:|---|
| Unread-count computation | 3–5 | **Already divergent and user-visible** (P1-003) |
| Scope/timeline filtering | 3 | `item_service.go:15`, `:229`, `export.go:48` — no test asserts they agree |
| Backup-name validation | 6 | One copy already diverged (`backup.go:479` drops the `.zip` check into a second `if`), producing different client error text via `safeError` |
| `resolveSrc` + `buildProxyUrl` | 2 pairs, one file | Byte-identical, 85 lines apart (`Reader.tsx:90-108`, `:175-201`) |
| OPML export | 2 | **Behaviourally different**: `SettingsModal.tsx:319` tries the native save dialog first; `App.tsx:804` does not |
| Shortcut table | 2 | Same 9 keys, two groupings |
| `escapeRegExp` | 2 | Identical body, two names |

### 9.5 Data model and migrations

Dead: `Source.ListRule` (write-only, always `"rss"` — and onboarding writes `listRule: 'show'`, a value nothing ever reads), `Source.Interval` (never read), `SourceHealth.NextCheckAtUnix` (never written, **carries an index**). Orphan: `Item.Source` GORM association, never preloaded — every query hand-joins. `FilterRule.Priority` (`models.go:139`) is honoured by the engine but has **no UI** — `priority` appears 0 times in `SettingsRulesTab.tsx` and 0 times in `api.ts`, so rule order is not user-settable.

Migrations are damning in aggregate: v1 is never declared; **v3 is a no-op** (body is a log line and `return nil`); **v5 exists to redo what v2 failed at** — v2 parsed dates with `RFC3339Nano`, silently skipped GORM's space-separated format, logged a skip count that nothing asserted on. **No migration has a post-condition assertion.** A newer-schema database is a hard error (`migrations.go:75-80`), which is defensible but user-hostile for a single-user local app. **Measured coverage for the `database` package is 0%.**

### 9.6 Layer violations

`ReaderHandler` (`handlers/reader.go:110-119`) owns a rate limiter with its own goroutine (constructed in `RegisterRoutes:175`) and a bounded frame cache, and its `stop()` is never called from `backend.go`. This contradicts `AGENTS.md` §5.1 ("handler 只做参数解析 → service → JSON").

---

## 10. UX and design problems

Detailed in [`USER_COMPLAINTS.md`](USER_COMPLAINTS.md). Summary of the structural issues:

1. **No state narrative.** There is no global notion of connecting / ready / offline / erroring. Every failure is an independent toast, and they compete for five slots.
2. **Feedback is monochrome.** Success and error are indistinguishable. This one defect degrades the value of every error message in the app.
3. **No progress for long operations.** Export (≤120s) closes its modal instantly. Sync (≤300s) shows a static button label. Backup and OPML import show button text only.
4. **Empty states are reused across incompatible meanings.** One string covers four situations, and is wrong for the most common one.
5. **Mobile is a squeezed desktop layout.** 360px fixed list, 920px fixed settings modal, hover-gated list actions, orientation change discards reading position.
6. **Accessibility is partial.** Good: focus traps with restoration, `aria-labelledby` on dialogs, `aria-live` on toasts, `role="menu"`. Missing: zero `htmlFor` in the codebase; ~30 unnamed toggles; invalid nested buttons; no keyboard navigation in context menus; no `aria-live` for the refresh spinner or progress bar.
7. **Discoverability.** "Pause feed" does not exist; "clear folder" / "delete folder" are context-menu only; bulk source management lives only inside settings; the diagnostic package has no entry point at all.

**What is good, and should not be cut**: OPML import's error chain (modal error + backend message passthrough + `internal_server_error` translation, `App.tsx:760-767`) is the best error design in the app. The `useFocusTrap` implementation is production-grade. The fetch orchestration (async submit + backoff polling + consecutive-failure notice) is well-built.

---

## 11. Database / security / privacy

Full detail: [`DATABASE_SECURITY.md`](DATABASE_SECURITY.md). Summary:

**Genuinely strong** — SSRF defence is 4 layers including a dial-context that resolves DNS once and dials the validated IP, closing the rebinding TOCTOU window (`urlpolicy.go:208-243`), applied on the direct **and** proxy transports (`fetcher.go:249`, with an explicit comment noting the LAN-pivot risk of omitting it). CSRF pairs an origin allow-list with a mandatory custom header (`security.go:40-66`, `fetchData.ts:31-36`). Auth uses constant-time comparison. SVG rejected on the image proxy; 10MB response caps; 5-redirect cap; FTS5 query sanitisation; upload size caps; zip-bomb guard; path-traversal confinement; restore validates the SQLite magic header, `PRAGMA schema_version`, and the `Item` table.

**Deductions, in order of weight:**

| # | Risk | Evidence |
|---|---|---|
| 1 | **No `PRAGMA integrity_check` anywhere**; a corrupt DB makes the backend exit (`cmd/main.go:10-13`) with no user-reachable recovery | 0 hits in `server/go`; `database.go:52-87`, `backend.go:92-108` |
| 2 | **Data-shaped endpoints outside `sensitive`** — `/items/export` (full history), `/articles/cleanup`, `/database/vacuum` reachable even with the token set; remote-exploitable under the documented `BIND_ADDR=0.0.0.0` | `reader.go:226,294,269` vs the deliberate protection and stated rule at `:271-272`; CSRF passes header-less requests by design (`security.go:48-53`) |
| 3 | **CORS trusts every loopback origin, any port, while reads are token-free** — any local web page can silently bulk-exfiltrate history and the FTS index | `security.go:16-29`; `backend.go:253-262`; reads at `reader.go:186,210,222,265` |
| 4 | **Restore is not crash-atomic** — `os.Create` truncates the live DB before the copy; restore-source validation omits an integrity check | `backup.go:175-197` (`:182`); `backup_restore.go:60-75` |
| 5 | **Diagnostic package leaks username, paths, full feed URLs, panic dumps**; `readDesensitizedLog` performs no redaction | `diagnostic.go:55-65,173-175`; `logging.go:124-137` (write-time, 8 keys); `proxy_handlers.go:197` comment misstates what Yandex receives |
| 6 | **Sync credentials plaintext in the DB and in every unencrypted backup zip**; `maskSecret` is UI-only | `sync_config.go:38-44`; `backup.go:297-308,364-372`; `db_handlers.go:275-282` |
| 7 | **SSRF boundary has zero tests** (see `BUILD_HEALTH.md`) | no `urlpolicy_test.go` |
| 8 | **16 unreachable routes, several unauthenticated writes** | independently verified; `/database/vacuum` and `/cache/clear-readability` are non-sensitive writes |
| 9 | **`readDesensitizedLog` / port file `0644`** — the latter means "unguessable dynamic port" is not a control in empty-token modes | `backend.go:177-181`; `apps/desktop/backend.go:52` |

**The two systemic weaknesses are structural, not local.** (1) The auth boundary is decided route by route, so data-shaped operations leak around it. (2) Durability is treated as a backup feature rather than a runtime invariant — no integrity monitoring, no crash-atomic replace, no recovery UX, backups off by default. Both are fixable with one rule and one `PRAGMA quick_check`.

**Data durability — the restore *sequence* is better than average.** `restoreFromFile` (`backup_restore.go:16-113`) does WAL-checkpoint → validate → `VACUUM INTO` snapshot → close and release locks → copy → reopen → migrate → vacuum, with rollback on failure **and** connection re-initialisation on the rollback path too (`:128-140`), plus explicit cache invalidation. The inline note explaining why `GetSettingInt` cannot be called while holding `dbMu` (RWMutex is not reentrant) is the kind of reasoning that indicates the author understood the failure mode.

**But the invariants around it are absent** — no integrity monitoring, crash-atomicity, or recovery UX (P1-019, P1-020) — and `backupAutoEnabled` defaults to `false` (P0-005). A week-one user with no backups and a disk failure loses everything, and the app never suggests enabling it.

---

## 12. Performance

| Observation | Detail |
|---|---|
| Bundle | 458 KB main (137 KB gzip) + 202 KB html2canvas chunk (48 KB gzip), correctly code-split via dynamic `import()` at `Reader.tsx:372`. Heavy for a 5-dependency app; `lucide-react` re-exports 65 symbols + 39 aliases, a known tree-shaking hazard. |
| List rendering | Uses CSS `content-visibility: auto` (`ArticleList.tsx:667`) rather than a JS virtualiser — the right call. |
| Pagination | Page size 50 with an `Item.id` tie-breaker for stable offset paging (`item_service.go:72-73`) — correct. |
| Redundant fetching | `useSourcesData.ts:85-87` re-fetches the full source list on **every** source/folder selection change. |
| Cache invalidation | Whole-cache invalidation from 9 call sites; cheap at current scale (documented as deliberate) but fragile. |
| Per-item requests | Scroll-mode auto-mark-read issues one POST per article with no batching (`ArticleList.tsx:215-239`). |
| Indexing | Batch FTS5 writes through a channel with an overflow buffer — good decoupling of fetch throughput from index writes. |

No runtime profiling was performed; these are static observations. **Confidence: medium.**

---

## 13. Testing and reliability gaps

- **4 untracked test files, one of which does not compile** — ≈1,900 lines that would fail CI on commit (P0-001).
- **Measured coverage on HEAD**: `sync` 67.4%, `logging` 34.2%, `services` 22.2%, `handlers` **0%**, `database` **0%**, `models` **0%**, `backend` **0%**, `pkg/paths` **0%**.
- **Zero frontend tests.** No Vitest/Jest, no `.test.*`/`.spec.*` file.
- **No ESLint config** despite the project having a code-standards doc.
- **`urlpolicy.go` — zero tests**, alongside `proxy_content.go`, `export.go`, `sync_config.go` (secret masking), `scheduler.go`.
- **No crash reporting** and no `unhandledrejection` handler — a shipped crash is invisible to the developer.
- **No dependency auditing** (`npm audit`, `govulncheck`) despite a network-facing proxy and user-supplied archive unpacking.
- **No coverage threshold**; `build-desktop.yml` does build on all three OSes (good), but there is no macOS-specific test job.

**Type-safety note**: `tsconfig.json` has `strict: true` but `noUnusedLocals: false` and `noUnusedParameters: false`. Enabling those two flags would have surfaced this audit's entire dead-code class — the dead file, the 28 dead aliases, the unused destructures — automatically.

**CI divergence noted**: `ci.yml` runs `tsc --noEmit` while the project script is `tsc -b`. Both pass; cosmetic only.

---

## 14. Missing features worth adding

Evidence-ranked. **Not** a wish list — each traces to a real gap.

| # | Feature | Why | Effort |
|---|---|---|---|
| 1 | **Pause / resume a feed**, with per-source refresh interval | Backend fully implemented and tested; only UI missing. Currently the only recourse is delete-and-lose-history. | ~4h |
| 2 | **Feed autodiscovery** — accept a bare site URL and find its feed | `CreateSource` requires an exact feed URL (`source_service.go:200-221`). Pasting a homepage is the highest-friction onboarding moment. | ~1 day |
| 3 | **OPML import dedup** | Importing twice duplicates everything. Remote sync already dedups; the user-facing importer does not. | ~2h |
| 4 | **Mark-unread on scroll-back** | `markReadMode: 'scroll'` is one-directional and the count is clamped at 0 (`App.tsx:266`) — recovery is impossible. | ~2h |
| 5 | **Nested folder creation** | Backend supports arbitrary depth with cycle guards; the Sidebar renders recursively. `CreateFolder` has no `parentId` parameter anywhere. Reachable only via nested OPML import. | ~4h |
| 6 | **Diagnostic package UI entry** | 283 LOC and 3 routes exist with no caller. Either surface it under "About" or delete it. | ~2h or delete |
| 7 | **`j`/`k` navigation** | The app already advertises it. | 10 min |

**Explicitly not recommended**: global media hotkeys (sandboxed cross-origin iframes make this unreliable and it is the wrong affordance for a text reader); shortcut customisation (`SettingsShortcutsTab` is a 28-line static reference and that is correct).

---

## 15. Expected user complaints

Summarised here; full simulated reviews with persona voice in [`USER_COMPLAINTS.md`](USER_COMPLAINTS.md).

| Persona | Headline | Severity |
|---|---|---|
| First-time user | "It told me I had no subscriptions when I had 200. I nearly reinstalled." | P0 |
| Mobile user | "The star and read-later buttons are invisible on my phone. Settings don't fit the screen." | P1 |
| Returning user | "Success and failure look exactly the same. And I can't pause a broken feed — only delete it." | P0/P1 |
| Power user | "File sync is a shell that collects my S3 keys. The sidebar says 23 unread, the list shows 18." | P1 |
| Skeptical client | "Your SSRF boundary — your best work — has no tests. Auto-backup is off. Two settings toggles do nothing. Your CI breaks the moment someone commits the tests." | P0 |

**Aggregate complaint severity: 7/10.** The engineering quality raises expectations; the UI gap between promise and delivery converts those expectations into distrust.

---

## 16. GitHub issue index

See [`ISSUE_INDEX.md`](ISSUE_INDEX.md) and [`issues/`](issues/).

| ID | Title | Priority |
|---|---|---|
| [P0-001](issues/P0-001-fix-services-test-build.md) | Fix the untracked test build before it turns CI red | P0 |
| [P0-002](issues/P0-002-no-backend-connection-state.md) | Add a global backend connection state | P0 |
| [P0-003](issues/P0-003-toast-types-unreachable.md) | Pass toast types so errors are distinguishable | P0 |
| [P0-004](issues/P0-004-urlpolicy-untested.md) | Add tests for the SSRF boundary (`urlpolicy.go`) | P0 |
| [P0-005](issues/P0-005-auto-backup-off-by-default.md) | Enable auto-backup by default | P0 |
| [P0-006](issues/P0-006-no-corruption-detection.md) | No corruption detection and no recovery path | P0 |
| [P1-001](issues/P1-001-pause-source-no-ui.md) | Add pause/enable UI for `Source.active` + per-source interval | P1 |
| [P1-002](issues/P1-002-unread-count-divergence.md) | Unify the three unread-count queries | P1 |
| [P1-003](issues/P1-003-tray-enabled-does-nothing.md) | `trayEnabled` does nothing — wire it or delete it | P1 |
| [P1-004](issues/P1-004-s3-option-collects-secrets.md) | S3 sync option is unimplemented but collects credentials | P1 |
| [P1-005](issues/P1-005-mobile-hover-only-actions.md) | Mobile: hover-gated list actions are unreachable | P1 |
| [P1-006](issues/P1-006-settings-modal-fixed-width.md) | Settings modal is unusable on mobile | P1 |
| [P1-007](issues/P1-007-search-empty-state-wrong-copy.md) | Search empty/error states show the wrong message | P1 |
| [P1-008](issues/P1-008-jk-advertised-not-bound.md) | Empty state advertises an unbound J/K shortcut | P1 |
| [P1-009](issues/P1-009-mark-all-read-no-confirm.md) | Confirm before marking everything read | P1 |
| [P1-010](issues/P1-010-favicon-mode-frozen.md) | `faviconMode` changes require a reload | P1 |
| [P1-011](issues/P1-011-apptheme-not-reapplied.md) | Server-synced `appTheme` is never re-applied at startup | P1 |
| [P1-012](issues/P1-012-unsigned-installers.md) | Windows SmartScreen and macOS Gatekeeper block every install | P1 |
| [P1-013](issues/P1-013-data-endpoints-outside-sensitive.md) | Data-shaped endpoints sit outside the `sensitive` group | P1 |
| [P1-014](issues/P1-014-loopback-cors-token-free-reads.md) | CORS trusts every loopback origin while reads are token-free | P1 |

---

## 17. Implementation roadmap

**Phase 0 — restore the safety net (半天)**
1. Fix `item_service_test.go:116`; confirm `go test ./...` and CI go green.
2. Add `urlpolicy.go` tests: `isPrivateIP` boundaries (loopback, RFC1918, link-local, IPv6-mapped, `0.0.0.0/8`), plus a dialer test proving a resolved-then-rebound host cannot reach a private IP.
3. Enable `noUnusedLocals` / `noUnusedParameters`, fix what surfaces.
4. *Gate: do not proceed until CI is green.*

**Phase 1 — stop lying to users (2–3 天)**
5. Global backend connection state; suppress per-request toasts until ready.
6. Toast types at all call sites.
7. Bind `j`/`k` (10 minutes).
8. Search empty/error state branching.
9. `faviconMode` reactivity.
10. `trayEnabled` — wire or delete.
11. Remove the S3 option and its credential fields.
12. Make onboarding card 3 a real button.
13. Confirm mark-all-read.

**Phase 2 — close the declare-without-implement gaps (1 周)**
14. Pause/enable + per-source interval UI.
15. Unify the unread-count queries.
16. Mark-unread on scroll-back.
17. Add-source feedback parity with OPML import; fix the silent validation dead-end.
18. OPML import dedup.

**Phase 3 — mobile and accessibility (1–2 周)**
19. Responsive settings modal; always-visible row actions under 768px; close the drawer on search; fix the orientation reset.
20. `htmlFor` on all form fields; accessible names on ~30 toggles; un-nest the buttons in `Sidebar.tsx`.
21. Keyboard navigation in context menus.
22. `aria-live` for refresh and progress.

**Phase 4 — architecture (2–4 周, only after 0–3)**
23. Collapse the 4 sequence counters into one `dbVersion`; relocate caches.
24. Resolve the two ref conventions in `App.tsx` to one.
25. Delete the **16** unreachable routes, the 6 dead Go symbols, the 3 dead model fields, the no-op migration, the dead file and dead aliases.
26. Delete the ticket-coded comments, or bind them to real issues.
27. Merge the divergent duplication: scope filters, backup-name validation, OPML export, shortcut tables.

**Phase 5 — launch prep**
28. macOS Developer ID signing + notarisation.
29. `npm audit` + `govulncheck` in CI; macOS job in the CI matrix.
30. **Answer the open question** (§19) before shipping the onboarding and migration paths.

---

## 18. Exact prompts to run next

See [`PROMPTS.md`](PROMPTS.md).

---

## 19. Questions still needing user confirmation

| # | Question | Blocks |
|---|---|---|
| 1 | **Was the removal of these features ever attempted, or was the repo's own `hai-razor` audit written and then shelved?** `docs/planning/hai-razor-frontend-review.md` (362 lines) independently recommends cutting the diagnostic subsystem, wiring `trayEnabled`, and deleting `Source.Interval`. None of it happened. | Understanding whether the bottleneck is decision or execution — it determines whether more audit documents help at all. |
| 2 | **Is the macOS build a real target?** `release.yml` builds and publishes a dmg, but with ad-hoc signing only and a self-documented Gatekeeper warning. | Urgency of Developer ID signing + notarisation. If macOS is not a target, drop the artefact and the claim. |
| 3 | **Was file-based sync (WebDAV/S3) ever a committed roadmap item?** The code says `P3 实现` in three places. | If yes, `FileProvider` is a stub awaiting work and the S3 form should be hidden until it works. If no, delete the interface, the 11 config fields, and the S3 option. |
| 4 | **Is the diagnostic package for end users or the developer?** It has no UI, so today it is unreachable by users. | If end users: needs a UI **and** real log redaction (it currently contains the user's full subscription list). If developer-only: should not ship in a release build. |
| 5 | **Do the 4 legacy settings migrations still have users?** Two releases exist (`v0.1.0-20260806`, `v0.3.0-20260901`), so **there may well be real installs.** | Gates whether `migrateMarkReadMode` / `migrateHidePrivate` / `migrateReaderTheme` / `loadOnlineAvatar→faviconMode` can be deleted. **My earlier assumption that these were safely deletable was based on Flore being pre-release — that assumption was wrong.** |
| 6 | **What is "Typestream" (`icons.tsx:2`) and the "原项目" (`:87`)?** | Only artefact in the repo with no possible referent. If a predecessor exists, the alias layer may be a real compatibility requirement rather than residue. |
| 7 | **Are `Source.ListRule` / `Source.Interval` still written by `apps/routing-tool` against the same database?** | Determines whether these columns can be dropped. My reading: routing-tool has its own SQLite DB and talks to the reader only over HTTP (`routing-tool/src/services/source.ts:6`), so they should be droppable — **needs confirmation**. |

**Assumption corrections made during this audit** (recorded so they are not repeated):
- ~~"Flore is pre-release."~~ **Wrong** — two tags exist, releases publish to GitHub + R2, and `README.md:7` describes a shipped product using Vibe Coding.
- ~~"CI is red on main."~~ **Wrong** — HEAD is green; the failure is in the dirty working tree, and is latent until the untracked tests are committed.
- ~~"The ticket-ID comment scheme has no referents."~~ **Wrong** — `T-v02/03-*`, `m-*`, `M-*`, `T-P0-*` resolve to real documents in `docs/planning/`. Only `B-08` and `M-A3` dangle.
- ~~"~43 settings keys."~~ **45**, verified programmatically; interface and defaults agree exactly.
- ~~"~18 zero-caller routes."~~ **16** confirmed after template-literal-aware re-verification (an initial naive count of 29 over-reported because the client builds several paths from literals).
- ~~"~39 dead icon aliases."~~ **28** of 38, by scripted grep.
- ~~"`Source.hideInTimeline` is unwired."~~ **Wrong** — fully wired both ends (`SettingsSourcesTab.tsx:241` → `EditSourceModal.tsx:131` → `item_service.go:40,208`). This is useful counter-evidence: cross-stack wiring *was* achievable here, which makes `trayEnabled` / `active` / `faviconMode` oversights rather than constraints.

---

## Appendix — what this audit did **not** establish

- No runtime profiling; performance scores are static.
- No rendered-screenshot review; visual polish was judged from tokens and markup. **A `/product-professor screenshots` pass would raise or lower that score with real evidence.**
- No macOS execution; the Wails shell's platform-specific paths (traffic-light inset, `wails://` scheme, notification registration) were read, not run.
- No penetration test; the security score reflects code review, not exploitation.
- `apps/routing-tool/` was out of scope by project rule; its own quality is unassessed.
- Third-party CVE status was not enumerated; only dependency *age* and maintenance status were noted.

```
To track your progress:
  product-professor record-score --auto "After initial audit"
  product-professor compare
```
