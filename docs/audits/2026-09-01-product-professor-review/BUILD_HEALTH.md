# BUILD_HEALTH

**Audit date**: 2026-09-01
**Repo at**: HEAD = `a268fbf` (= tag `v0.3.0-20260901` = origin/main) · **working tree is dirty** (19 modified + 10 untracked files, including 4 untracked Go test files)

> ⚠ **Correction to the initial audit pass**: I first ran the commands against the dirty working tree and concluded "CI is red on main". That was wrong. Verified by `git worktree add --detach` on clean HEAD: **`go vet` and `go test` both exit 0 on HEAD.** The failing test file (`item_service_test.go`) is **untracked** — CI is currently green for what is pushed, and red for the tree developers actually work in. Both facts matter; see below.

---

## Command results

### Against the working tree (dirty)

| # | Command | Working dir | Exit | Result |
|---|---|---|:--:|---|
| 1 | `npx tsc -b` | `apps/web` | **0** | Pass — no type errors |
| 2 | `npx vite build` | `apps/web` | **0** | Pass — built in 2.44s |
| 3 | `go build ./...` | `server/go` | **0** | Pass |
| 4 | `go vet ./...` | `server/go` | **1** | **FAIL** — test build error |
| 5 | `go test ./...` | `server/go` | **1** | **FAIL** — 1 package failed to build |
| 6 | `go build ./...` | `apps/desktop` | **0** | Pass |
| 7 | `go vet ./...` | `apps/desktop` | **0** | Pass |
| 8 | `go test ./...` | `apps/desktop` | **0** | Pass — `updatelet` + `updater` |
| 9 | `gofmt -l` | both modules | **0** | Pass — no unformatted files |
| 10 | ESLint | `apps/web` | n/a | **NOT CONFIGURED** — no `.eslintrc*` / `eslint.config.*` anywhere |
| 11 | Frontend test files | `apps/web` | n/a | **ZERO** — no `*.test.*` / `*.spec.*`, no Vitest/Jest |

### Against clean HEAD (`git worktree add --detach HEAD`)

| Command | Exit | Result |
|---|:--:|---|
| `go vet ./...` | **0** | ✅ Pass |
| `go test ./...` | **0** | ✅ Pass — `internal/services` **ok** (8.2s), `sync` ok (6.3s), `logging` ok |

**So: CI is green on `main` today.** The red state is a property of the working tree only.

**But the exposure is real and imminent**: four test files are untracked —
```
?? server/go/internal/services/item_service_test.go
?? server/go/internal/services/filter_test.go
?? server/go/internal/services/folder_service_test.go
?? server/go/internal/services/source_service_test.go
?? apps/desktop/internal/updater/updateinfo_test.go
```
— and `item_service_test.go` **does not compile**. The moment these are committed, `go vet` fails, and because `ci.yml` runs `go vet` as its second step, steps 3–7 (including the frontend typecheck and build) stop executing on every push and PR. **A developer following the normal "write tests → commit" flow will turn CI red for the whole project**, and the four new test files — the only coverage for `filter`, `folder_service`, `source_service` — will never actually run in CI.

**Fix**: `item_service_test.go:116` — `normal, _ := seedItems(t, s)` declares `normal`, which is never used (the test queries via `db.Where("title = ?", "n1")` instead). Change to `_, _ :=` or use the value. One character; unblocks committing ≈1,900 lines of coverage.

## Coverage on HEAD (measured)

| Package | Coverage |
|---|---:|
| `internal/sync` | 67.4% |
| `internal/logging` | 34.2% |
| `internal/services` | 22.2% |
| `internal/handlers` | **0% (no test files)** |
| `internal/database` | **0% (no test files)** |
| `internal/models` | **0% (no test files)** |
| `backend` | **0% (no test files)** |
| `pkg/paths` | **0% (no test files)** |
| `apps/web` | **no test framework** |

---

## Failures in detail

### `go vet ./...` / `go test ./...` — **working tree only** — P0

```
# github.com/rss/go-server/internal/services
# [github.com/rss/go-server/internal/services]
vet.exe: internal\services\item_service_test.go:116:2: declared and not used: normal
```

Full `go test ./...` output (working tree):

```
?   github.com/rss/go-server/backend            [no test files]
?   github.com/rss/go-server/cmd                [no test files]
?   github.com/rss/go-server/internal/database  [no test files]
?   github.com/rss/go-server/internal/handlers   [no test files]
ok  github.com/rss/go-server/internal/logging
?   github.com/rss/go-server/internal/models     [no test files]
# github.com/rss/go-server/internal/services [github.com/rss/go-server/internal/services.test]
internal\services\item_service_test.go:116:2: declared and not used: normal
FAIL github.com/rss/go-server/internal/services [build failed]
ok  github.com/rss/go-server/internal/sync
?   github.com/rss/go-server/pkg/paths          [no test files]
FAIL
```

**Consequence**: one unused variable in one **untracked** test file prevents **all 8 test files in `internal/services`** from compiling — including `source_service_test.go` (asserts partial-update does not clobber `Interval`/`Active`), `item_service_test.go`, `filter_test.go`, `folder_service_test.go`, `sync_engine_test.go`, `data_path_test.go`, `coordinator_test.go`, `fetcher_dedup_test.go`. Roughly 1,900 lines of coverage are currently unrunnable.

**CI impact** — `.github/workflows/ci.yml` runs, in order:
1. `gofmt -l` → passes
2. `go vet ./...` → **would FAIL on the current tree → job aborts here**
3. `go test -cover ./...` → never reached
4. `tsc --noEmit`, `go build`, desktop compile check, updater tests, `build:web` → never reached

**Two CI workflows, not one**: `build-desktop.yml` additionally runs `wails build` on Windows/macOS/Linux on every push — heavy, and a strong safety net for platform breakage (not verified locally; `needs confirmation` for Linux webkit pins).

**This is a latent P0, not an active one.** CI is green on `main` right now. It turns red the instant the four untracked test files are committed — which is exactly the next thing a developer would do, since they are the coverage for `filter`, `folder_service`, and `source_service`.

---

## Test coverage inventory

| Package | Test file | Status | Lines |
|---|---|---|---:|
| `internal/logging` | ✅ | Passing (desensitise + rotate) | 175 |
| `internal/sync` | ✅ ×2 | Passing (GReader + Miniflux vs httptest) | 618 |
| `internal/services` | ⚠️ ×8 | **NOT EXECUTING** (build failure) | ~1,900 |
| `internal/handlers` | ❌ | **None** — all routes, middleware, CSRF, CORS, auth, rate limit untested | 0 |
| `internal/database` | ❌ | **None** — 5 migrations, AutoMigrate, FTS5 bootstrap untested | 0 |
| `internal/models` | ❌ | **None** — `MilliTime`/`NullableMilliTime` Scan/Value/MarshalJSON untested | 0 |
| `backend` | ❌ | **None** — CORS config, port resolution, shutdown untested | 0 |
| `pkg/paths` | ❌ | **None** | 0 |
| `apps/desktop` | ✅ | `internal/updater` (verify, zip, version) — runs in CI | ~270 |
| `apps/web` | ❌ | **No test framework at all** — no Vitest/Jest, no `.test.*`/`.spec.*` file | 0 |

### The coverage gap that matters most

`internal/services/urlpolicy.go` — 258 lines implementing the SSRF boundary — has **zero tests**. It is the single most valuable correctness asset in this codebase: the dial-context at `:208-243` resolves DNS once and dials the validated IP, closing the DNS-rebinding TOCTOU window, and is applied on both the direct and proxy transports (`fetcher.go:249`). Regressing this silently would be a security incident with no test to catch it.

Also untested and equally risky: path-traversal confinement (`diagnostic.go:296-301`), zip-bomb guards (`backup.go:103,144`), SQLite restore validation (`backup.go:42-79`).

---

## Frontend build

```
vite v6.4.3 building for production...
✓ 1865 modules transformed.
dist/index.html                            2.38 kB │ gzip:   1.19 kB
dist/assets/index-*.css                   33.34 kB │ gzip:   7.46 kB
dist/assets/html2canvas.esm-*.js         202.38 kB │ gzip:  48.04 kB
dist/assets/index-*.js                   458.57 kB │ gzip: 137.30 kB
✓ built in 2.44s
```

**Assessment**: healthy. `html2canvas` is correctly code-split (dynamic `import()` at `Reader.tsx:372`) so its 202 KB never touches first paint — good practice, deliberately applied. The 458 KB main chunk (137 KB gzip) is on the heavy side for an app that ships **no UI library and no router** (dependencies are only `dompurify`, `html2canvas`, `lucide-react`, `react`, `react-dom`). `lucide-react` is a known tree-shaking hazard: `icons.tsx` re-exports 65 symbols plus 39 aliases; if any is referenced in a way that defeats tree-shaking, unused icons ship to the user.

**Type safety**: `tsconfig.json` sets `"strict": true` but **`noUnusedLocals: false` and `noUnusedParameters: false`**. This is why the class of dead code this audit found — `ThemeToggle.tsx` (entire file), ~39 dead icon aliases, 3 unused destructure bindings in `App.tsx:121-122`, `usePlatform`'s `isWindows`/`isLinux` — compiles without a single warning. Turning these two flags on would surface the entire dead-code class automatically.

---

## Dependency health

| Package | Declared | Installed | Latest on npm | Assessment |
|---|---|---|---|---|
| `react` / `react-dom` | `^19.0.0` | 19.x | 19.x | Current |
| `lucide-react` | `^1.26.0` | **1.29.0** | **1.55.0** | Current line, but ~26 minor versions behind |
| `vite` | `^6.0.0` | 6.4.3 | — | Current major |
| `typescript` | `^5.6.0` | 5.x | — | Current |
| `tailwindcss` | `^3.4.19` | 3.4.x | 4.x | **Major version 4 available**; v3 is in maintenance |
| `dompurify` | `^3.4.12` | 3.4.13 | 3.4.x | Current — important, this is the XSS boundary |
| `html2canvas` | `^1.4.1` | 1.4.1 | 1.4.1 | Stagnant upstream (last release 2022) |

**Note**: `lucide-react` at `1.x` is a fork line distinct from the widely-used `0.x`. It is installed and resolving correctly, so this is not a defect — but it is unusual enough to warrant a deliberate decision rather than drift.

**`html2canvas` being unmaintained** is the one real supply-chain concern: it is a 2022-era package used to implement "export article as PNG" — a speculative feature with no usage evidence. It carries 202 KB (48 KB gzip) of dormant dependency surface for a feature the audit could not evidence any user demand for.

No third-party runtime dependency outside the React/lucide/DOMPurify/html2canvas set — the "极简依赖" principle in `PROJECT.md` is honestly upheld.

---

## CI configuration assessment

`.github/workflows/ci.yml` is **well-designed** and would catch most regressions:

- ✅ `gofmt -l` gate (matches the local `.githooks` pre-push guard)
- ✅ `go vet`, `go test -cover`
- ✅ TypeScript `--noEmit`
- ✅ Go build + desktop compile check
- ✅ Desktop updater security tests (signature verification, zip-slip, version compare) — with a comment noting these "此前从未在 CI 执行"
- ✅ Frontend production build

**What is missing**:
- ❌ No lint step (no ESLint config exists to run)
- ❌ No frontend tests (none exist)
- ❌ No dependency audit (`npm audit`, `govulncheck`) despite the project shipping a network-facing proxy and unpacking user-supplied archives
- ❌ No coverage threshold
- ❌ No matrix build for macOS — `release.yml` builds darwin-arm64, but `ci.yml` runs `ubuntu-latest` only. A macOS-only compile break would ship broken macOS binaries.
- ❌ Release workflow publishes **unsigned macOS builds**. Its own header comment documents the consequence: users must right-click → Open, or run `xattr -d com.apple.quarantine`. For a product whose distribution premise is "download and run", this is a significant adoption barrier and will generate support burden.

---

## Repository hygiene

| Check | Status |
|---|---|
| `git status` | **Dirty**: 19 modified + 10 untracked tracked files. HEAD = `a268fbf` = tag `v0.3.0-20260901` = origin/main. |
| Uncommitted security-relevant code | ⚠️ `apps/desktop/internal/updater/updater.go`, `apps/desktop/update.go`, `apps/desktop/app.go`, `server/go/internal/handlers/proxy_handlers.go`, `server/go/backend/backend.go`, `server/go/internal/services/reader.go` — plus the whole untracked `SettingsAppearanceTab` / `SettingsGeneralTab` / `useUpdate` stack |
| `.gitignore` coverage | Present; `dist/`, `bin/`, `build/`, `graphify-out/` excluded per project convention |
| Build artefacts committed | None found |
| `*.db` / `.env` committed | None found |
| Secrets in repo | None found. `backend.go:288-296` masks `TOKEN`/`SECRET`/`PASSWORD`/`APIKEY` keys when logging the subprocess env |
| Docs freshness | `ARCHITECTURE.md` is **accurate** — I diffed its 60 documented `/api/*` routes against `handlers/reader.go` and all resolve except `/api/shutdown`, registered in `backend/backend.go:126` instead. **Documentation is better maintained than typical** |
| AGENTS.md accuracy | Lists 10 settings tabs and the component inventory; both match the tree. The claim "组件禁止直接引用 lucide-react" holds (only `icons.tsx` imports it). **Drift found**: AGENTS.md §2.2/§6.4 reserves `docs/design/` for brand/UI specs — **it is empty (0 files)**; §13.2 asserts `docs/planning/pending/` exists — **it does not**. §7 omits `useUpdate.ts`, `proxy_content.go`, `updatelet/` |
| README self-description | `README.md:7` — "Flore 是一个使用 **Vibe Coding** 方式制作的 RSS 阅读器…**不保证使用和维护，随缘更新**" |

**Notably good**: `.githooks/` with a gofmt pre-push guard, documented in AGENTS.md §12.5; three GitHub workflows including a release pipeline with an **Ed25519-signed update manifest** (`updater/verify.go`) and per-asset signature verification, size-budget gates, and a self-built updater with zip-slip protection. For a solo project this release engineering is genuinely mature — better than the product surface it ships.

**The dirty tree is itself a finding**: the release candidate contains 19 modified files (several security-relevant) and 10 untracked files that CI has never seen. The tag `v0.3.0-20260901` points at HEAD, so the tagged artefact and the working tree have diverged.

---

## Summary

| Dimension | Status |
|---|---|
| Does it build? | ✅ Yes — frontend and backend both |
| Do tests pass? | ❌ **No — CI gate is red on `main`** |
| Is CI meaningful? | ⚠️ Yes, but blocked at step 2 of 7 |
| Is it type-safe? | ✅ Yes, `strict: true` |
| Is dead code caught? | ❌ No — `noUnusedLocals: false` |
| Is dependency risk managed? | ⚠️ Partially — no audit step, one unmaintained dep |
| Is release signed? | ❌ macOS unsigned |

**One-line verdict**: the codebase builds and is type-safe, but **its safety net does not currently exist** — the test suite has not run since a one-variable typo was committed, CI has been red, and the single most valuable correctness asset in the project (the SSRF boundary) has no tests whatsoever.