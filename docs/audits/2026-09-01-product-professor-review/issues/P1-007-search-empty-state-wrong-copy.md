# P1-007 — Search empty and error states show the wrong message

**Priority**: P1
**Category**: UX
**Labels**: `ux`, `copy`, `search`, `error-handling`, `p1`
**Difficulty**: Low (~2h)
**Risk**: Low

---

## Evidence

One branch serves four incompatible situations — `ArticleList.tsx:428-432`:

```jsx
) : visibleItems.length === 0 ? (
  <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
    <CheckCheck size={28} className="text-border-strong" />
    <p className="text-[13px] text-muted">没有更多文章了</p>
  </div>
```

1. **Search finds nothing** → "没有更多文章了" (wrong: nothing was "more"; the user searched)
2. **Search request fails** → same message (`useItemsData.ts:204-207` catch sets `items=[]` and `totalCount=0`) — the user cannot distinguish "no results" from "the request died"
3. **Unread list fully read** → correct use
4. **All-articles list genuinely empty** → correct use

Aggravating factors:

- **Search is silently capped at 50**: `useItemsData.ts:197` hardcodes `params.append('limit', '50')`, and `hasMore` is never set on the search path — results beyond 50 vanish without indication.
- **No retry**: the failed-search path offers no way to re-run the query.
- **Search clears the view and does not restore it**: `App.tsx:208-214` (`handleSearch`) resets `selectedSourceId`, `selectedFolderId`, and `filter`; `handleClearSearch` (`:217-220`) clears the keyword but does not restore the previous scope. A user who searched from inside a source's unread view is returned to the global all-articles view.

## Why it matters

The empty state is the moment a user decides whether the app respects their input. "没有更多文章了" over a failed search reads as a non-sequitur and hides the failure. Combined with the invisible 50-result cap, the user cannot tell whether their query genuinely returned little or whether the app silently truncated or errored.

## Expected behavior

- Search-empty: "没有找到与「{keyword}」相关的文章" + a hint to try fewer/different words
- Search-failed: an error state with a 重试 button
- Reading-done states (unread cleared / list empty): keep the current `CheckCheck` + "没有更多文章了" — it is correct there
- Search results beyond 50 are either paginated or the UI states the truncation ("显示前 50 条结果")

## Acceptance criteria

- [ ] Four states are distinguishable: search-empty, search-error, unread-cleared, list-empty
- [ ] Search-empty includes the query text
- [ ] Search-error offers a retry that re-runs the same query
- [ ] The 50-result cap is either removed (paginate the search path) or surfaced in the UI
- [ ] Clearing a search restores the previous source/folder/filter scope (or the product decision to not restore is documented)
- [ ] No regression in the two correct uses of the current empty state

## Implementation notes

- Track which state produced emptiness: add a `searchError: boolean` (or reuse a status enum) in `useItemsData`, set in the `catch` at `:203-207`, cleared on success.
- For scope restoration: capture the previous `{selectedSourceId, selectedFolderId, filter}` in `handleSearch` before clearing, and restore in `handleClearSearch`. This is ~6 lines and resolves a real annoyance; if the product prefers global-view-after-search, document that choice in the handler's comment so it is not "fixed" again later.
- Pagination on search is more work (the backend `SearchItems` supports `limit`/`offset` — verify, `item_service.go:339-343`); if deferring, surface the cap with a footnote line, which is honest and cheap.

## Verification

Manual: (1) search a nonsense string → specific empty message with the query; (2) stop the backend → search → error + retry; (3) read everything in a source → unchanged correct copy; (4) search a common word → truncation notice if applicable.