# P1-009 — Confirm before marking everything read

**Priority**: P1
**Category**: UX
**Labels**: `ux`, `data-safety`, `confirmation`, `p1`
**Difficulty**: Trivial (~1h)
**Risk**: Low

---

## Evidence

`App.tsx:669-677`:

```js
const handleMarkAllRead = useCallback(async (scope: { sourceId?: number; folderId?: number }) => {
  try {
    await markAllRead(scope);
    await Promise.all([...]);
```

No confirmation state, no undo. It is reachable from three surfaces:

- Sidebar filter rows and folder/source context menus (`contextMenu.ts:82,125,154`)
- Article-list header button (`ArticleList.tsx:379-386`) via `handleMarkAllRead` (`:274-277`)

**Every other destructive or bulk action in the app is confirmed**, through the same `setConfirmAction` mechanism:

| Action | Confirmed? | Evidence |
|---|---|---|
| Delete source | ✅ | `App.tsx:649-666` |
| Delete folder | ✅ | `App.tsx:607-623` |
| Clear folder sources | ✅ | `App.tsx:730-745` |
| Delete filter rule | ✅ | `SettingsModal.tsx:472` |
| Delete backup | ✅ | `useBackups.ts` |
| Restore backup | ✅ (multi-step) | `SettingsDataTab.tsx:26-40` |
| **Mark all read** | ❌ | `App.tsx:669` |

## Why it matters

"Mark all read" is **irreversible and destroys information the user cannot reconstruct** — there is no "mark all unread", and the unread state is the primary way a user tracks what they have not read. Mis-clicking it on "全部文章" can mark hundreds of articles read in a scope the user did not intend.

The risk asymmetry is the point: the app asks before deleting *one* source but not before clearing the unread state of *everything*. Deleting a source is more recoverable (the feed can be re-added) than losing the unread boundary.

## Expected behavior

A confirmation naming the affected scope and count, matching the existing `ConfirmDialog` pattern used for deletions.

## Acceptance criteria

- [ ] Clicking any "全部标为已读" entry point opens `ConfirmDialog` first
- [ ] The message names the scope precisely and includes the count: e.g. "确定将「技术」文件夹的 128 篇未读标记为已读？" / "确定将全部 340 篇未读标记为已读？"
- [ ] Cancel is the default focus; `danger` styling is **not** used (it is not destructive of data, just of state) — or is used, but decide deliberately
- [ ] Confirming performs exactly the current behaviour
- [ ] No change to the bulk "mark read" path for explicitly selected IDs (`handleBatchMarkRead`) — the user already expressed intent by selecting

## Implementation notes

- Route through the existing mechanism: `setConfirmAction({ message, onConfirm })` — `App.tsx:608-622` is the template.
- For the count: `globalUnreadCount` is already available in `ArticleList` (`App.tsx:932`); for source/folder scopes, `source.unreadCount` / the folder aggregate are available. The count is what makes the confirmation meaningful — "确定吗？" without a number is noise.
- Consider whether a scope with 0 unread should disable the action rather than confirm it (`ArticleList.tsx:382` already has `canMarkAllRead`).

## Verification

Manual: click "全部标为已读" from (a) the list header, (b) a source context menu, (c) a folder context menu — all three must confirm, and cancelling must change nothing.