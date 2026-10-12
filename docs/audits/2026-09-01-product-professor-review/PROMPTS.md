# Prompts to run next

Generated from the 2026-09-01 Product Professor full audit of Flore (`v0.3.0-20260901`, grade **C−**).

---

## Recommended sequence

The audit found that P0-001 (a single unused variable) gates all backend work. Fix that first — it is one character and it restores the safety net that makes every subsequent change verifiable.

```bash
# Step 0 — restore the safety net (1 character, do this alone)
/make-it-better p0-001

# Step 1 — the P0 set (~2 days total)
/make-it-better p0-p1

# Step 2 — re-score and see what moved
/score-product after
```

---

## Fix P0 issues

```
/make-it-better p0
```

Individual, in recommended order:

| Prompt | Issue | Why this order |
|---|---|---|
| `/make-it-better P0-001-fix-services-test-build` | Untracked test build | 1 character; gates everything else |
| `/make-it-better P0-005-auto-backup-off-by-default` | Auto-backup off by default | Smallest change with the largest data-loss reduction |
| `/make-it-better P0-006-no-corruption-detection` | No corruption detection | Detection is ~3h; the recovery UI is the larger half |
| `/make-it-better P0-003-toast-types-unreachable` | Toast types | Mechanical sweep; restores meaning to every error in the app |
| `/make-it-better P0-002-no-backend-connection-state` | Backend connection state | The worst first impression; ~1 day |
| `/make-it-better P0-004-urlpolicy-untested` | SSRF boundary tests | Do this **before** any refactor touching `urlpolicy.go` or the fetch path |

**Note on P0-005 + P0-006**: they are a pair. Turning backups on is only meaningful once there is a way to restore from them, and the recovery UI needs a backup to point at. Ship them together.

---

## Fix P1 issues

```
/make-it-better p1
```

### Group A — stop the app from lying to users (~1 day)

```
Fix the declare-without-implement gaps in Flore:
- P1-003: trayEnabled — wire it in apps/desktop (note the coupling trap in the
  issue: disabling the tray while minimizeBehavior/closeBehavior is 'tray'
  would leave the user unable to restore the window), or delete the toggle
- P1-007: search empty/error states — four distinct states, query text in the
  empty message, retry on error, and surface or remove the silent 50-result cap
- P1-008: bind j/k navigation — the empty state at Reader.tsx:549 already
  advertises them; also extract ONE shared shortcut table so
  ShortcutsHelpModal and SettingsShortcutsTab stop diverging
- P1-010: faviconMode — pass it as a prop instead of the useState snapshot
- P1-011: appTheme — apply the server value to the DOM after
  loadSettingsFromServer, add a matchMedia listener for 'system', delete the
  dead ThemeToggle.tsx
```

### Group B — close declare-without-implement gaps (~1 day)

```
Close the backend-ready-but-unreachable features in Flore:
- P1-001: add pause/enable + per-source interval UI. IMPORTANT: decide first
  whether per-source interval is implemented (requires coordinator.go to
  filter on elapsed time since lastFetchAt) or deleted (remove the field from
  AddSourceModal/OnboardingWelcome and drop the column). Do not leave a control
  that does nothing.
- P1-004: remove the S3 sync option and its 6 credential fields, delete the
  zero-implementation FileProvider interface and ReaderPackage types from
  internal/sync/provider.go, and stop leaking English errors like
  'file provider "s3" not supported yet' into the Chinese UI
```

### Group C — make mobile usable (~2 days)

```
Make Flore usable on a 375px viewport:
- P1-005: row-level 收藏/稍后阅读 actions are opacity-0 + group-hover only —
  add focus-within and a max-md: opacity-100 fallback, keeping desktop unchanged
- P1-006: SettingsModal is a fixed 920x740 with no responsive variant; use
  min(920px,100vw), switch the 200px tab rail to a scrollable strip under 768px,
  and stop rendering it inside the off-canvas sidebar
- Also from AUDIT_REPORT §7 P2-001/P2-002: close the mobile drawer when a
  search is submitted, and stop force-resetting mobileView on every non-mobile
  resize (App.tsx:160-163) so rotation does not discard the article being read
```

### Group D — correctness and distribution

```
/make-it-better P1-002-unread-count-divergence
/make-it-better P1-009-mark-all-read-no-confirm
/make-it-better P1-012-unsigned-installers
```

### Group E — security: close the auth boundary (~1 day, one design decision)

```
P1-013 and P1-014 are ONE design decision applied twice. The rule:

    Everything that returns user data requires the token.
    Only <img>-consumable proxy endpoints stay open.

- P1-013: move /items/export (full history), /opml/export, /articles/cleanup,
  /database/vacuum, /items/read-all, /items/mark-read, /sources/create,
  /database/info, /cache/stats into the `sensitive` group. The codebase's own
  comment at reader.go:271-272 already states this rule ("备份含全量数据库…防护口径一致").
  TRAP: apps/desktop/notify.go:38 and systray.go:174 use raw net/http with no
  auth header — verify their routes before moving anything.
- P1-014: gate reads as well as writes. This needs a PRODUCT DECISION about the
  web build (currently works token-free): document it as localhost-only, or add
  a token UI. Also: write the port file 0600 instead of 0644, and stop treating
  port obscurity as a control.
```

Note for P1-012: code signing needs paid certificates. Do the `SHA256SUMS` +
install-instructions part now; treat certificate procurement as a decision to
make explicitly rather than a task to defer silently.

---

## Fix P2 issues

```
/make-it-better p2
```

Grouped from `AUDIT_REPORT.md` §7 — mobile drawer/orientation (P2-001/002),
progress feedback for long operations (P2-003), add-source feedback parity with
OPML import (P2-004), the silent validation dead-end in `AddSourceModal`
(P2-005), batching per-item mark-read POSTs in scroll mode (P2-006),
accessibility: `htmlFor` on form fields, accessible names on ~30 toggles, and
un-nesting the `<button>` inside `<button>` at `Sidebar.tsx:732-749`
(P2-007/008), and OPML import dedup (P2-009).

---

## Re-score after fixes

```
/product-professor quick
```

A quick pass is sufficient to re-score; the full audit is not needed until the
P0/P1 set is closed. Expected movement if the P0 set ships: Reliability
35 → ~60, UX 45 → ~58, Launch Readiness 38 → ~62, overall ~62 → ~71 (C+).

---

## Generate GitHub issues

```
/github-issues from latest audit
```

Or, for a specific priority only:

```
/github-issues from latest audit --priority P0
```

The 17 issue files in `issues/` are already GitHub-ready: title, priority,
category, evidence, why it matters, expected behaviour, acceptance criteria,
implementation notes, difficulty, risk, and labels.

---

## Check launch readiness

```
/ship-it-check full
```

Re-run this **after** the P0 set. The blockers in `AUDIT_REPORT.md` §5 must all
be closed. Expect **NO** until P0-001 and P0-005 are done — and note that
macOS code signing (P1-012) may remain open by decision rather than by neglect.

```
Also verify the two structural security fixes landed together:
- P1-013 + P1-014 (auth boundary) — a half-applied boundary is worse than none
- P0-006 + P1-020 (durability invariants) — restore must be crash-atomic and
  corruption must be detected, or the recovery UI is cosmetic
```

---

## Additional targeted audits

```
# Deeper security pass — the current review was code-reading, not exploitation
/product-professor security

# Deeper architecture pass — the god-object and App.tsx decomposition need a
# design, not a bug fix
/product-professor architecture

# Visual polish from rendered screenshots — this audit judged the design system
# from CSS tokens and markup only, which is why Visual polish carries a Medium
# confidence rating
/product-professor screenshots

# UX-only pass with personas, once the connection state and toast fixes land
/product-professor user-pov
```

---

## Follow-up work not covered by this audit

```
# The repo already contains a razor review at
# docs/planning/hai-razor-frontend-review.md that recommends cutting the
# diagnostic subsystem, wiring trayEnabled, and deleting Source.Interval.
# None of it was executed. Decide whether that document should be closed or
# turned into tracked issues.
# See AUDIT_REPORT.md §19 question 1.

# Enable noUnusedLocals / noUnusedParameters in apps/web/tsconfig.json — this
# would automatically surface the dead file, 28 dead icon aliases, and 3 unused
# destructures that this audit found by hand.
```

---

## Track progress

```
product-professor record-score --auto "Baseline: initial full audit 2026-09-01"
product-professor compare
```