# P0-002 — Add a global backend connection state

**Priority**: P0
**Category**: UX / Reliability
**Labels**: `ux`, `startup`, `error-handling`, `p0`
**Difficulty**: Medium (~1 day)
**Risk**: Low

---

## Evidence

The backend can take up to 15s to become healthy — `apps/desktop/backend.go:74-79` polls `/health`, and `api.ts:207-219` polls `GetBackendStatus` 75×200ms. During that window:

- `App.tsx:909` gates the main area on `sources.length === 0` → a returning user with 200 subscriptions sees **`OnboardingWelcome` — "你还没有订阅任何源"** (`OnboardingWelcome.tsx:47`).
- `useSourcesData.ts:39,51,74` and `useItemsData.ts:116,132,148,177` — seven independent catch blocks each fire `showToast('获取…失败')` against `toast.ts:12 MAX_TOASTS = 5`, so the toasts overwrite each other.
- `loadingSources` exists precisely for this window and is **never rendered** — verified: its only occurrence is the destructure at `App.tsx:121`.
- If the backend never comes up, `api.ts:221` **silently falls back** to the default base (`console.warn` only). There is no offline surface: the user sees the welcome screen and per-action toasts indistinguishable from a bad feed URL (`OnboardingWelcome.tsx:31` "订阅失败，请检查链接或稍后重试").
- Mid-session crash: `backend.go:122-142` (`monitorBackend`) logs and clears `goStarted`, but nothing notifies the frontend.

Grep for any existing global state (`offline|离线|连接中|backendDown|unavailable`): **0 relevant hits**.

## Why it matters

This is the worst first impression the product makes, and it hits **every** user on every cold start. It also converts a diagnosable infrastructure failure ("backend didn't start") into an undiagnosable product failure ("my subscriptions are gone"). A user who sees the welcome screen after a crash may re-import OPML and duplicate everything (import has no dedup — separate issue).

## Expected behavior

1. A single `backendStatus: 'connecting' | 'ready' | 'offline'` state, owned at `App` level.
2. While `connecting`: a splash/skeleton — **not** `OnboardingWelcome`, **not** per-request toasts.
3. While `offline`: a persistent banner with a retry action and a link to the log location — never the welcome screen.
4. Per-request error toasts suppressed while status ≠ `ready`.

## Acceptance criteria

- [ ] Cold start with a healthy backend: no welcome flash for existing users; skeleton visible ≤15s max
- [ ] Existing user (sources exist in DB) never sees `OnboardingWelcome` due to loading
- [ ] Backend killed mid-session: banner appears within one failed request, with retry
- [ ] Backend never started: banner with "打开日志文件夹" guidance; **zero** toast flood
- [ ] No more than 1 toast per failure category per session while offline

## Implementation notes

- Derive `connecting → ready` from the first successful `listSources()`; derive `offline` from N consecutive failures of *different* endpoints (a single 404 is not offline).
- `initApiBase()` already knows when polling times out (`api.ts:221`) — return a status instead of only warning.
- Suppress toasts via a module-level flag in `utils/toast.ts` (e.g. `setSuppressed(bool)`) rather than touching 7 call sites.
- Reuse `ErrorBoundary`'s visual language (`ErrorBoundary.tsx:40-68`) for the offline banner so failure states look related.

## Verification

Manual: kill the Go process after startup; confirm the banner and the absence of toast flood. Automated: none possible until frontend tests exist (P3-003 gap) — add a Vitest scaffold with this component as the first test.
