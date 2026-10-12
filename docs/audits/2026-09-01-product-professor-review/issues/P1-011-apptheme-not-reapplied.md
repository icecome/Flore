# P1-011 — Server-synced `appTheme` is never re-applied at startup

**Priority**: P1
**Category**: Correctness
**Labels**: `bug`, `settings`, `theme`, `sync`, `p1`
**Difficulty**: Low (~2h)
**Risk**: Low

---

## Evidence

There are **two independent stores** for the theme:

| Store | Written by | Read by |
|---|---|---|
| `localStorage['theme']` | `SettingsAppearanceTab.tsx:48` (`setItem('theme', value)`) | `index.html:28` (pre-paint script) and `SettingsAppearanceTab.tsx:35` |
| `Setting['appTheme']` (server DB) | `updateSetting('appTheme', value)` → `saveSettings` → `PUT /api/settings` (`settings.ts:422`) | `loadSettingsFromServer` (`settings.ts:346-356`) → `App.tsx:80` |

At startup, `index.html:26-34` sets `data-theme` **only** from `localStorage` (or the OS preference). Nothing re-applies the server-loaded `settings.appTheme` to the DOM attribute afterwards — `App.tsx:65-67` only applies `accentColor`.

**Consequences:**
1. On a fresh machine (or after a webview data wipe, or in the browser build with cleared storage), the server's stored dark preference is **ignored** — the app boots light. The theme is therefore **not actually cross-device**, despite being persisted server-side.
2. There is **no `matchMedia('(prefers-color-scheme: dark)')` change listener** anywhere. With `appTheme: 'system'`, flipping the OS theme while the app runs does nothing until restart.
3. `SettingsAppearanceTab.tsx:32-41` reads `localStorage['theme']` in a mount effect, overriding its own `settings.appTheme` initializer (`:28-30`) — so the two stores can disagree and localStorage wins in the UI.

Additional dead code: `settings/ThemeToggle.tsx` is an entire file with **zero importers**.

## Why it matters

The theme is one of the few settings a user notices immediately, and the product stores it server-side specifically so it travels. Today it does not travel. A user with a dark-mode preference who opens Flore on a second machine (or after clearing site data) gets a light interface and reasonably concludes the sync is broken — because it is.

The missing OS-theme listener is the smaller half, but it is the one that makes "跟随系统" feel wrong on a laptop that switches to dark in the evening.

## Expected behavior

Precedence: server `appTheme` (if present) → `localStorage['theme']` → OS preference. Changes to the OS theme apply live while `appTheme === 'system'`.

## Acceptance criteria

- [ ] After `loadSettingsFromServer` resolves, `data-theme` is set from `settings.appTheme` (resolving `'system'` via `matchMedia`)
- [ ] `appTheme: 'system'` + OS theme flip → interface updates **without restart**
- [ ] A single source of truth: either migrate off `localStorage['theme']` entirely, or keep it purely as the pre-paint hint and make the server value authoritative once loaded
- [ ] No flash of wrong theme on startup (the pre-paint script must keep working)
- [ ] `ThemeToggle.tsx` deleted (zero importers)
- [ ] `Reader.tsx`'s `MutationObserver` on `data-theme` (`:328-344`) continues to drive reader-theme variants

## Implementation notes

- Keep `index.html`'s pre-paint script — it prevents a flash and must stay synchronous. It is a **hint**, not the authority.
- Add an effect in `App.tsx` mirroring the accent-color one (`:65-67`):
  ```
  useEffect(() => { applyThemeToDom(settings.appTheme) }, [settings.appTheme])
  ```
  where `applyThemeToDom` resolves `'system'` and sets the attribute. Extract that helper into `utils/settings.ts` so `SettingsAppearanceTab` and `App` share it instead of duplicating the logic (`:52-59` vs the new effect) — this is the third place theme logic would otherwise live.
- For live OS changes: register one `matchMedia` listener at module scope (or in the same effect), guarded so it only acts when `appTheme === 'system'`.
- Decide whether to keep writing `localStorage['theme']`. If the pre-paint hint is kept, it must be kept in sync on every theme change or the flash returns; that is an acceptable, documented duplication. Otherwise remove it and accept a possible flash on a fresh machine only.

## Verification

Manual: (a) set dark, restart → still dark; (b) clear `localStorage`, restart → still dark (this is the failing case today); (c) with 跟随系统, switch the OS theme → interface follows immediately. Automated: a unit test on the resolver helper (`system` + mocked `matchMedia` → expected attribute).