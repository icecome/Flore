# P1-008 — Empty state advertises an unbound J/K shortcut

**Priority**: P1
**Category**: UX
**Labels**: `ux`, `copy`, `keyboard`, `quick-win`, `p1`
**Difficulty**: Trivial (10 minutes)
**Risk**: None

---

## Evidence

`Reader.tsx:549`:

```jsx
<EmptyState
  icon={<BookOpen size={30} />}
  title="选择一篇文章开始阅读"
  description="从左侧列表挑选一篇文章，或按 J / K 上下浏览。"
/>
```

The keyboard handler is `App.tsx:364-412`. Its `switch (e.key)` handles: `ArrowDown`, `ArrowUp`, `Enter`, `m`/`M`, `s`/`S`, `l`/`L`, `r`/`R`, `x`/`X`, `?`, `Escape`.

**There is no `case 'j'` or `case 'k'` anywhere in the codebase** — verified by grep across `apps/web/src` for `'j'`, `'k'`, `KeyJ`, `KeyK`: **0 matches**.

Meanwhile the two screens that document shortcuts are accurate: `ShortcutsHelpModal.tsx:8-18` and `SettingsShortcutsTab.tsx:6-26` both list ↑/↓, not J/K. So three screens exist and exactly one of them lies.

## Why it matters

This is the **only keyboard teaching a new user encounters** — it appears in the empty state, which is what they see before they have selected anything. A user who tries J and gets nothing learns that the app's text cannot be trusted. It costs less to fix than the reputational damage of leaving it.

It is also the cheapest item in this entire audit: either bind two keys or change a string.

## Expected behavior

Either the keys work, or the copy does not mention them.

## Acceptance criteria (recommended: bind the keys)

- [ ] `j`/`J` moves focus down one article; `k`/`K` moves it up
- [ ] Behaviour matches ↑/↓ exactly, including the wrap-around at list ends (`App.tsx:415-433`)
- [ ] `?` shortcut help and `SettingsShortcutsTab` are updated to list J/K alongside ↑/↓
- [ ] Keys are inert while a modal is open and while focus is in an input (already handled by `App.tsx:448-450`)
- [ ] `Enter` still opens the focused article

## Alternative (copy fix)

Change `Reader.tsx:549` to "从左侧列表挑选一篇文章，或按 ↑ / ↓ 上下浏览。" — one string, zero behaviour change.

## Implementation notes

- Binding is 2 lines: add `case 'j': case 'J': handleArrowDown(e); break;` and the `k` equivalent to the switch in `App.tsx:364-412`.
- J/K is a long-standing convention in RSS readers (greader, Newsboat, Inoreader). Binding it is not scope creep — the app already told users it existed.
- If binding, note the duplication risk: `ShortcutsHelpModal.tsx:8-18` and `SettingsShortcutsTab.tsx:6-26` are two copies of the same table (P3-004). Update both, or better, extract one shared constant and let both render from it — that fixes the duplication and this bug in one change.

## Verification

Manual: press `j` then `k` with the reader empty-state visible and with a list focused; confirm focus moves and the highlight ring follows. Confirm no interference while typing in the search box.
