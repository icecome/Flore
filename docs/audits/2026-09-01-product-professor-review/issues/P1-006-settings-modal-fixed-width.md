# P1-006 — Settings modal is unusable on mobile

**Priority**: P1
**Category**: Mobile UX
**Labels**: `mobile`, `ux`, `responsive`, `p1`
**Difficulty**: Medium (~1 day)
**Risk**: Medium — 10 tabs must remain navigable

---

## Evidence

- `SettingsModal.tsx:574` — the dialog is a fixed **920px** wide, **740px** tall.
- No responsive variant exists. `isMobile` (`App.tsx:156-168`) is used only for the three-column → stacked layout swap and the sidebar drawer; it is never passed into the settings stack.
- The modal is rendered **inside the sidebar drawer** (`Sidebar.tsx:566-576`), which on mobile is itself 260px wide and off-canvas — compounding the geometry.
- Layout is `nav 200px + content` (per the modal's internal structure), so at 375px the content area is squeezed to near nothing and a 920px dialog overflows the viewport on both sides.

## Why it matters

Settings is where the user fixes things. On a phone it is unreachable: labels and toggles are clipped off-screen, and the tab rail and content compete for a 375px width. Combined with P1-005 (hover-only actions) and P2-001 (search drawer covering results), this is the third distinct way mobile is broken — and it is the one that blocks workarounds for the other two.

Also note: at 45 settings keys across 10 tabs, the modal is already dense on desktop. The mobile layout needs to reduce surface, not just reflow it.

## Expected behavior

On viewports <768px the settings modal occupies the full viewport, the tab rail becomes a horizontally scrollable strip or a full-screen list, and the content area is single-column.

## Acceptance criteria

- [ ] At 375px width the settings modal fits the viewport with no horizontal overflow (`document.documentElement.scrollWidth <= clientWidth`)
- [ ] All 10 tabs are reachable on mobile
- [ ] Every control is tappable (≥44×44px effective target where practical)
- [ ] Closing the modal returns focus correctly (the existing `useFocusTrap` already handles restoration — `ModalLayout.tsx:53-80`)
- [ ] Desktop layout (920px) is unchanged
- [ ] The modal does not render inside the off-canvas sidebar on mobile — either it takes over the viewport, or the drawer closes when settings opens

## Implementation notes

- Change the fixed width to `min(920px, 100vw)` and height to `min(740px, 100dvh)` as a minimum viable fix — but this alone leaves the nav/content squeeze, so treat it as step one, not the fix.
- For the tab rail: at `<768px`, switch to a select dropdown or a scrollable pill row; the current vertical 200px rail cannot coexist with content at 375px.
- Handle the sidebar interaction explicitly: `Sidebar.tsx:277-281` (`handleSettingsClick`) and `:284-288` (`openSyncSettings`) both set `showSettings` — have them also signal the parent to close the drawer, mirroring how `App.tsx:859,867,873` close it for other navigation.
- Reuse the existing `ModalLayout` responsive conventions rather than inventing new ones.

## Verification

Manual: 375×667 emulation — open Settings from the sidebar, visit all 10 tabs, confirm no clipping and no horizontal scrollbar. Verify desktop at 1440px is unchanged.