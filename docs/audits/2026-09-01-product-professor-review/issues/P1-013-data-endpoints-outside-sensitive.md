# P1-013 — Data-shaped endpoints sit outside the `sensitive` group

**Priority**: P1
**Category**: Security
**Labels**: `security`, `auth`, `api`, `p1`
**Difficulty**: Low (~2h)
**Risk**: Medium — changes which callers reach which routes; needs a frontend audit after

---

## Evidence

The auth gate is `sensitive := api` with an optional Bearer layer (`handlers/reader.go:179-183`); `AuthMiddleware` returns `nil` when `FLORE_API_TOKEN` is empty (`:156-159`). Membership was decided **route by route**, and data-bearing operations landed on both sides.

**Outside `sensitive` (verified placement):**

| Route | Line | Exposes / does |
|---|---|---|
| `POST /api/items/export` | `:226` | **The entire reading history** — titles, links, full descriptions, authors, read/star state — no token |
| `GET /api/opml/export` | `:207` | All subscriptions |
| `POST /api/articles/cleanup` | `:294` | Mass-deletes read articles |
| `POST /api/database/vacuum` | `:269` | Write operation that locks the DB |
| `POST /api/items/read-all`, `/api/items/mark-read` | `:220-221` | Bulk state mutation |
| `POST /api/sources/create` | `:195` | Data write |
| `GET /api/database/info` | `:265` | Absolute DB path, backup dir, journal mode |
| `GET /api/cache/stats` | `:266` | — |

**The codebase contradicts its own rule**: `:271-272` protects DB export/restore/backups with the comment "备份含全量数据库…防护口径一致". `POST /items/export` **is** a full-database export in JSON form.

**Why CSRF does not save this**: `security.go:48-53` passes requests **without an Origin header** by design (curl, the desktop shell's Go client). CSRF is a browser-only barrier; a non-browser local caller reaches all of the above regardless of `FLORE_API_TOKEN`.

**Graded impact (desktop uplift model)**: low locally — a same-user process could read `reader.db` directly. The token (`app.go:77-96`, fail-closed) exists precisely to deny same-machine processes this API surface, and these routes void that guarantee.

**Real impact — non-loopback deployment**: `.env.example:7-8` documents `BIND_ADDR=0.0.0.0`. In that mode a remote unauthenticated attacker exfiltrates the full history and destroys read data.

## Expected behavior

Every endpoint that returns or mutates user data requires the token, with a narrow carve-out only for `<img>`-consumable proxy endpoints that cannot send headers.

## Acceptance criteria

- [ ] `POST /items/export`, `GET /opml/export`, `POST /articles/cleanup`, `POST /database/vacuum`, `POST /items/read-all`, `POST /items/mark-read`, `POST /sources/create`, `GET /database/info`, `GET /cache/stats` moved to the `sensitive` group
- [ ] The browser build still works end to end (the token is presented via `fetchData.ts:34` when present; on web-without-token the app degrades deliberately — see P1-014 for that decision)
- [ ] Every route in `reader.go` has been re-audited against a written rule rather than remembered placement, and the rule is documented at `RegisterRoutes`
- [ ] The desktop shell's own non-webview callers (`notify.go:38`, `systray.go:174`, `window.go:31`, `backend.go:208`) are updated to present the token — `notify.go` and `systray.go` currently use raw `http.Get`/POST with no `Authorization` header and **will break** when their routes move

## Implementation notes

- **`notify.go:38` and `systray.go:174` are the trap.** They hit `/sources/fetch-status` and `/sources/fetch-all` with plain `net/http` and no auth header. Those two routes may legitimately stay in the open group (they are not data-bearing), but verify each before deciding; if they move, the shell must attach the token it already has (`app.go:99-101`).
- Write the rule down in `reader.go` above `RegisterRoutes` so the next route is added by principle, not by precedent: *"everything that returns user data → `sensitive`; only `<img>`-consumable proxies stay open."*
- The favicon/image/css proxy family legitimately stays open — `setProxyCORS` (`security.go:71-77`) already models that carve-out correctly.
- Pair this with P1-014 (gate reads as well as writes) — they are the same fix applied twice, and doing them separately risks leaving the inconsistent half in place.

## Verification

```bash
# with FLORE_API_TOKEN set:
curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:PORT/api/items/export -d '{"format":"json"}'
# must be 401
```
Manual: with a token set, confirm no unauthenticated caller can export history or mutate data.