# P1-010 — `faviconMode` changes require a reload

**Priority**: P1
**Category**: Correctness
**Labels**: `bug`, `settings`, `react`, `p1`
**Difficulty**: Trivial (~30 min)
**Risk**: Low

---

## Evidence

`SourceAvatar.tsx:27-30`:

```js
const [faviconMode] = useState(() => {
  const s = getCachedSettings();
  return s.faviconMode ?? 'off';
});
```

A `useState` **initializer** — read once at mount, then frozen. There is no subscription to settings changes and no prop carrying the value.

Consumed at `:40-43` to decide between the Yandex proxy (`favicon-proxy`), the direct source-site fetch (`favicon-direct`), or the letter-avatar fallback.

The setting is editable in `SettingsNetworkTab.tsx:77-84`.

**Consequence**: a user switches "站点图标" from 关闭 → 直接抓取, and **nothing happens** until the component remounts (switching sources) or the app reloads. The setting appears broken.

Note this is the *opposite* failure mode from `trayEnabled` (P1-003): there, nothing reads the setting at all; here, the setting is read but cached past its useful life.

## Why it matters

`faviconMode` is the user's lever over whether the app contacts a third party (Yandex) or the source site at all — it is a **privacy control** (`SettingsNetworkTab.tsx:77-84`). A privacy control that appears not to apply is worse than one that does not exist, because the user believes their choice took effect.

## Expected behavior

Changing the setting updates every visible avatar immediately.

## Acceptance criteria

- [ ] Changing `faviconMode` in Settings updates avatars without a reload
- [ ] The three modes behave correctly: `off` → letter avatar only; `direct` → `/favicon-direct`; `yandex` → `/favicon-proxy`
- [ ] No per-avatar network request storm on change (avatars share the setting, not independent fetches)
- [ ] `getCachedSettings` remains correct for other consumers

## Implementation notes

- The cleanest fix: pass `faviconMode` down as a prop. `SourceAvatar` is used in `Sidebar.tsx:672` and `ArticleList.tsx:697`; both trees already receive `settings` (`Sidebar.tsx:37`, `ArticleList.tsx:117`), so the value is available without new plumbing.
- Alternative: subscribe to settings changes. `getCachedSettings` is a module-level cache refreshed by `saveSettings` and a `storage` event (`settings.ts:330-342`) — but **`saveSettings` updating a module variable does not re-render React components**. If you keep the cache approach, add a subscribe mechanism; otherwise you have reinvented the bug.
- Prefer the prop. It is explicit, testable, and removes a hidden global read from a display component.

## Verification

Manual: with articles visible, switch 站点图标 between all three modes — avatars must change immediately in both the sidebar and the list. Automated: a component test asserting each mode selects the right URL prefix once a Vitest scaffold exists.