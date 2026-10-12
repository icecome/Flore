# DATABASE_SECURITY

**Audit date**: 2026-09-01
**Scope**: SQLite data durability, authn/authz model, SSRF, CSRF/CORS, secrets handling, privacy posture, injection surface.

**Threat model applied**: DESKTOP LOCAL app. Default attack surface = (a) other processes running as the same user, (b) web pages in the user's browser, (c) malicious RSS feed/item content, (d) the network endpoints Flore itself contacts. "Same-user local malware" can already read every file Flore can, so findings are graded on **uplift** over that baseline — inflating loopback findings to internet severity would be dishonest and would obscure the ones that actually matter.

> **Two late findings** arrived after this file was first written (SEC-01, SEC-02), both verified against the code before inclusion. They change the auth picture materially: the boundary is defined by **route placement**, not by a capability rule, and data-shaped reads sit outside it.

---

## A. Database durability — the real risk in a local-first app

### A1. Auto-backup is **off by default** — P1

`settings.ts:217` — `backupAutoEnabled: false`. The scheduler honours it (`scheduler.go:186-188`) and does a genuinely careful job when enabled (backup → cleanup by count and age → VACUUM).

**Consequence**: a user who installs Flore, subscribes 300 feeds, reads for three weeks, and then hits disk corruption or a bad restore loses everything, and the app never once suggested a backup. For a product whose entire value proposition is "your data is yours and lives on your machine", the default is wrong.

**Recommendation**: default `backupAutoEnabled: true` with a conservative interval, or surface a one-time prompt after N articles accumulate.

### A2. Restore path — **well engineered** (no finding)

`backup_restore.go:16-113` is the most careful code in the repository:

1. `PRAGMA wal_checkpoint(TRUNCATE)` before snapshotting (`:25`) — without this, uncommitted WAL data would be silently dropped from the `VACUUM INTO` snapshot.
2. `validateSQLite(tmpPath)` before touching anything (`:29`).
3. `VACUUM INTO` snapshot of the *current* DB while it is still open (`:46`) — chosen specifically to avoid Windows byte-range lock conflicts, with a documented close-and-copy fallback (`:143-163`).
4. `escapeSQLitePath` on the interpolated path (`:46`) because Windows paths can contain single quotes.
5. On copy failure: rollback copy **and** connection re-init regardless of rollback outcome (`:66-75`, `reinitAfterRestoreFailure:128-140`). The comment is explicit: without the re-init, every subsequent DB operation in the process fails on a closed connection and the only recovery is a restart.
6. Post-replace: `database.Init()` → `AutoMigrate()` → `RunMigrations()` → `VACUUM`, each with a failure branch that still re-points `s.db` at a live handle (`:79-106`).
7. Cache invalidation for unread counts and settings after the swap (`:109-111`).

There is even a note (`:36-37`) explaining that `GetSettingInt` cannot be called while `dbMu` is held because Go's `RWMutex` is not reentrant — the author understood the deadlock, not just the code.

### A3. No integrity checking on read — P1 (revised upward)

`validateSQLite` (`backup.go:42-79`) checks the magic header, `PRAGMA schema_version`, and the presence of an `Item` table — good enough to reject a non-database file. But **`grep integrity_check|quick_check` across `server/go` returns 0 hits**, and no corruption detection runs at startup: `database.go:52-87` (`Init`) just opens the file.

**Failure chain**: power loss / disk fault / AV quarantine corrupts `reader.db` → app starts → `AutoMigrate`/`RunMigrations` fail (`backend.go:92-108`) → `Start` returns error → `cmd/main.go:10-13` **exits 1** → the desktop shell's health check fails and marks the backend down (`apps/desktop/backend.go:298-328`), or the port file is never written (`:75-79`).

The user sees a generic "backend not ready". They have **no UI path** to learn the DB is corrupt, find the `reader.db.bak.<ts>` snapshots, or trigger a restore — restore endpoints exist only behind the API (`reader.go:261-282`) and require the app to be working. The only "obvious" fix a user can find is deleting `reader.db`, which destroys the entire history.

**Cost of the guard**: `PRAGMA quick_check` at startup is milliseconds. Combined with auto-backup off by default (A1), this is the highest expected-loss item in the product.

### A3.1 Restore is not crash-atomic and its input validation is shallow — P2

`backup.go:175-197` — `copyFile` calls `os.Create(dst)` at `:182`, which **truncates the live database before the copy begins**. Crash or power loss in that window leaves a truncated main DB → the A3 brick, with the pre-restore `.bak.<timestamp>` (`backup_restore.go:42-53`) present on disk but user-invisible.

Second path: a page-corrupt DB (valid header, broken B-tree pages — e.g. a partially-synced cloud copy of a backup) passes `validateSQLite` (magic + `schema_version` + `Item` table, no `integrity_check`), **replaces the healthy live DB** (`backup.go:66`), and only then fails or half-works. The healthy pre-restore snapshot is eventually rotated out by `cleanupOldBackups`.

**Credit where due**: the surrounding sequence is careful — pre-restore `VACUUM INTO` snapshot, WAL checkpoint, rollback copy **plus** forced reconnection on failure (`backup_restore.go:66-75, 128-140`), and `fsync` on copied files (`backup.go:192-196`). The gap is atomicity (write to a temp file in the same directory, `fsync`, then `os.Rename`) and the absence of an integrity check on the restore source — i.e. the operation users run *when things are already bad* is guarded by the weakest check in the codebase.

### A4. Migration strategy — P1

| Issue | Evidence |
|---|---|
| **v3 is a no-op** | `migrate_v3_next_check.go:9-13` — body is `slog.Info(...)` + `return nil`. It exists only to advance the version counter for a column AutoMigrate already creates. |
| **v5 exists because v2 silently failed** | `migrate_v2` parses `pubDate` with `time.Parse(RFC3339Nano, ...)` (`migrate_pubdate.go:48`), which cannot parse GORM's space-separated format — so rows were skipped. v5 (`migrate_v5_pubdate_strftime.go:18-22`) redoes it with SQLite `strftime`. |
| **No post-condition assertion** | v2 *did* count skips (`migrate_pubdate.go:51`) and log them (`:67`) — but nothing asserted the result, and `migrations.go` has no verification step. A migration that silently does nothing is indistinguishable from one that succeeded. |
| **Newer schema is a hard error** | `migrations.go:75-80` refuses to open a DB with a version above `CurrentSchemaVersion`. Defensible, but for a single-user local app it means a downgrade (or a beta build touching the DB) bricks the user's database with no recovery UI. |
| **Dead column carries an index** | `SourceHealth.NextCheckAtUnix` (`models.go:51`) is never written and never read, yet has `index`. It is also the sole reason migration v3 exists. |

### A5. Connection and concurrency

`database.go:160-161` — pool 20/10, but `ARCHITECTURE.md` §Decision 1 documents `SetMaxOpenConns(1)` (single writer) with WAL for concurrent readers. Consistent. `busy_timeout(5000)` and `auto_vacuum(INCREMENTAL)` are set in the DSN (`:68`).

The `dbMu` hot-swap design is the residual risk: `setDb` (`reader.go:109`) has **zero callers** — the restore path assigns `s.db` directly under the write lock instead (`backup_restore.go:80,87,94,101,136`). So the encapsulated setter is dead while the bypass is live, and `execLocked` (`:119`) exists only to close the window that pattern creates.

---

## B. Authentication and authorization

### B0. The boundary is route placement, not a capability rule — **P1** (two verified findings)

The `sensitive` group is the only auth gate, and membership was decided route by route. Data-bearing operations sit on both sides:

**`reader.go:156-159`** — `AuthMiddleware` returns `nil` for an empty token; **`:179-183`** — `sensitive := api` falls back to the open group.

**Outside `sensitive` (verified placement):**

| Route | Line | What it exposes / does |
|---|---|---|
| `POST /api/items/export` | `:226` | **The entire reading history** — titles, links, full descriptions, authors, read/star state — no Bearer token |
| `GET /api/opml/export` | `:207` | All subscriptions |
| `POST /api/articles/cleanup` | `:294` | Mass-deletes read articles |
| `POST /api/database/vacuum` | `:269` | Write operation that locks the DB |
| `POST /api/items/read-all` + `/mark-read` | `:220-221` | Bulk state mutation |
| `POST /api/sources/create` | `:195` | Data write |
| `GET /api/database/info` | `:265` | Absolute DB path, backup dir, journal mode |
| `GET /api/cache/stats` | `:266` | — |

Contrast `:260-282`, where `database/export`, `database/restore`, and all 10 backup endpoints are deliberately protected — with a comment stating the intent: "备份含全量数据库…防护口径一致" (`:271-272`). `POST /items/export` **is** a full-database export in JSON form. The inconsistency is not a judgement call; it contradicts the codebase's own stated rule.

**Why the CSRF middleware does not save this** (`security.go:48-53`): requests **without an Origin header pass by design** (curl, the desktop shell's Go client). So CSRF is a browser-only barrier. A non-browser local process reaches all of the above regardless of `FLORE_API_TOKEN`.

**Two concrete exploit paths, graded separately:**

- **Desktop (low uplift)**: a same-user process could read `reader.db` directly anyway — so the *data* exposure is small. But the token (`app.go:77-96`, fail-closed, 32 bytes) exists precisely to deny same-machine processes this API surface, and these routes void that guarantee.
- **Any non-loopback deployment — the real P1**: `.env.example:7-8` explicitly documents `BIND_ADDR=0.0.0.0`. In that mode a remote unauthenticated attacker can exfiltrate the full history and destroy read data.

### B0.1 Any local web page can bulk-read the history, even with the token set — **P1**

This is the browser-side half of the same design gap.

- CORS trusts **every loopback origin, any port**: `security.go:16-29` accepts `127.0.0.1` / `localhost` / `wails.localhost` as hostnames with no port check; `backend.go:253-262` wires it as `AllowOriginFunc`; preflight allows `Authorization` + `X-Requested-With` (`backend.go:248`).
- All **reads** are outside `sensitive`: `GET /api/sources` (`:186`), `GET /api/items` (`:210`), `GET /api/items/search` (`:222`), `GET /api/database/info` (`:265`), `GET /api/diagnostic/items` (`:297`).

**Scenario**: the user visits any page served from `http://127.0.0.1:<anyport>` or `http://localhost:<anyport>` — another app's admin UI, a dev server, a compromised local service. Its JS does `fetch('http://127.0.0.1:'+port+'/api/items?limit=200')`. The preflight succeeds (loopback origin, allowed headers). GET needs no CSRF header. And no token, because reads are not gated. The response is CORS-readable. It can page through the entire history, query the FTS index, and read the DB path (username disclosure) via `/database/info`.

The dynamic port is trivially discoverable (B9). The Bearer token never gates reads, so it cannot distinguish "the app's own webview" from "any other loopback page".

**What is correctly blocked**: DNS rebinding from the public internet — the origin string check (`security.go:24-27`) rejects non-loopback hostnames regardless of resolved IP. The hole is strictly **other-local-page → API**.

**Fix direction** (both findings, one change): gate **reads** as well as writes — the token exists precisely to make that distinction unnecessary, and `ApiRequest` host pinning (`app.go:140-150`) means the app's own webview can always present it. Favicon/proxy `<img>` endpoints need a narrower carve-out (they cannot send headers), which is exactly what `setProxyCORS` already models. Then move `/items/export`, `/articles/cleanup`, `/database/vacuum`, `/database/info` behind `sensitive`.

### B1. The default posture is unauthenticated — P1 (by design, but under-communicated)

```go
// handlers/reader.go:156-159
func AuthMiddleware(token string) gin.HandlerFunc {
    if token == "" { return nil }
    ...
// :179-183
auth := AuthMiddleware(apiToken)
sensitive := api
if auth != nil { sensitive = api.Group("", auth) }
```

When `FLORE_API_TOKEN` is unset — the default — `sensitive` **is** `api`. Every "sensitive" route (database export/restore, OPML import, all 11 backup endpoints, settings read/write including plaintext secrets, sync config) is reachable with no credential.

The token itself is well-handled: constant-time comparison with an exact-length pre-check (`:163`), sourced from the desktop shell via `GetAPIToken()` (`api.ts:194-200`).

**Assessment**: this is a deliberate local-first tradeoff, and it is documented (`ARCHITECTURE.md:171`). But it means the *only* browser-facing barrier for writes is the CSRF custom header (B2), and the only barrier against another local process is the unguessable dynamic port. Both are real but neither is strong. For a single-user desktop app, acceptable; for the web build served to multiple users, **not** acceptable — and the web build is a first-class target (`npm run dev:web`, `PROJECT.md` mentions "附带 Web 版").

**Recommendation**: if the web build is offered to anyone but the developer, the token must be mandatory there, and the settings endpoints that return plaintext proxy URLs and sync passwords must never be exposed on a shared origin.

### B2. CSRF — correctly designed for the loopback model — **no finding**

`security.go:40-66`:
- GET/HEAD/OPTIONS skipped (so `<img>`/`<link>` proxy consumption still works).
- Requests **without** an `Origin` header are trusted (curl, the desktop shell's Go client).
- `Origin: null` rejected (file:// pages, sandboxed iframes).
- Non-local origins rejected.
- Local origins must additionally carry `X-Requested-With: XMLHttpRequest`.

The client cooperates correctly: `fetchData.ts:31-36` adds `X-Requested-With` only for non-GET/HEAD, deliberately avoiding preflights on reads.

Cross-origin form posts and `fetch` in no-cors/simple mode cannot set a custom header without a preflight, and CORS (`backend.go:260` + `IsLocalOrigin`) rejects non-local origins. **This is a correct, well-reasoned CSRF design.** The trust decision "no Origin ⇒ non-browser client" is the one soft edge: a same-origin page could omit `Origin` on some request shapes, but same-origin already implies trust.

`IsLocalOrigin` (`:16-29`) matches on **hostname only**, which correctly covers `wails://`, `http://wails.localhost`, and dev ports while rejecting `wails.localhost.evil.com` — exact-match, not suffix-match. Good.

### B3. Rate limiting is inert in the default configuration — P3 (informational)

`reader.go:93-96` exempts `127.0.0.1` and `::1` outright, with a comment explaining why (React StrictMode double-invoke, polling overlap). Since the backend binds loopback, **the limiter never fires in the default desktop configuration.** It is real protection only for a non-loopback web deployment. Do not "fix" the exemption — it is correct — but do not count the limiter as a defence-in-depth layer for desktop either.

### B4. Route surface is accumulated, not designed — P2

81 routes registered (`api.` + `sensitive.` groups in `handlers/reader.go`); **16 have no reachable caller** in either `apps/web/src` or `apps/desktop`. Several are non-sensitive, so they are permanently unattested attack surface:

| Route | Concern |
|---|---|
| `GET /database/info` (`reader.go:265`, **not sensitive**) | Returns the **absolute database file path**, file size, backup directory path, and `PRAGMA journal_mode` (`setting_service.go:71-99`). Path disclosure to any local process or any page that can reach the port. |
| `GET /cache/stats`, `POST /cache/clear-readability`, `POST /database/vacuum`, `POST /cache/rebuild-search`, `POST /items/index-search/rebuild`, `POST /items/read-later/clear`, `POST /items/:id/apply-filters` | All non-sensitive, all with no caller. `POST /database/vacuum` is a write operation that locks the database, reachable unauthenticated. |
| `/items/index-search/rebuild` (`:224`) **and** `/cache/rebuild-search` (`:268`) | Both bind `h.RebuildSearchIndex` — the same handler at two paths. |

Also inconsistent: three different create verbs (`POST /sources/create` vs bare `POST /folders` and `POST /filter-rules`); six boolean toggle routes (`/read`+`/unread` etc.) implemented as three helpers while the client picks the path with a ternary anyway (`api.ts:333-345`).

---

## C. SSRF — the strongest part of the codebase

### C1. Four layers, correctly applied — **genuinely good, no finding**

`urlpolicy.go`:
1. Scheme allow-list http/https (`:20-23`, enforced `:139,177`).
2. Cloud metadata deny-list (`:72-78`): `metadata.google.internal`, `metadata.azure.com`, `metadata.tencentyun.com`, `169.254.169.254`, `in-addr.arpa` — exact-or-suffix match (`:153-156,188-191`).
3. Private/reserved IP rejection (`isPrivateIP:83-97`) — loopback, RFC1918, link-local, multicast, unspecified, plus explicit `0.0.0.0/8`, with IPv4-mapped IPv6 handled via `To4()`.
4. **The DNS-rebinding fix** (`newSSRFDialContext:208-243`): IP literals are validated then dialled directly; hostnames are resolved **once** through `lookupHostWithCache`, every returned IP is validated, and the connection is dialled to the **validated IP string** (`:235` `net.JoinHostPort(ip, port)`), never back to the hostname. This closes the TOCTOU window between validation and connection.

Applied on **every** fetch path I traced: `ValidateURL` at `source_service.go:153,204` and `sync_config.go:101-104`; `ValidateURLOnly` at `fetcher.go:290,428`, `readability.go:200,247,255,301,387`, `folder_service.go:276` (OPML import); `TransportWithSSRFProtection` at `fetcher.go:258`, `readability.go:183`.

**And critically on the proxy branch** (`fetcher.go:247-253`), with an explicit comment stating that omitting it would let an attacker use the configured proxy to reach the LAN. That is the mistake most implementations make, and this codebase documents having avoided it.

Redirect following is capped at 5 (`fetcher.go:265-270`, `readability.go:177-179`), and because the cap is enforced through the same SSRF dial-context, a redirect to an internal address is re-validated at dial time rather than trusted from the initial check. **This is correct.**

### C2. SSRF has zero tests — **P0**

`urlpolicy.go` is 258 lines of the subtlest, highest-value logic in the repository, and no test file covers it. `isPrivateIP` boundary behaviour (IPv6-mapped, `0.0.0.0/8`, link-local) and the rebinding dialer are exactly the kind of code a future refactor breaks silently. See issue P0-004.

### C3. Proxy endpoints: correctly hardened, one cosmetic inconsistency — P3

`/image-proxy` (`proxy_handlers.go:101-144`) validates the URL, enforces an image content-type allow-list that explicitly excludes `image/svg+xml` (`reader.go:313-327`), caps the response at 10MB (`:138`), and reflects CORS only to local origins (`:136`). `/css-proxy` and `/favicon-*` follow the same shape.

**Inconsistency**: `readability.go:321` sets `Accept: ...image/svg+xml...` on the *outbound* request while the *inbound* handler rejects SVG. Harmless (we fetch what we then refuse), but it advertises a capability the server disclaims — a small sign the two ends were written independently.

`/css-proxy` itself has **no caller**: `RewriteCSSUrls` (`proxy_content.go:148,163`) emits only `/image-proxy` URLs. The route is dead.

---

## D. Secrets and privacy

### D1. Sync credentials are stored in plaintext — P1 (accepted risk, but undocumented to the user)

`sync_config.go:62-77` reads `sync_api_password`, `sync_api_token`, `sync_file_password`, `sync_file_access_key`, `sync_file_secret_key` straight from the `Setting` table. The table is an unencrypted SQLite key-value store (`models.go:154-157`).

`maskSecret` (`:65-76`) is **read-path only** — it prevents the UI from echoing secrets back, and `isMaskedSecret` (`:139`) lets a masked value round-trip without overwriting. That is correct and useful for its purpose. It is **not** protection at rest: anyone who can read `reader.db` has every credential in cleartext, and `GET /api/settings` (sensitive, but unauthenticated by default per B1) returns them raw.

**Recommendation**: at minimum, state in the sync settings UI that credentials are stored unencrypted on disk. OS keychain storage is the proper fix but is a large project.

### D2. Log redaction is real but bypassed by the diagnostic package — P1

Redaction (`logging.go:124-137`, installed via `NewDesensitizeHandler` at `backend.go:87-89`) covers exactly 8 keys: `url`/`referer`/`ref` reduced to scheme+host, `source` prefixed, `path`/`db_path`/`database` replaced with `[REDACTED_PATH]`, and `error` messages having embedded URLs reduced to scheme+host.

**The gap**: `diagnostic.go:60-61` reads the raw log *files* through `readDesensitizedLog` (`:173-175`) — whose body is `return readLastLines(filename, 100)`. **The function name promises desensitisation and performs none.** Because the handler already redacts at write time, the file contents are mostly already scrubbed — so this is not a live leak of the redacted keys. But:

- Redaction is **key-based only**. Free-form message text is not scrubbed (`logging.go` applies `DesensitizeAttr` to attrs; `slog.Info("...")` messages pass through). Any secret interpolated into a message body lands in the log and therefore in the diagnostic package.
- The diagnostic package's `db-snapshot.json` (`diagnostic.go:111`) contains every **source URL and host** — i.e. the user's complete reading subscription list — plus failure counts, item counts, and the last 20 error lines.

**Impact**: a user who shares a diagnostic zip on a public issue tracker publishes their entire subscription list. Given the diagnostic subsystem has **no UI** (nobody can generate one through the app), the practical exposure today is near zero — but the moment P1-006/diagnostic UI is added without fixing this, it becomes a real privacy footgun.

**Recommendation**: make `readDesensitizedLog` actually desensitise, and add an explicit warning + a "sources included, review before sharing" disclosure to any future diagnostic UI.

### D3. Third-party favicon service contradicts the privacy positioning — P2

`/favicon-proxy` proxies to **Yandex** (`api.ts:733-735`, consumed at `SourceAvatar.tsx:44`). A privacy-positioned local-first app (`PROJECT.md`: "关注隐私和数据自主权的用户（数据完全本地存储）") offers a setting that sends every subscribed domain to a third-party Russian service.

**Mitigating**: `faviconMode` defaults to `'off'` (`settings.ts:240`), so nothing is sent unless the user opts in. `SettingsNetworkTab.tsx:77-84` does present it as a first-class option alongside `direct` (source-site fetch), with no explanation of what is disclosed.

**Recommendation**: either drop the Yandex option (the `direct` mode covers the use case without a third party), or label it explicitly: "向 Yandex 发送你访问的站点域名".

### D4. Update check — clean — **no finding**

`updateCheckOnStartup` defaults true with a 3-day throttle (`App.tsx:53,96-99`) and hits the project's own R2 bucket via the desktop shell. No telemetry, no analytics, no crash reporting anywhere (`ErrorBoundary.tsx:27-28` explicitly comments "仅 console.error，不涉及第三方上报"). For a local-first app this is honest.

### D5. Injection surface — well handled — **no finding**

| Vector | Control |
|---|---|
| FTS5 query injection | `sanitizeFTS5Query` (`item_service.go:277-284`) strips everything but alphanumerics/spaces before appending `*` |
| LIKE fallback | `%`/`_`/`\` escaped (`:314-319`) |
| Parameterised queries | Consistent `?` placeholders throughout; the only string interpolation into SQL is `VACUUM INTO '%s'` (`backup.go:34,267`, `backup_restore.go:46`), mitigated by `escapeSQLitePath` quote-doubling (`backup.go:23`) — worth a comment noting why it cannot be parameterised (SQLite does not allow bound parameters there) |
| XSS in article HTML | `DOMPurify.sanitize` with an explicit config (`Reader.tsx:43-50,610`) applied on **both** the readability and the RSS-desc render paths |
| `javascript:`/`data:` links | Protocol allow-list in `openExternal` (`api.ts:136-146`); anchor interception in reader content (`Reader.tsx:508-521`) |
| Path traversal | Diagnostic output confined to `os.TempDir()` with a **separator-aware** prefix check (`diagnostic.go:36-43,296-301` — the comment notes that omitting the separator would let `C:\...\Temp2\x` bypass a naive prefix match); `..` rejected (`:288`) |
| Zip bombs | Declared-size check + 256MB streaming cap (`backup.go:103,134,144`) |
| Upload size | 256MB (DB restore), 10MB (OPML) |
| Batch size | 500 (delete-batch, mark-read), 5000 (export ids), 200-char search, 100 keys / 10KB per settings value |
| iframe escalation | Proxy mode drops `allow-same-origin` from `sandbox` (`Reader.tsx:631-635`); direct mode keeps it, gated by a frameable precheck |

### D6. Backup-name validation duplicated 6× and already diverged — P1

Identical conditions repeated at `backup.go:107,461,479-484,519,561` plus `db_handlers.go:158`. The copy at `:479` splits the `.zip` suffix check into a second `if`, producing a **different error string**, which `safeError` (`reader.go:333-346`) then maps to a different client-facing message. Security validation duplicated six times with one already drifted is a maintenance hazard, not a style issue.

---

## E. What is genuinely well built (do not cut any of this)

| Asset | Why it must survive future refactoring |
|---|---|
| `newSSRFDialContext` + its use on the **proxy** branch | Closes a real TOCTOU window; the proxy-branch application is the detail most implementations miss and this codebase comments on it |
| `restoreFromFile`'s full sequence incl. rollback re-init | Prevents total data loss and prevents the "closed connection" zombie state |
| `CSRFProtection` + `IsLocalOrigin` hostname-only matching | Correct for the loopback model; rejects `wails.localhost.evil.com` |
| Constant-time token compare with length pre-check | Standard but done properly |
| SVG rejection on the image proxy | Feed content is attacker-controlled; SVG executes |
| `DOMPurify` on both render paths, with a config | The XSS boundary for the app's entire content surface |
| Separator-aware temp-dir confinement in diagnostics | The comment shows the author knew the naive version was bypassable |
| WAL checkpoint before snapshot | Without it, restores silently lose data |
| `execLocked` (`reader.go:119`) | Patches a documented race (m-03) between backup/export and concurrent DB replacement |
| Loopback rate-limit exemption | Correct local-first decision — **do not "tighten" this** |
| `ENUM_VALIDATORS` + `coerceSettingValue` (`settings.ts:359-391`) | Settings arrive from an untrusted API path where `String()` round-tripping genuinely produces `"true"` and `"7"` — this is boundary validation, not defensive noise |

---

## F. Top security/data risks, ordered by real impact

| # | Risk | Severity | Realistic impact |
|---|---|---|---|
| 1 | **Corrupt DB = silent total loss** — no `integrity_check` (0 hits in `server/go`), backend exits, auto-backup off, no recovery UI | P1 | Highest expected loss. The flagship promise ("your reading history lives here") has no runtime durability invariant. |
| 2 | **Data-shaped endpoints outside `sensitive`** — `/items/export` (full history), `/articles/cleanup` (mass delete), `/database/vacuum`, `/database/info` reachable without the token | P1 | Remote-exploitable in any non-loopback deployment (`BIND_ADDR=0.0.0.0` documented in `.env.example:7-8`); voids the token boundary locally. |
| 3 | **CORS trusts every loopback origin + token-free reads** | P1 | Any local web page can silently bulk-exfiltrate history, subscriptions, and the FTS index — even with the token set. The origin model cannot distinguish the app's webview from another loopback page. |
| 4 | **SSRF boundary has no tests** and the test suite's safety net is one commit away from breaking | P0 | A refactor silently drops the dial-context check; nothing catches it. LAN pivot via a malicious feed or a crafted `?url=` on an unauthenticated proxy endpoint. |
| 5 | **Restore is not crash-atomic** (truncate-then-copy) with shallow input validation | P2 | The recovery tool is the sharpest durability edge; a corrupt restore source replaces the healthy DB. |
| 6 | **Diagnostic package leaks username, paths, full feed URLs, panic dumps** — and `readDesensitizedLog` performs no redaction | P2 | A support-channel data leak that contradicts the privacy posture. |
| 7 | **Plaintext credentials in DB and in every unencrypted backup zip**; `maskSecret` is UI-only | P2 | Shared/moved/synced backup zips carry live FreshRSS/Miniflux/WebDAV/S3 credentials. |
| 8 | **Auto-backup off by default** in a data-sovereignty product | P1 | Total silent data loss on the first disk/DB failure. No warning ever shown. |
| 9 | **Migration v3 is a no-op; v5 exists because v2 silently failed; no post-conditions** | P1 | A future migration can silently do nothing and report success. |
| 10 | **Backup-name validation duplicated 6×** | P3 | Drift-prone; one copy has already split its checks. |
| 11 | **16 unreachable routes, several unauthenticated writes** (`POST /database/vacuum`, `POST /cache/clear-readability`) | P2 | Unattested attack surface; a local process can lock or degrade the database. |
| 12 | Yandex favicon proxy + default-on update check vs privacy posture | P3 | Disclosed deltas; the comment at `proxy_handlers.go:197` misstates what Yandex receives (domains **and** the user's IP). |
| 13 | Port file `0644` with a predictable name in `%TEMP%` | P3 | Not a secret in empty-token modes; harden and stop treating it as a control. |

---

## G. Verdict

**Security engineering: B (78/100).** Above the standard for a solo project and above many shipped desktop apps. The SSRF work and the update-integrity pipeline are professional-grade.

**Data durability: C (65/100).** The restore *sequence* is excellent; the *invariants* around it are missing — no integrity monitoring, no crash-atomic replace, no recovery UX, auto-backup off by default.

**The two systemic weaknesses are structural, not local:**

1. **The auth boundary is defined by route placement rather than a capability rule.** Data-shaped reads and exports leak around it (B0, B0.1), and the default read set is exactly what makes the loopback CORS model exploitable. The fix is one rule — "everything that returns user data requires the token, with a narrow `<img>`-consumable carve-out for the proxy family" — rather than re-litigating 81 routes.

2. **Durability is treated as a backup feature, not a runtime invariant.** No integrity monitoring, no crash-atomic restore, no recovery UX, backups off by default (A1, A3, A3.1). For a product whose stated value is "your data is yours", the data has no runtime protection.

Both are fixable with modest, well-scoped changes: gate reads as well as writes, run `PRAGMA quick_check` at startup with a recovery dialog, make restore temp-file+rename atomic, and flip the backup default.