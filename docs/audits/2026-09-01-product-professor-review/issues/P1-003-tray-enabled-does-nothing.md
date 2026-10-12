# P1-003 — `trayEnabled` does nothing: wire it or delete it

**Priority**: P1
**Category**: Correctness
**Labels**: `bug`, `settings`, `desktop`, `p1`
**Difficulty**: Low (~2h)
**Risk**: Low

---

## Evidence

The UI offers the toggle and persists it:
- `SettingsNotifyTab.tsx:50-51` — `<Toggle checked={settings.trayEnabled} onChange={(v) => updateSetting('trayEnabled', v)} />`
- `settings.ts:153` declares it; `:229` defaults it to `false`
- It is written to the server `Setting` table via `saveSettings` → `PUT /api/settings`

Nothing reads it:
- `grep trayEnabled apps/desktop/*.go` → **0 hits**
- `apps/desktop/app.go:255` — `a.startTray()` called **unconditionally** in `startup`
- `apps/desktop/window.go:69-70` reads only `minimizeBehavior` and `closeBehavior`
- `server/go` — 0 hits

**Result**: a user turns "显示托盘图标" off, restarts, and the tray icon is still there.

## Why it matters

Beyond the bug itself, this is the clearest instance of the pattern that defines this product's problems: **a visible control with no effect**. A user who discovers one placebo toggle has no way to know how many of the other 44 settings are real.

Note the counter-evidence that this is an oversight, not a constraint: `Source.hideInTimeline` **is** fully wired end-to-end (`SettingsSourcesTab.tsx:241` → `EditSourceModal.tsx:131` → `item_service.go:40,208`), and `closeBehavior`/`minimizeBehavior` are read by the shell (`window.go:69-70`). Cross-stack wiring was demonstrably achievable here.

## Expected behavior

Either (a) the toggle controls whether the tray is created, or (b) the toggle does not exist.

## Acceptance criteria (option A — recommended)

- [ ] Shell reads `trayEnabled` at startup (same mechanism as `window.go:31` already uses for `GET /api/settings/:key`)
- [ ] `trayEnabled = false` → no tray icon created; window close behaviour must then **not** silently hide the app (see note)
- [ ] Changing the value takes effect on next launch, and the UI says so ("重启后生效")
- [ ] Default reconsidered: `false` is currently the default, which — if wired naively — would remove the tray from every existing install's next launch

## Acceptance criteria (option B — delete)

- [ ] `trayEnabled` removed from `AppSettings`, `DEFAULT_SETTINGS`, and `SettingsNotifyTab`
- [ ] Existing `Setting` rows for the key are ignored (harmless) or cleaned

## Implementation notes

**Option A has a trap.** The tray is the only way to restore a window that was minimised-to-tray. If `trayEnabled = false` disables the tray while `minimizeBehavior = 'tray'` or `closeBehavior = 'tray'`, the user can hide the app with no way back except relaunching. So wiring it properly requires coupling: **tray disabled ⇒ force taskbar behaviour**, or refuse the combination in the UI with an explanation.

Given that coupling cost, option B (delete) is defensible and honest — but the tray is genuinely useful for a desktop reader, so option A is preferred if the coupling is handled.

Read the setting through the existing shell path (`window.go:31` already does `GET /api/settings/:key`) rather than adding a new binding.

## Verification

Manual: toggle off → restart → no tray icon, and closing the window quits rather than hiding. Toggle on → restart → tray present.
