# P1-014 — CORS trusts every loopback origin while reads are token-free

**Priority**: P1
**Category**: Security
**Labels**: `security`, `cors`, `auth`, `privacy`, `p1`
**Difficulty**: Medium (~1 day, includes frontend coordination)
**Risk**: Medium — token-free reads are the web build's default mode

---

## Evidence

**CORS accepts every loopback origin, on any port:**
- `security.go:16-29` — `IsLocalOrigin` matches hostnames `127.0.0.1` / `localhost` / `wails.localhost`, **ignoring scheme and port**, deliberately so it covers `wails://`, `http://wails.localhost`, and dev ports.
- `backend.go:253-262` — `AllowOriginFunc = handlers.IsLocalOrigin`.
- `backend.go:248` — preflight allows `Authorization` and `X-Requested-With`.

**All reads are outside the `sensitive` group** (so token-free): `GET /api/sources` (`reader.go:186`), `GET /api/items` (`:210`), `GET /api/items/search` (`:222`), `GET /api/database/info` (`:265`), `GET /api/diagnostic/items` (`:297`).

## The attack

The user visits any page served from `http://127.0.0.1:<anyport>` or `http://localhost:<anyport>` — another application's admin UI, a dev server, a compromised or malicious local service. Its JavaScript runs:

```js
const port = 3002  // or discovered — see P3, port file is 0644 in %TEMP%
const items = await fetch(`http://127.0.0.1:${port}/api/items?limit=200`).then(r => r.json())
```

- CORS preflight succeeds: loopback origin, headers allow-listed.
- GET needs no CSRF header (`security.go:43-46` skips read methods).
- No token required — reads are not gated.
- The response is CORS-readable, so the page can page through the entire history, query the FTS index via `/items/search?q=`, and read the DB path (username disclosure) via `/database/info`.

**The Bearer token cannot prevent this**: it never gates reads, so it cannot distinguish "the app's own webview" from "any other loopback page". The dynamic port is not a barrier either (`backend.go:177-181` writes it `0644` with a predictable name to `%TEMP%`).

**What is correctly blocked**: DNS rebinding from the public internet — `security.go:24-27` is an origin *string* check, so a non-loopback hostname is rejected regardless of resolved IP. The hole is strictly **other-local-page → API**, which is a realistic desktop scenario (dev servers, local admin panels, LAN apps bound to loopback).

## Why it matters

Silent bulk exfiltration of the entire reading history and full subscription list by any page served from loopback — even when the user has done everything right and the token is set. For a product whose stated positioning is privacy and data autonomy, this is the finding most likely to be quoted back in a public disclosure.

## Expected behavior

Reads require the token. The app's own webview always presents it (`api.ts:194-200` fetches it from the desktop shell; `fetchData.ts:34` attaches it). The proxy family keeps its narrow `<img>`-consumable carve-out.

## Acceptance criteria

- [ ] All read endpoints that return user data (`/sources`, `/items`, `/items/search`, `/items/count`, `/folders`, `/backups*`, `/settings`, `/sync/config`, `/database/info`, `/diagnostic/items`) require the token when one is set
- [ ] The web build (`npm run dev:web`, no token) behaves by explicit decision, not by accident — document it and decide whether it stays read-only-unauthenticated or becomes localhost-only by default
- [ ] `IsLocalOrigin` is tightened to the specific ports/origins the app uses rather than "any port on loopback", **or** the rationale for accepting all loopback ports is documented with the compensating control (token on reads) named
- [ ] The port file is written `0600` (currently `0644`, `backend.go:177-181`) and the product stops treating port obscurity as a control
- [ ] The frontend presents the token on every request, including retries and `AbortController` paths (`fetchData.ts:31-36` already does this — verify no path bypasses `fetchData`)
- [ ] No regression in the favicon/image/css proxy family, which must stay header-free-consumable

## Implementation notes

- The cleanest rule, stated once and applied to 81 routes: **"returns user data ⇒ `sensitive`; only `<img>`-consumable proxies stay open."** Write it at `RegisterRoutes` (`reader.go:173`) so the next route is added by principle.
- **Coordinate with P1-013** — they are one design decision expressed twice. Splitting them risks shipping the inconsistent half.
- **Frontend consequence to plan for**: the web build currently works with no token because reads are open. If reads start requiring a token, the web build needs either (a) a documented localhost-only deployment story, or (b) a token entry in the UI. This is a product decision, not a code one — surface it to the owner rather than defaulting.
- Do **not** "fix" this by adding a custom header to reads: a cross-origin page could then simply send it. The token is the control.

## Verification

Manual: start a throwaway HTTP server on `127.0.0.1:9999` serving a page that fetches `http://127.0.0.1:<flore>/api/items?limit=200`. With the token set, the request must fail (401). With no token and the new policy applied, it must fail by design — not by CORS accident.