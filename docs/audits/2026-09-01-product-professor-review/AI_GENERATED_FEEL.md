# AI_GENERATED_FEEL

**Audit date**: 2026-09-01
**Score: 4.5 / 10 badness** (higher = worse)

---

## Verdict up front

**This does not read like a vibecoded prototype, and it does not look like a Tailwind template.**

The visual layer is genuinely authored: `index.css` defines a documented token system with a stated rationale, complete light/dark pairs, a four-step shadow ladder, and a named easing curve. The security layer is better than most hand-written production apps. The DOM structure uses semantic elements, `role="dialog"` with `aria-labelledby`, focus traps with focus restoration, and `aria-live` on toasts.

**What gives it away is not the craft — it is the residue.** Three specific classes of evidence, all of them unobtainable without heavy AI assistance:

1. **Ticket-coded comments** referencing a task system with no trace in the repo.
2. **Features built to a spec nobody asked for**, whose backend is complete and whose UI never arrives.
3. **Self-contradiction at scale** — two docs, two conventions, two implementations of the same rule, all internally consistent and mutually inconsistent.

A human team produces (3) too. A human team almost never produces (1).

---

## A. Feature-by-feature demand evidence

For each: is it reachable, is it consumed, is there any evidence a user asked for it?

| Feature | Reachable in UI? | Consumed? | Demand evidence | Verdict |
|---|---|---|---|---|
| **Core**: fetch, read, star, read-later, folders, OPML, search, filter rules, retention, backup | ✅ | ✅ | This is the product | **Keep** |
| `Source.active` (pause source) | ❌ **no UI** | ✅ backend filters on it (`coordinator.go:165`) | Table stakes for RSS | **Build the UI** — backend done, ~50 LOC |
| `Source.Interval` (per-source refresh) | ❌ **no UI** | ❌ **scheduler never reads it** | Commonly expected | **Build the UI or drop the column** |
| **File sync** (WebDAV/S3, 11 config fields) | ⚠️ form exists, does nothing | ❌ **no read/write path exists** | No evidence | **Delete or implement** — currently collects S3 keys for a feature that cannot work |
| `FileProvider` interface + `ReaderPackage` types | n/a | ❌ **zero implementations** | None | **Delete** — textbook speculative abstraction. Its own comments confess: `provider.go:130` "（**P3 实现**，此处仅定义契约）", `:172` "（P3 实现）", `sync_config.go:52` "（P3 交付，预留枚举）" |
| `ProviderTTRSS` constant | n/a | ❌ unreferenced | None | **Delete** |
| `ErrUnsupported` | n/a | ⚠️ one producer, one test consumer | None | **Delete or wire up** |
| Diagnostic package (283 LOC, 3 routes) | ❌ **no UI at all** | ❌ | Plausible operator need, zero evidence | **Delete or add a UI entry** |
| `FilterRule.Priority` (`models.go:139`) | ❌ **no UI** — `priority` appears 0 times in `SettingsRulesTab.tsx` and 0 times in `api.ts` | ✅ engine sorts by it | None | **Delete the field or add the control** — a rule engine with an unorderable priority |
| Export article as PNG (html2canvas) | ✅ in share menu | ✅ | None found | **Prove first** — 202 KB unmaintained dep |
| Export as PDF | ✅ | ⚠️ opens print window + `setTimeout(100)` | None found | **Prove first** |
| `RECOMMENDED_SOURCES` (9 hardcoded feeds) | ✅ onboarding | ✅ | Plausible onboarding aid | **Keep, but verify feeds** — CoolShell is defunct |
| Reader themes (paper/sepia/green × light/dark = 6) | ✅ | ✅ | Plausible | **Keep** — cheap, real preference |
| 5 typography knobs (size/line-height/family/spacing/width) | ✅ | ✅ | Plausible | **Keep** — standard for a reader |
| `listDensity` × `listDateFormat` × `listSortOrder` × `listShowPreview` | ✅ | ✅ | Plausible | **Keep** — 3 of 4 are standard |
| `faviconMode` incl. Yandex third party | ✅ | ⚠️ **snapshot-frozen, changes don't apply** | Contradicts privacy positioning | **Replace** — fix reactivity, reconsider third party |
| `trayEnabled` | ✅ | ❌ **nothing reads it** | None | **Delete or wire** |
| `updateAutoDownload` | ✅ | ✅ | Plausible | **Keep** — default off is the right call |
| Filter rules engine (full condition/scope/action model) | ✅ | ✅ | Plausible power-user feature | **Keep** — but note the action set lacks "hide", which is what the placeholder text implies |
| Remote sync (GReader/Miniflux, 2,210 LOC) | ✅ | ✅ | Real for multi-device users | **Keep** — but it dominates the codebase (see §D) |

**Pattern**: the speculative features are not random. They cluster in **backends with no front-ends** (`active`, `Interval`, `FileProvider`, diagnostic package) and in **front-ends with no backends** (`trayEnabled`, S3 option). Features were generated from both directions and never reconciled.

---

## B. Over-engineering metrics

| Metric | Count | Evidence |
|---|---:|---|
| `AppSettings` keys | **45** | Parsed programmatically from `settings.ts` — interface and `DEFAULT_SETTINGS` both enumerate 45, no drift between them |
| Settings tabs | **10** | `SettingsModal.tsx:60-71` |
| API routes registered | **81** | Parsed from `handlers/reader.go` (`api.` + `sensitive.` groups) |
| Routes with no reachable caller (web **or** desktop) | **16** | Independently re-verified with template-literal-aware regex + per-fragment grep; includes the 5 boolean toggle routes `/items/:id/{unread,star,unstar,read-later,unread-later}` whose *client* always sends `/read`-style paths with a body flag |
| Literal duplicate routes (same handler, two paths) | **2** | `/items/index-search/rebuild` (`:224`) + `/cache/rebuild-search` (`:268`) → both `h.RebuildSearchIndex` |
| Dead Go symbols (zero callers) | **6+** | `setDb`, `applyFiltersAndIndex`, `FetchReadability`, `ProxyOriginal`, `compressToZip`, `MoveSourceFolder` |
| Write-only model fields | **2** | `Source.ListRule` (always literal `"rss"` — and onboarding writes `listRule: 'show'`, a value nothing reads), `Source.Interval` |
| Never-written model fields | **1** | `SourceHealth.NextCheckAtUnix` (+ carries an index) |
| No-op migrations | **1** | `migrate_v3_next_check.go` — body is a log line and `return nil` |
| Dead icon exports/aliases | **28** | Scripted grep of all 38 alias names in `icons.tsx:97-136` against every `.ts/.tsx` outside that file |
| `ReaderService` independent caches | **6** | `reader.go:19-56` |
| Distinct cache-invalidation sequence numbers | **4** | `unreadCount` / `httpClient` / `settings` / `filterRules` |
| `invalidateUnreadCount` call sites | **9** | forget one → stale sidebar, no error |
| `App.tsx` | **1,082 lines**, 23 `useState`, 10 `useRef` | — |
| `useRef` "latest closure" patterns | **4**, under **2 contradictory conventions** | `:328` says never write refs during render; `:447`/`:472` do exactly that |
| Duplicated logic instances | **14** | see §E |
| Unread-count implementations | **3–5**, **already divergent and user-visible** | `source_service.go:81` vs `item_service.go:203/217` |
| Scope-filter implementations | **3** | `item_service.go:15`, `:229`, `export.go:48` |
| Backup-name validation copies | **6** | `backup.go:107,461,479,519,561` + `db_handlers.go:158` — and one has already drifted |
| TODOs in frontend source | **2** | both about deprecated `execCommand` (`contextMenu.ts:251,298`). **Every unimplemented feature ships a phase label instead of a TODO** — see §F |
| Product identities in one tree | **5** | `package.json` "flore"; `apps/web/package.json:2` "rss-fetcher-client"; Go module `github.com/rss/go-server`; repo `github.com/icecome/Flore`; `PROJECT.md:34` data path `%LOCALAPPDATA%\RSSReader\` while code writes `%LOCALAPPDATA%\Flore` (`paths.go:11,19`) |

**The 45-key settings panel is not itself the problem** — nearly every key has a real consumer. The problem is **presentation**: all 45 are offered flat, with no tiering between "this reader's genuine preferences" (density, font, sort) and "infrastructure you should not need to touch" (fetch timeout, concurrency, proxy URL). A first-run user opening settings meets 45 controls at once.

**And the copy that fills them is template-shaped**: all 16 rows in `SettingsGeneralTab` render a `title` + `desc` pair where the desc tautologically restates the title — "日期格式" / "文章时间的显示格式。", "显示摘要预览" / "在列表中显示文章摘要。", "标记方式" / "选择自动标记文章为已读的触发方式。" (verified: 16 Rows, 16 `desc=`). The `Row` component requires a desc, so one was generated for every row. Only two descs in the file add information the title does not ("自动抓取周期" explains 304 negotiation; "首次抓取范围" explains why history is limited).

---

## C. Template / generic feel

**Assessment: 3/10 badness — this is the project's strongest dimension.**

Evidence **against** generic-AI feel:

| Signal | Finding |
|---|---|
| Colour system | Cold-tinted neutrals with an explicit rationale in-comment: `柔和冷白，R 微低于 B` (`index.css:7`). Not a stock palette. |
| Dark mode | Full parallel token set (`index.css:83-130`), not an inverted filter — including re-tuned shadows (`rgba(0,0,0,0.2)` vs the light `rgba(25,35,50,0.04)`) |
| Motion | Named easing curve `--ease-out-expo: cubic-bezier(0.16, 1, 0.3, 1)` and a 3-step duration scale |
| Elevation | Four-step shadow ladder, not one generic `shadow-md` |
| Reader themes | 3 authored colour moods × light/dark, each with 5 tuned tokens (`bg`/`color`/`muted`/`link`/`border`) — `settings.ts:14-70` |
| Glass surfaces | Dedicated `--glass-bg` / `--glass-border` tokens, correctly re-tuned per theme |
| Progressive enhancement | `content-visibility: auto` for list virtualisation (`ArticleList.tsx:667`) instead of a JS virtualiser |
| No hero section, no gradient, no icon-grid marketing | Correct for a tool |

Evidence **for** generic feel:

| Signal | Finding |
|---|---|
| Brand mark | A stock Lucide `Rss` glyph in a purple rounded square (`OnboardingWelcome.tsx:42-44`, `TitleBar.tsx:196`). No custom mark, no wordmark treatment. |
| Accent default | `#7B68EE` — "medium slate blue", the single most common AI-default accent |
| Product naming in the UI | The mobile header says **"RSS 阅读器"** (`App.tsx:828`) — the generic category, not "Flore", while the desktop title bar says "Flore" (`TitleBar.tsx:199`) |
| Onboarding structure | Three equal cards in a `grid-cols-3` with icon/title/subtitle — the canonical AI-onboarding shape (`OnboardingWelcome.tsx:52-82`) |
| Font stack | `Inter, "SF Pro Display", "Segoe UI"…` — the default modern stack, undeclared as a brand decision |

**Positive portfolio risk**: the accent default plus the stock RSS glyph plus the three-card onboarding is the combination a reviewer notices first. Everything *underneath* is more distinctive than the surface — which is the opposite of the usual AI tell, and worth fixing because the surface is what gets judged in the first 10 seconds.

---

## D. Where the effort actually went

LOC by area, versus user value for a **local-first** reader:

| Area | LOC | Value | Ratio |
|---|---:|---|---|
| Organize (Sidebar/ArticleList/source/folder/item) | 2,798 | Core | 0.55 |
| **Remote sync** (engine + 2 protocol clients + config + 568-LOC settings tab) | **2,210** | Low for the positioning | **0.15** |
| Read/render (Reader + toolbar + readability + proxy) | 1,924 | Core | 0.70 |
| Updater (desktop) | ~1,450 | Medium | 0.25 |
| Backup/restore | 1,324 | Medium | 0.30 |
| Fetch (fetcher + coordinator + scheduler) | 1,080 | Core | 1.00 |
| Filter rules | ~800 | Medium | 0.35 |
| Export formats | ~600 | Medium | 0.35 |
| Themes/accents | ~500 | Low-Med | 0.20 |
| **Diagnostic** | **283** | **Zero (no caller)** | **0.05** |
| Article retention | ~170 | Medium | 0.60 |

**The single most telling number**: `SettingsSyncTab.tsx` is **568 lines** — larger than `ArticleList.tsx`'s entire feature set (search + multi-select + infinite scroll + unread segmentation). In an app whose `PROJECT.md` states 本地优先 as principle #1.

**Second**: 2,210 LOC of sync + 283 LOC of uncalled diagnostics = **2,493 lines for capabilities whose value is unproven**, while per-source refresh interval and pause-source — both backend-complete — have **zero** lines of UI.

---

## E. Self-contradiction inventory (doc vs reality)

This is the strongest AI-assistance signal in the codebase: not sloppiness, but **two coherent worldviews that never met**.

| # | Claim | Reality | Evidence |
|---|---|---|---|
| 1 | `reader.go:328`: "避免渲染期写 ref（React 并发渲染下不纯）" | `reader.go:447`, `:472` write refs during render | Both conventions documented as house style in the same file, ~130 lines apart |
| 2 | `ARCHITECTURE.md:82`: "reader.go — ReaderService **聚合根**" | One struct holding 6 caches, a hot-swappable `db`, 2 goroutines, coordinator and sync engine; methods are a union of 9 files; the 3 "service layers" are file partitions (proven by cross-file unexported calls) | `reader.go:15-57` |
| 3 | `PROJECT.md`: "本地优先 / 数据完全本地存储" | `favicon-proxy` → Yandex third party | `SourceAvatar.tsx:44`, `api.ts:733` |
| 4 | `PROJECT.md`: "极简依赖" | Upheld — 5 runtime deps. **This claim is true.** | `apps/web/package.json` |
| 5 | `Reader.tsx:549`: "或按 J / K 上下浏览" | No `j`/`k` handler exists anywhere | `App.tsx:364-412` |
| 6 | `AGENTS.md` §5.2: "所有数据获取在 App.tsx 完成" | All fetching lives in `useItemsData`/`useSourcesData`; `useUpdate.ts` is a module-level external store (hand-rolled Zustand) | `hooks/*` |
| 7 | `AGENTS.md` §5.1: "handler 只做参数解析→service→JSON" | `ReaderHandler` owns a rate limiter with its own goroutine and a bounded frame cache; `stop()` is never called | `reader.go:110-119,175` |
| 8 | `icons.tsx:2`: "**Typestream** 图标系统" | The product is called Flore. | `icons.tsx:2` |
| 9 | `icons.tsx:87`: "保留与**原项目**命名兼容" | There is no prior project in this repo | `icons.tsx:87` |
| 10 | `icons.tsx:12`: "别名仅保留语义差异显著的情况" | 39 aliases created; ~30 have zero external references | `icons.tsx:97-136` |
| 11 | `diagnostic.go:173`: `readDesensitizedLog` | Function body is `return readLastLines(filename, 100)` — **no desensitisation** | `diagnostic.go:173-175` |
| 12 | `models.go:49`: `NextCheckAtUnix` — "遗留字段（不再写入）" | Carries a GORM `index`, and migration v3 exists solely to advance the version past it | `models.go:51`, `migrate_v3_next_check.go` |
| 13 | `sync_config.go`: `s3` is an allowed provider | `sync_engine.go:256`: `"s3" not supported yet` | both |
| 14 | `settings.ts:190`: `defaultInterval` — "现为「全量轮询周期」" | `Source.Interval` still exists, is written, and is never read | `models.go:19` |

**Reading of this table**: items 1, 8, 9, 10 are the fingerprints. Item 8 — a header comment naming **a different product** — is the clearest single artefact: the file was generated in the context of another project and carried across. Items 8+9+10 together describe a "compatibility layer" for a predecessor that does not exist in this repository, built to a naming convention the file's own header contradicts.

---

## F. Code-level AI tells

| Tell | Evidence |
|---|---|
| **Ticket-coded comments for an invisible task system** | `T-v03-02`, `T-v03-04`, `T-v02-02`, `T-v02-03`, `m-01`…`m-13`, `M-P1`, `M-R1`, `M-R2`, `M-A3`, `B-08`, `n-02`, `n-09`, `A1`, `P0-1`, `P2-1/B`, `m-14`. **None of these IDs exist anywhere in the repo** — no tracker, no changelog, no reference doc. They are addresses to a conversation that has ended. |
| **Comments narrating a bug fix rather than the code** | `item_service.go:110`: "旧写法引用了不存在的 sources/items/source_id/fail_count 导致快照恒为空（M-R2）". `reader.go:116-118` explains a race fixed by `execLocked`. Useful, but it is a changelog living in the source. |
| **Ceremony in a comment about removing ceremony** | `fetcher.go:104-106` marks `fetchHTTPClient` as removed dead code — a comment about a deleted thing that itself remains. |
| **Defensive checks on impossible inputs** | `main.tsx:13-16`: throws if `#root` is missing. `settings.ts:375-391` coerces every setting against a validator table. (Note: the setting coercion is **legitimate** — settings genuinely arrive from an untrusted API path — but the `#root` check guards a static HTML file.) |
| **Magic-numbered policy with no derivation** | `sync_engine.go:218` `greaderFullPullEvery = 10` — a GReader-specific cadence living in the generic engine, keyed on `provider.Name()`. |
| **Two spellings of the same helper** | `escapeRegex` (`Reader.tsx:221`) and `escapeRegExp` (`ArticleList.tsx:494`) — identical bodies, two names. |
| **The same function twice, 85 lines apart, byte-identical** | `resolveSrc` + `buildProxyUrl` at `Reader.tsx:90-108` and again at `:175-201`. Not refactored, not imported — **generated twice in one file**. |
| **Degenerate structure** | `<button>` nested inside `<button>` (`Sidebar.tsx:732-749`) — invalid HTML that React does not warn about. |
| **Empty-state copy for the wrong state** | One string ("没有更多文章了") serves search-empty, search-failed, unread-cleared, and list-empty (`ArticleList.tsx:428-432`). |
| **A whole file that is dead** | `settings/ThemeToggle.tsx` — zero imports. |
| **Aliases exported for a consumer that never arrived** | `icons.tsx:97-136`. |

**Counter-evidence (real, not charitable)**: the code is genuinely well-organised, consistently named, uses proper Go error wrapping with `%w`, has real concurrency discipline (`sync.RWMutex` with documented non-reentrancy reasoning at `backup_restore.go:36-37`), and writes comments that explain *why* rather than *what*. This is not noise generated without review.

---

## G. THE 10 CLEAREST SIGNS THIS WAS AI-GENERATED

Ranked by how obvious each is to a skeptical outside reviewer. (Ticket codes were originally #1 here; on verification they turned out to be **traceable** to real planning docs, so they dropped off the list — see §F. What replaced them is better evidence.)

| # | Sign | Why it is unmistakable |
|---|---|---|
| **1** | **Ticket-coded comments scattered across 6+ ID schemes, two of which are dangling** | ⚠️ **Weakened on verification.** I initially claimed none of these IDs had referents. That was wrong: `T-v02-*`/`T-v03-*` resolve to `docs/planning/08-dev-tasks-v02-v03-2026-08-10.md`; `m-01…m-13`, `M-P1/2/3/4`, `M-A3`, `n-02/08/09` resolve to `docs/planning/2026-08-29.md`; `T-P0-*` resolves to `09-dev-tasks-sync-p0-2026-08-29.md`. **The convention is coherent and traceable — this is a real practice, not noise.** What survives as a tell is narrower: (a) `B-08` and `M-A3` have **zero** referents anywhere in the repo; (b) the scheme is split across ~6 prefixes with no index tying them together, so tracing one ID requires grepping four documents; (c) the IDs appear in **shipped source comments** (`reader.go:333-336`, `logging.go`), meaning a reader of the code must consult a planning folder to understand why a line exists. That is a documentation-location smell, not a fabrication smell. |
| **2** | **`icons.tsx:2` names a different product** — "Typestream 图标系统" | The single most damning artefact. A file header in a repo called Flore, naming Typestream, with `:87` referring to "原项目" (the original project) — a predecessor that does not exist here. |
| **3** | **Backends built with no front-end, front-ends with no backend** | `Source.active` + `Source.Interval` fully implemented and unreachable; `trayEnabled` fully exposed and unread. Features were generated from both ends independently and never joined. |
| **4** | **Two contradictory conventions in one file, both documented as the rule** | `App.tsx:328` forbids render-time ref writes; `:447`/`:472` do it and call it the convention. Each was written with confidence, ~130 lines apart. |
| **5** | **`FileProvider` + `ReaderPackage` with zero implementations, while WebDAV is hand-rolled outside it** | The abstraction was declared; the implementation was written somewhere else entirely. Textbook generated-scaffold-then-independent-code. |
| **6** | **A no-op migration** — `migrate_v3_next_check.go` logs a line and returns `nil` | A version step that exists only to increment a counter. Written because the sequence had a gap. |
| **7** | **Migration v5 rewrites what v2 was supposed to do** | v2 parsed dates with `RFC3339Nano`, silently skipped GORM's space-separated format, logged a skipped-count that nobody asserted on; v5 replaces it with `strftime`. Two attempts at the same migration, no post-condition test either time. |
| **8** | **The same 20-line helper generated twice, 85 lines apart, in one file** | `Reader.tsx:90-108` and `:175-201`. Importing is free. This is generation without a whole-file view. |
| **9** | **`readDesensitizedLog` does not desensitise** | `diagnostic.go:173-175`. The name records an intent; the body is a pass-through. The intent was written first and the implementation never matched it. |
| **10** | **The app advertises a keyboard shortcut it does not implement** | `Reader.tsx:549` says J/K; no `j`/`k` handler exists. The copy was written from intent, not from the handler table. |
| **11** | **Phase labels instead of TODOs** | Only **2** TODOs exist in the entire frontend (`contextMenu.ts:251,298` — both about deprecated `execCommand`). `FileProvider` (0 impls), the S3 backend, the 16 dead routes, `Source.active`'s missing UI, and `IsReadLater`'s missing sync **all ship without a TODO** — they ship with confident milestone tags (`provider.go:130` "（P3 实现，此处仅定义契约）", `sync_config.go:52` "（P3 交付，预留枚举）") that read as schedule rather than debt. A TODO is a promise; a phase label on unimplemented code is a disguise. |
| **12** | **Five product identities in one tree** | `package.json` = "flore"; `apps/web/package.json:2` = **"rss-fetcher-client"**; Go module = `github.com/rss/go-server`; real repo = `github.com/icecome/Flore`; `PROJECT.md:34` documents the DB at `%LOCALAPPDATA%\RSSReader\reader.db` while `paths.go:11,19` actually writes `%LOCALAPPDATA%\Flore`. Each naming decision was locally reasonable; none were reconciled. |
| **13** | **`safeError` — a security control implemented as English substring matching** | `reader.go:333-346` allow-lists 13 English phrases (`"invalid"`, `"not found"`, `"missing"`, …), passes those through verbatim, and maps everything else to `internal_server_error`. It works. It also pipes raw English into a Chinese UI: `sync_engine.go:256` returns `file provider "s3" not supported yet`, which `SettingsSyncTab.tsx:237` toasts directly to the user. |
| **14** | **Export features that are half-true** | "导出 PDF" is `window.open()` + `document.write()` + `print()` (`Reader.tsx:402-438`) — its own toast admits it: "PDF打印窗口已打开，请选择'另存为PDF'". Its injected `font-family: ${settings.readerFontFamily}` (`:421`) writes the **enum name** (`serif`/`sans`/`mono`) rather than the resolved font stack the screen uses (`:347`). "导出 PNG" forces `backgroundColor: '#ffffff'` (`:375`) regardless of reader theme — dark/sepia/green users export white-background images. |

### Runners-up

- **39 icon aliases for a naming convention the file's own header contradicts** (`icons.tsx:12` vs `:97-136`)
- **Three unread-count implementations that already disagree, one of them user-visible** (`source_service.go:81` lacks the `hideInTimeline`/`isPrivate` filters the other two apply)
- **One dead file, shipped** (`ThemeToggle.tsx`)
- **An audit document in `docs/planning/` recommending cuts that were never made** — the process gap that explains the rest

## H. Conclusion

**Badness: 4.5 / 10.**

The two extremes both miss. This is neither a template-clone nor a carefully hand-crafted product. It is **a competently-engineered system assembled without a single organising hand** — which is exactly what sustained AI assistance produces when the human reviews each change but never the whole.

Three properties distinguish it from a vibecoded prototype:
- the visual system has a stated rationale and complete dark-mode parity;
- the security work (SSRF with TOCTOU closure, CSRF via custom header, constant-time auth, zip-bomb and path guards) is more rigorous than most shipped apps;
- error handling uses real rollback, not `console.log`.

Three properties identify it as AI-assisted:
- **artefacts from a predecessor that was never reconciled** ("原项目", "Typestream", five product identities);
- **features generated from one end only** (backend without UI, UI without backend);
- **the same decision made twice, differently, in the same file.**

The fix is not a rewrite. It is **one pass with a single owner** — the thing the project has never had. Concretely: pick one ref convention, close the four declare-without-implement gaps (`active`, `Interval`, `trayEnabled`, `faviconMode`), delete the three abstractions with no implementers (`FileProvider`, `ProviderTTRSS`, `ErrUnsupported`), and reconcile the five identities into one.

**A correction worth keeping on the record**: the ticket-ID scheme (§F item 1) initially looked like the strongest tell — comments referencing task IDs with no visible system. On verification, `T-v02-03`, `m-03`, `M-P1` and friends **do resolve** to real planning documents (`docs/planning/08-dev-tasks-*.md`, `2026-08-29.md`). That changes the reading: this is a real, if quirky, engineering practice — a changelog distributed through the source — and not a fabrication artefact. The honest version of the criticism is that the scheme spans ~6 prefixes and 4 documents with no index, and that `B-08` and `M-A3` dangle with zero referents. The `Typestream` header (`icons.tsx:2`) and "原项目" (`:87`) remain unexplained and unexplainable from within this repository.

The one artefact I would still delete outright: **`icons.tsx:2`'s "Typestream 图标系统" header**. It names a product that does not exist here, and it is the first thing a code reviewer who greps the brand reads.