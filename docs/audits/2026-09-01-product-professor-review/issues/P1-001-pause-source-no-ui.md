# P1-001 — Add pause/enable UI for `Source.active` and per-source interval

**Priority**: P1
**Category**: Feature completeness
**Labels**: `feature`, `backend-ready`, `rss`, `p1`
**Difficulty**: Low (~4h) — the backend is already done
**Risk**: Low

---

## Evidence

**Backend — complete and consumed:**
- `models.go:20` — `Active bool \`json:"active" gorm:"default:true"\``
- `coordinator.go:165` — `WHERE active = ? AND remoteOrigin = ''` (skips paused sources during full submit)
- `coordinator.go:225` — same filter for folder submits
- `source_handlers.go:57` — `Active *bool` accepted on update
- `source_service.go:180` — `setIfSet(updates, "active", req.Active)`

**Frontend — absent:**
- `EditSourceModal.tsx` exposes only `isPrivate` and `hideInTimeline` (`:123-131`); grep for `active` against a `Source` instance returns **0 hits**.
- No context-menu entry, no settings-tab control, no keyboard shortcut.

Same story for `Source.Interval`:
- `models.go:19` — `Interval int \`default:120\``, with a working validator (`source_service.go:258-266 normalizeInterval`) and an update path (`:176`)
- The scheduler **ignores it** — `scheduler.go:168-176` uses the global `fetchInterval`/`defaultInterval` setting
- `AddSourceModal.tsx:134` sends `interval: settings.defaultInterval`; `OnboardingWelcome.tsx:24` hardcodes `30`

## Why it matters

**Pausing a feed is table stakes for an RSS reader.** A source that has become noisy, or that is failing repeatedly, or that the user wants to keep for reference without reading — the only recourse today is to **delete it, which cascades to its entire article history** (`models.go:73` `constraint:OnDelete=CASCADE`).

The cost/value asymmetry is stark: the backend work is finished and tested (`source_service_test.go:97` even asserts `Active` survives a partial update), and the missing piece is a checkbox and a number input.

`Source.Interval` is the same debt in a worse form: a per-source refresh frequency is written by the client, stored, validated, and **never read**. Two controls for one concept (`defaultInterval` in settings, `interval` per source), and only one of them works.

## Expected behavior

1. `EditSourceModal` gains an "启用此订阅源" toggle bound to `active`.
2. `EditSourceModal` gains a "抓取间隔" field bound to `interval`, or the field is removed from the create form and `Source.Interval` is dropped from the schema.
3. Paused sources render distinctly in the sidebar (dimmed, with a pause glyph) and are excluded from "全部标为已读" and unread totals.
4. A paused source can be resumed without re-adding it.

## Acceptance criteria

- [ ] Toggling "启用" in `EditSourceModal` persists and is reflected after reload
- [ ] A paused source is skipped by `POST /sources/fetch-all` (verify via `coordinator.go:165`)
- [ ] Paused sources are visually distinguishable in `Sidebar.tsx`
- [ ] Paused sources do not contribute to sidebar unread badges
- [ ] If per-source interval is kept: changing it changes that source's fetch cadence. **If it is not implemented, remove the field from the create form and drop the column** — do not leave a control that does nothing.
- [ ] `go test ./internal/services/` still passes (note `source_service_test.go:97` asserts `Active`/`Interval` on partial update — keep that assertion meaningful)

## Implementation notes

- The decision to make first: **per-source interval — implement or delete?** Implementing means `coordinator.go:165`/`:225` must filter on elapsed time since `lastFetchAt` rather than submitting all active sources each tick; that is a real change to the scheduling model and interacts with the 304-negotiation design documented in `scheduler.go:10-14`. **Deleting is cheaper and honest**: remove the field from `AddSourceModal`/`OnboardingWelcome`, drop the column in a migration, and let the global interval own cadence.
- For pause alone, no scheduler change is needed — the filter already exists.
- Add the paused state to the sidebar's existing health-vs-unread visual language (`Sidebar.tsx:682-690`) rather than inventing a fourth badge.

## Verification

Manual: pause a source, confirm it stops fetching and its badge clears; resume and confirm it fetches. Automated: extend `source_service_test.go` with an `active:false` round-trip.