# P0-003 — Pass toast types so errors are distinguishable from successes

**Priority**: P0
**Category**: UX
**Labels**: `ux`, `accessibility`, `error-handling`, `p0`
**Difficulty**: Trivial (mechanical)
**Risk**: Low

---

## Evidence

`utils/toast.ts:69` — `showToast(message: string, type: ToastType = 'info', duration = 3000)`.
`ToastContainer.tsx:6-10` defines three fully-implemented visual variants:

```ts
success: { icon: Check,          accent: 'var(--success)' },
error:   { icon: AlertTriangle,  accent: 'var(--danger)'  },
info:    { icon: Info,           accent: 'var(--primary)' },
```

**All 122 `showToast(` call sites pass only a message** (verified: grep for a second string argument → 0 matches). Every toast renders as `'info'`. The `success` and `error` branches are unreachable code.

Representative pairs that are today visually identical:

| Success | Failure |
|---|---|
| `OnboardingWelcome.tsx:27` "已订阅「…」" | `OnboardingWelcome.tsx:31` "订阅失败，请检查链接或稍后重试" |
| `App.tsx:306` (star toggled) | `App.tsx:303` "收藏失败" |
| `useBackups.ts` "已备份" | `SettingsDataTab.tsx:96` "清理失败" |
| `SettingsSyncTab.tsx:229` "同步完成…" | `SettingsSyncTab.tsx:237` (sync error message) |

## Why it matters

This structurally weakens **every** error message in the application. A user scanning the bottom-right corner cannot tell whether the thing they just did worked. For a local-first app whose main anxiety is "did my data save", that is a trust problem, not a cosmetic one. It is also an accessibility problem: the toast container is `aria-live="polite"` (`ToastContainer.tsx:28`), so a screen-reader user hears the text but gets no severity cue.

The fix is unusually cheap because the rendering layer is already complete and correct.

## Expected behavior

Errors render with the danger accent + `AlertTriangle`; confirmations render with the success accent + `Check`; informational messages keep `'info'`.

## Acceptance criteria

- [ ] Every `catch` block's toast passes `'error'`
- [ ] Every user-initiated destructive or state-changing success toast passes `'success'`
- [ ] Informational/progress toasts (e.g. "正在生成图片…") keep `'info'`
- [ ] `grep -c "showToast([^)]*," apps/web/src` returns a count > 0 (i.e. the parameter is actually used)
- [ ] No toast in the app renders `AlertTriangle` for a success or `Check` for a failure

## Implementation notes

- Mechanical sweep across `App.tsx`, `SettingsModal.tsx`, `settings/*.tsx`, `hooks/*.ts`, `Reader.tsx`, `ReaderToolbar.tsx`, `OnboardingWelcome.tsx`, `Sidebar.tsx`, `ContextMenu.tsx`.
- **Do not** add a `type` to the ~10 toasts that are genuinely neutral ("已提交重试，稍后自动刷新状态").
- Optional but recommended: give error toasts a longer default duration (e.g. 6000ms) — a 3s window is short for reading an error you did not expect.
- Consider a small helper `showError(message)` / `showSuccess(message)` to make the intent explicit and to allow a lint-style check later.

## Verification

Manual: trigger one failure per major flow (delete a source while the backend is down; import a malformed OPML; star an item) and confirm red + triangle. Automated: snapshot test of `ToastContainer` for each type once a Vitest scaffold exists.
