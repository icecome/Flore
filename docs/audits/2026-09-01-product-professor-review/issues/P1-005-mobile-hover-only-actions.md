# P1-005 — Mobile: hover-gated list actions are unreachable

**Priority**: P1
**Category**: Mobile UX
**Labels**: `mobile`, `ux`, `accessibility`, `touch`, `p1`
**Difficulty**: Low (~1h)
**Risk**: Low

---

## Evidence

`ArticleList.tsx:733`:

```jsx
<div className="flex items-center gap-0.5 opacity-0 transition-opacity duration-150 group-hover:opacity-100 [&:has(.on)]:opacity-100">
```

Inside it are the two per-row actions — "稍后阅读" (`:734-743`) and "收藏" (`:744-757`). They are `opacity-0` and revealed **only** by `:hover`.

On a touch device there is no hover state. The buttons exist in the DOM but are permanently invisible. There is no `focus-within` variant and no width-based fallback.

Mitigating but insufficient: `[&:has(.on)]:opacity-100` reveals a button once it is *already* toggled — which does not help the user toggle it in the first place. Long-press does fire `contextmenu` in some mobile browsers, but `ReaderToolbar`'s menus are not wired to the row actions on touch, and behaviour is WebView-dependent (**needs confirmation**).

Related: `App.tsx:156-168` already computes `isMobile` (`window.innerWidth < 768`) and passes nothing about it into `ArticleList`.

## Why it matters

Favouriting and saving-for-later from the list is a core reading behaviour — it is how a user triages a feed without opening each article. On mobile it is impossible; the user must open the reader and use the toolbar, one article at a time. Combined with the settings modal overflow (P1-006) and the search-drawer layering (P2-001), mobile reads as a squeezed desktop layout rather than a supported target.

## Expected behavior

On touch/narrow viewports, per-row actions are visible (or reachable by an explicit affordance). On desktop, the current hover reveal is retained — it is a good density choice there.

## Acceptance criteria

- [ ] At `window.innerWidth < 768`, row actions are visible without hover
- [ ] At ≥768px, the hover-reveal behaviour is unchanged
- [ ] Keyboard users can reach the actions via Tab (add `focus-within:opacity-100`) — a desktop accessibility win regardless of mobile
- [ ] Tapping an action does not also trigger row selection/opening (event propagation check)
- [ ] Row height does not jump when actions become always-visible

## Implementation notes

- Simplest correct fix: add `focus-within:opacity-100` **and** make the wrapper `opacity-100` under a mobile media query, e.g. `max-md:opacity-100`. Tailwind's `max-md:` variant keeps it declarative and avoids threading `isMobile` through props.
- If row height is a concern, reserve the space (fixed-width action cluster) rather than hiding it, so the layout does not shift between breakpoints.
- Prefer this over adding a mobile-only overflow menu — the row is already dense and a second menu is worse than two small buttons.

## Verification

Manual: Chrome DevTools device emulation at 375px — both buttons visible and tappable; toggling updates state and the sidebar/list counts. Keyboard: Tab into a row, confirm actions become visible.