# P1-002 — Unify the three unread-count queries

**Priority**: P1
**Category**: Correctness
**Labels**: `bug`, `correctness`, `duplication`, `p1`
**Difficulty**: Low (~4h)
**Risk**: Medium — changes a user-visible number; needs verification per scope

---

## Evidence

Three code paths compute "unread", and they **already disagree**:

| # | Function | Filters applied |
|---|---|---|
| 1 | `source_service.go:81-99` `computeUnreadCountMap` — powers the **sidebar per-source badge** | `WHERE isRead = false` + `GROUP BY sourceId`. **No join to Source. No `hideInTimeline` filter. No `isPrivate` filter.** |
| 2 | `item_service.go:203-214` `GetUnreadCount` | `Item.isRead = false` **AND** `Source.hideInTimeline = false` (+ `Source.isPrivate = false` when `hidePrivate`) |
| 3 | `item_service.go:217-227` `CountItems` → `applyCountScopeFilters:229` | scope-aware variant of the same rules |

`computeUnreadCountMap` (`:87-91`):

```go
s.getDb().Model(&models.Item{}).
    Select("sourceId AS source_id, COUNT(*) AS count").
    Where("isRead = ?", false).
    Group("sourceId").
    Find(&counts)
```

No `Joins("LEFT JOIN Source ...")`, therefore no way to filter on source-level flags.

## Why it matters

**This is the duplication this codebase has in the worst possible place.** The sidebar badge is the number a user navigates by. When it disagrees with the list behind it, the user stops trusting the whole application — including the parts that are correct.

Concrete consequence: a source with `hideInTimeline = true` (a setting the UI exposes, `SettingsSourcesTab.tsx:241` → `EditSourceModal.tsx:131` → `item_service.go:40`) contributes **zero items** to the article list but **still shows its full unread count** in the sidebar. The user sees "23 unread", clicks, and gets an empty or short list.

This is not a hypothetical: `hideInTimeline` is fully wired on both ends and is the intended way to keep a high-volume feed out of the main timeline while retaining it — exactly the case that produces the mismatch.

Secondary cost: three implementations means the next filter change (or a new source-level flag) must be applied in three places, and the one that gets missed is the one the user looks at most.

## Expected behavior

All three paths derive from **one** query builder. The sidebar badge for a source always equals the unread count shown when that source is selected.

## Acceptance criteria

- [ ] A single shared builder (e.g. `buildUnreadCountQuery(...)`) is the only place these filters are expressed
- [ ] Sidebar badge for source S == unread count in the article list when S is selected, for every combination of `hideInTimeline` ∈ {true,false} and `isPrivate` ∈ {true,false}
- [ ] `hidePrivateInTimeline` setting honoured consistently in all three
- [ ] A regression test asserts the equivalence across all 4 flag combinations
- [ ] `GetSources` (`source_service.go:69`) still uses the cache; cache invalidation behaviour unchanged (9 call sites of `invalidateUnreadCount`)

## Implementation notes

- Add the missing `LEFT JOIN Source ON Item.sourceId = Source.id` to `computeUnreadCountMap` and the two `Where` clauses. **Then** extract the shared builder so the next divergence is impossible.
- Be careful with the join in the aggregate: `GROUP BY sourceId` with a `LEFT JOIN` on a `not null` FK is safe, but confirm no item is orphaned (the FK has `OnDelete:CASCADE`, so it should not be).
- The cache (`unreadCountCache`) stores a `map[int]int64`; no shape change needed.
- **Behaviour-preservation gate**: this changes a displayed number. Decide explicitly whether the *intended* semantic is "all unread items in this source" or "unread items visible in the timeline". The codebase's other two paths say the latter; the sidebar currently does the former. Confirm with the product owner before changing, and state the chosen semantic in the function's doc comment.

## Verification

```bash
cd server/go && go test ./internal/services/ -run Unread -v
```
Manual: create a source with 5 unread items, set `hideInTimeline = true`, confirm sidebar badge and list agree (both 0, or both 5 — but the same).
