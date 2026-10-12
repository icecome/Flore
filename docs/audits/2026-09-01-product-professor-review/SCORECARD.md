# SCORECARD — Flore (RSS Reader)

**Audit date**: 2026-09-01
**Repo**: `Flore` @ `0.3.0-20260901` · 307 tracked files · medium size
**Mode**: `full`

---

## Headline scores

| Metric | Score |
|---|---:|
| **Overall Product Professor Score** | **62 / 100** |
| **Launch Readiness Score** | **38 / 100** |
| **True Score** ("does this feel like a real product?") | **61 / 100** |
| **AI-Generated Feel Badness** | **4.5 / 10** |
| **Expected User Complaint Severity** | **7 / 10** |

**Letter grade: C−**

> The engineering is better than the product. Data integrity, security boundaries, and error-rollback discipline are genuinely above average for a solo/AI-assisted project. The user-facing narrative — first run, state distinction, progress feedback, mobile, accessibility — is consistently half-finished.

---

## Weighted category scores

| Category | Weight | Raw (0–100) | Weighted | Grade |
|---|---:|---:|---:|:--:|
| Product clarity | 10% | 72 | 7.20 | C+ |
| Core functionality | 15% | 74 | 11.10 | C+ |
| UX and user flow | 15% | 45 | 6.75 | D |
| Visual polish | 10% | 78 | 7.80 | B− |
| Architecture | 12% | 52 | 6.24 | D+ |
| Security and privacy | 12% | 76 | 9.12 | B− |
| Performance | 8% | 70 | 5.60 | C |
| Reliability and tests | 8% | 40 | 3.20 | F+ |
| Feature completeness | 6% | 62 | 3.72 | C− |
| Business / launch readiness | 4% | 30 | 1.20 | F |
| **Total** | **100%** | | **61.9** | **C−** |

> **Two scores revised during the evidence gate, both recorded transparently.**
>
> **Security 80 → 76.** A late, dedicated security pass found that the auth boundary is defined by route placement rather than a capability rule: `POST /api/items/export` returns the entire reading history from outside the `sensitive` group, reachable even with the token set, and remote-exploitable in any non-loopback deployment. Combined with token-free reads under a loopback-origin CORS policy, this is a structural gap, not a bug — but it is the kind that the rest of this category's excellent work had obscured.
>
> **Reliability 35 → 40.** My first pass scored it as "the tests do not run", measured on the dirty working tree. **On clean HEAD they do run** (22.2% in `services`). The score is still failing — `handlers`, `database`, `models`, `backend`, `pkg/paths` at 0% each; zero frontend tests; no ESLint; and `urlpolicy.go` untested — but it is failing for the honest reason.
>
> **Feature completeness 66 → 62**, after confirming auto-backup ships off, pause-a-feed has no UI, the S3 option collects credentials for a backend that refuses to work, and the server-stored theme is never applied.

---

## Evidence behind each score

### Product clarity — 72 (C+)
**For**: `docs/architecture/PROJECT.md` states positioning, audience, and explicit non-goals. The name "Flore" is used consistently in `TitleBar.tsx:199`, `OnboardingWelcome.tsx:45`, `SettingsAboutTab.tsx:82`, `useNotification.ts:6`.
**Against**: inconsistency remains — `App.tsx:828` renders the mobile header as **"RSS 阅读器"**, a generic product category, not the brand. Feature scope contradicts positioning: a "本地优先" app ships a 2,210-LOC remote-sync stack.

### Core functionality — 74 (C+)
**For**: The core loop genuinely works. Fetch → parse → dedupe (`Item.link + sourceId` unique index) → FTS5 index → render → mark read/star/read-later, with optimistic update and rollback on failure (`App.tsx:298-351`). Reader has three real modes with a frameable precheck and proxy fallback (`Reader.tsx:481-505`). Backup/restore is complete with a WAL-checkpoint → `VACUUM INTO` → validate → swap → rollback-on-failure sequence (`backup_restore.go:16-113`) that is more rigorous than most commercial apps.
**Against**: **`Source.Interval` and `Source.active` are fully implemented in the backend and unreachable from the UI.** The pause-a-noisy-source feature — table stakes for RSS — has no entry point.

### UX and user flow — 45 (D)
This is the weakest user-facing category. See `USER_COMPLAINTS.md` for the persona evidence. Headlines:
- No global "backend connecting / offline" state. On cold start (up to 15s) the app renders the **first-run onboarding screen** to an existing user with 200 subscriptions, while firing 7 failure toasts that overwrite each other (`MAX_TOASTS = 5`).
- `loadingSources` **exists and is never rendered** — verified: sole occurrence is the destructure at `App.tsx:121`.
- All 122 `showToast` calls use the default `'info'` type, so success and error are visually identical (`toast.ts:69`, `ToastContainer.tsx:6-10`).
- Search-empty and search-failure reuse the copy "没有更多文章了" — written for "you finished reading".
- Long operations (export ≤120s, sync ≤300s) close their modal immediately with no progress and no start confirmation.
- Mobile hover-only controls are unreachable by touch.

### Visual polish — 78 (B−)
**For**: This is *not* generic AI output. `index.css` defines a deliberate token system — cold-tinted neutrals with a documented rationale ("柔和冷白，R 微低于 B"), a 4-step shadow ladder, a 4-step radius scale, a named easing curve (`--ease-out-expo`), and complete light/dark token pairs including glass surfaces and selection color. Radius, spacing, and type scale are applied consistently through Tailwind.
**Against**: 11-12px muted text is used heavily in dense lists (`ArticleList.tsx:731`), which is a legibility problem, not a style choice. The brand mark is a stock Lucide RSS glyph (`OnboardingWelcome.tsx:43`, `TitleBar.tsx:196`).

### Architecture — 52 (D+)
**For**: Dependency direction is genuinely one-way and `internal/sync` is a real adapter with 2 implementations.
**Against**: `ReaderService` (`reader.go:15-57`) is a god object — 6 independent caches, a mutex-guarded hot-swappable `db`, a `sync.Once` process singleton, 2 background goroutines, plus coordinator and sync engine, and a method set that is the **union of 9 files**. `setDb` (`reader.go:109`) replaces the connection and invalidates nothing; `execLocked` (`reader.go:119`) exists solely to patch the race that design creates. `App.tsx` is 1,082 lines with 23 `useState` / 10 `useRef` and **two contradictory ref-sync conventions documented in its own comments** (`:328` says never write refs during render; `:447`/`:472` do exactly that). **16 of 81 routes have no reachable caller**; two bind the same handler.

### Security and privacy — 76 (B−)
**Reviewed in depth**, under a threat model that grades findings on *uplift* over "same-user malware already reads everything" — a loopback desktop app should not be scored against an internet threat model. Details in [`DATABASE_SECURITY.md`](DATABASE_SECURITY.md).

**Genuinely strong (verified, not charitable):**
- **SSRF**: 4 layers (`urlpolicy.go`), including a dial-context that resolves DNS once and dials the validated IP (`:208-243`), closing the rebinding TOCTOU. Applied on **every** outbound path traced: feed fetch, title fetch, readability, image/CSS/favicon proxies, frame precheck, both sync providers, the WebDAV probe. Redirects are re-validated at dial time. Body caps 16MB, redirect cap 5.
- **Zip handling**: declared-size + streamed 256MB cap; the updater's `unzipSafe` blocks zip-slip with a normalized-path double-check **and** refuses to overwrite `data/`/`backups/`.
- **Update pipeline**: fixed manifest URLs with no env override (documented anti-hijack), sha256 + **Ed25519** against an embedded public key, BAT templates with cmd-metacharacter escaping.
- **CSRF / CORS / sandbox**: origin string matching rejects subdomain spoofing and `null`; writes require an unforgeable custom header; sandboxed iframe without `allow-same-origin` for proxied originals; DOMPurify with an explicit allow/deny list.
- **SQL / FTS5**: sanitisation makes injection impossible by construction; LIKE escaping with an explicit `ESCAPE '\'`; the one string-concatenated SQL (`VACUUM INTO`) cannot be parameterized in SQLite and is single-quote-escaped.
- **Desktop shell**: fail-closed 32-byte token (exits on RNG failure rather than degrading), token-masked env logging, **host pinning** so the token can never be forwarded off `127.0.0.1`, Job Object kill-on-parent-death.
- **Honest telemetry**: no analytics/Sentry; the update check carries no version or ID.

**Two findings arrived late and are verified — they move this from 80 to 76:**
1. **The auth boundary is route placement, not a capability rule.** `POST /api/items/export` (`reader.go:226`) returns the **entire reading history** from outside the `sensitive` group — reachable even with the token set, because CSRF only constrains browsers (`security.go:48-53` passes header-less requests by design). Same for `/opml/export`, `/articles/cleanup`, `/database/vacuum`, `/database/info`. Contrast `:271-272`, where DB export is deliberately protected with a comment stating the intent — the codebase's own rule contradicts its own placement. **Remote-exploitable in any non-loopback deployment** (`BIND_ADDR=0.0.0.0` is documented in `.env.example:7-8`).
2. **CORS trusts every loopback origin and reads are token-free** (`security.go:16-29`, any port). Any local web page — another app's admin UI, a dev server — can silently bulk-exfiltrate the whole history and query the FTS index, **even with the token set**: the token never gates reads, and the origin model cannot distinguish the app's webview from any other loopback page.

**Durability findings counted here too**: no `integrity_check` anywhere (0 hits in `server/go`), auto-backup off by default, restore is truncate-then-copy (not crash-atomic), and the diagnostic package leaks username, paths, full feed URLs and panic dumps — `readDesensitizedLog` performs no redaction despite its name.

### Performance — 70 (C)
**For**: `content-visibility: auto` for browser-level list virtualisation (`ArticleList.tsx:667`); `html2canvas` dynamically imported to keep it out of the initial bundle (`Reader.tsx:372`); page size 50 with an `Item.id` tie-breaker for stable offset paging (`item_service.go:72-73`); batch FTS5 indexing through a channel with overflow buffer.
**Against**: 458 KB main chunk / 137 KB gzip is large for a reader that ships no UI library. `fetchersRef` re-fetches the full source list on every selection change (`useSourcesData.ts:85-87`); the unread-count cache is invalidated wholesale from 9 call sites. **Mobile list rows with `opacity-0 group-hover`** (`ArticleList.tsx:733`) cost nothing but signal a desktop-only design.

### Reliability and tests — 40 (F+)
**Deductions, in order of weight:**
- **`urlpolicy.go` has zero tests.** The 258-line SSRF boundary — including the dial-context that resolves DNS once and dials the validated IP to close the rebinding TOCTOU window (`:208-243`) — is the single most valuable correctness asset in the repository and has no coverage at all.
- **Zero test files** in `handlers` (all routes, middleware, CSRF, CORS, auth, rate limiting), `database` (5 migrations + FTS5 bootstrap), `models` (`MilliTime` marshalling), `backend`, `pkg/paths` — **0% each, measured**.
- **Zero frontend tests.** No Vitest/Jest, no `.test.*`/`.spec.*` file.
- **No ESLint config** despite the project maintaining a coding-standards document.
- **4 untracked test files, one of which does not compile** (`item_service_test.go:116`) — CI is green today only because they are uncommitted (P0-001).
- No crash reporting; no `unhandledrejection` handler.
- No dependency audit (`npm audit`, `govulncheck`) despite a network-facing proxy that unpacks user-supplied archives.

**Credit**: measured coverage where it exists is meaningful — `sync` 67.4%, `logging` 34.2%, `services` 22.2%. The `sync` tests exercise both protocol clients against `httptest`, and the desktop updater tests cover signature tampering, zip-slip, and version comparison, and run in CI.

### Feature completeness — 62 (C−)
**Delivered well**: OPML import/export, folders with recursive trees, filter rules with a test-run modal, full-text search with a CJK LIKE fallback, backup/restore at three granularities, article retention (fully wired — `RetentionForm` + a daily scheduler tick), the update flow, three reader modes, remote sync.
**Missing or half-wired — the pattern that defines this product:**
- **Auto-backup ships OFF** (`settings.ts:217`) in a product whose premise is data sovereignty — no protection for a first-week user, no prompt.
- **Pause / disable a feed** has no UI despite a complete, consumed backend (`coordinator.go:165`).
- **Per-source refresh interval** is written by the client and never read by the scheduler.
- **S3 sync is selectable** and collects Access/Secret keys, while the backend returns `not supported yet` — and the only `sync_file_*` consumer is a WebDAV probe that transfers nothing.
- **Feed autodiscovery is absent** — pasting a bare domain fails; `CreateSource` requires an exact feed URL.
- **OPML import has no dedup** — importing twice duplicates everything, while remote sync *does* dedup.
- **Nested folder creation is unreachable** — the backend supports arbitrary depth and the sidebar renders recursively, but `CreateFolder` takes no `parentId`.
- **`FilterRule.Priority` has no UI** — `priority` appears 0 times in `SettingsRulesTab.tsx` and 0 times in `api.ts`, so rule order is not user-settable.
- **Mark-unread on scroll-back is impossible** — `markReadMode: 'scroll'` is one-directional and the count is clamped at 0.
- **The diagnostic package (283 LOC, 3 routes) has no entry point at all.**

### Business / launch readiness — 30 (F)
**Correction to an initial assumption**: I opened this audit assuming Flore was pre-release. **That was wrong.** Two tags exist (`v0.1.0-20260806`, `v0.3.0-20260901`), HEAD equals the version tag, releases publish to GitHub *and* an R2 CDN, and `README.md:7` describes a shipped product built with "Vibe Coding … 不保证使用和维护，随缘更新".

Users already have this installed. The distribution engineering is real and better than the product surface — an Ed25519-signed update manifest, per-asset verification, zip-slip guards, size budgets, three-OS build matrix. But:

- **macOS builds are ad-hoc signed only**; the release workflow's own comment documents the Gatekeeper workaround users must perform. macOS is an **arm64-only** build.
- **Windows binaries are unsigned** — SmartScreen "unknown publisher" on every download.
- **Auto-backup off by default** — a released build where new users have no protection.
- **Two shipped settings do nothing** (`trayEnabled`, `faviconMode`) and a third (S3 sync) collects credentials for a feature that cannot run.
- **The tag points at HEAD while the working tree has 19 modified + 10 untracked files**, several security-relevant (`apps/desktop/app.go`, `internal/updater/updater.go`, `server/go/backend/backend.go`, `handlers/proxy_handlers.go`, `services/reader.go`) — the released artefact and the working tree have diverged.
- `docs/design/` is **empty** despite `AGENTS.md` reserving it for brand/UI specs; `docs/planning/pending/` is asserted to exist in `AGENTS.md` §13.2 and does not.

---

## Scoring hard-limit checks applied

| Rule | Triggered? | Effect |
|---|---|---|
| Launch Readiness ≤ 80 if build fails | ⚠️ **Partially** — the build passes on HEAD; the *working tree's* test build fails and would break CI on commit | Applied (38) — a released product whose dev tree cannot be committed without red CI is not launch-ready |
| Security ≤ 75 if auth/permissions not reviewed | No — auth, sensitive-group routing, CSRF, SSRF, secrets at rest all reviewed in depth | 80 allowed |
| UX ≤ 80 if no mobile state was reviewed | No — mobile reviewed at 375px; it is poor | 45 reflects review |
| True Score ≤ 80 if empty/error/loading states missing | **Yes** — several missing or wrong (P0-002, P1-007) | Applied (61) |
| Overall ≤ 85 if unresolved P0s | **Yes** — 5 P0s open | Applied |

---

## Score confidence

| Category | Confidence | What would change it |
|---|---|---|
| UX and user flow | High | — |
| Reliability and tests | High | — |
| Security and privacy | Medium-High | Would rise with an authorised pentest of the live loopback API |
| Performance | Medium | No runtime profiling was done; scores reflect static review + bundle sizes |
| Visual polish | Medium | Judged from CSS/tokens and markup, **not from rendered screenshots** |
| Business / launch readiness | High | — |

**Not scored / not audited**: `apps/routing-tool/` (read-only by project rule); live runtime behaviour of the Wails desktop shell on macOS (no environment to run it); third-party dependency CVE scan.

---

## Trajectory note

Two of the P0s are one-line-to-ten-line fixes:
1. `item_service_test.go:116` — a single unused variable; restoring it re-enables ≈1,900 lines of coverage across 4 test files **before they are committed and turn CI red**.
2. Binding `j`/`k` — the empty state already advertises them (`Reader.tsx:549`).

And one is a default flip: `backupAutoEnabled: true` (`settings.ts:217`), which converts "a data-sovereignty product with no protection" into "a product with daily backups on day one."

Executing the P0 set would move Reliability to ~60, UX to ~58, Launch Readiness to ~62, and the overall score to roughly **71 (C+)**. The gap to B+ is architectural (god object, App.tsx), not cosmetic.

---

```
To track progress:
  product-professor record-score --auto "After initial audit"
  product-professor compare
```
