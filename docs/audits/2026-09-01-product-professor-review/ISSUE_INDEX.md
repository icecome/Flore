# ISSUE_INDEX

**Audit**: 2026-09-01 · Flore `v0.3.0-20260901` · mode `full` · grade **C−**

20 issues. **6 × P0**, **14 × P1**. Every finding carries `file:line` evidence that was verified during the evidence gate — several of my own initial findings were corrected by that gate and are recorded in `AUDIT_REPORT.md` §19.

---

## P0 — do these before the next release

| ID | Title | Category | Effort | Issue file |
|---|---|---|---|---|
| P0-001 | Fix the untracked test build before it turns CI red | Reliability / CI | **1 char** | [link](issues/P0-001-fix-services-test-build.md) |
| P0-002 | Add a global backend connection state | UX | ~1 day | [link](issues/P0-002-no-backend-connection-state.md) |
| P0-003 | Pass toast types so errors are distinguishable | UX | mechanical | [link](issues/P0-003-toast-types-unreachable.md) |
| P0-004 | Add tests for the SSRF boundary (`urlpolicy.go`) | Security | ~4h | [link](issues/P0-004-urlpolicy-untested.md) |
| P0-005 | Enable auto-backup by default | Data durability | ~1h | [link](issues/P0-005-auto-backup-off-by-default.md) |
| P0-006 | No corruption detection and no recovery path | Data durability | ~3h + 2d | [link](issues/P0-006-no-corruption-detection.md) |

**Total P0 effort: roughly 3 days.** P0-001 and P0-005 are each a handful of lines; the two largest (P0-002, P0-006) are the ones that fix the worst first impressions.

## P1 — should ship soon

| ID | Title | Category | Effort | Issue file |
|---|---|---|---|---|
| P1-001 | Add pause/enable UI for `Source.active` + per-source interval | Feature completeness | ~4h | [link](issues/P1-001-pause-source-no-ui.md) |
| P1-002 | Unify the three unread-count queries | Correctness | ~4h | [link](issues/P1-002-unread-count-divergence.md) |
| P1-003 | `trayEnabled` does nothing — wire it or delete it | Correctness | ~2h | [link](issues/P1-003-tray-enabled-does-nothing.md) |
| P1-004 | S3 sync option is unimplemented but collects credentials | Security / Trust | ~1h to remove | [link](issues/P1-004-s3-option-collects-secrets.md) |
| P1-005 | Mobile: hover-gated list actions are unreachable | Mobile UX | ~1h | [link](issues/P1-005-mobile-hover-only-actions.md) |
| P1-006 | Settings modal is unusable on mobile | Mobile UX | ~1 day | [link](issues/P1-006-settings-modal-fixed-width.md) |
| P1-007 | Search empty and error states show the wrong message | UX | ~2h | [link](issues/P1-007-search-empty-state-wrong-copy.md) |
| P1-008 | Empty state advertises an unbound J/K shortcut | UX | **10 min** | [link](issues/P1-008-jk-advertised-not-bound.md) |
| P1-009 | Confirm before marking everything read | UX | ~1h | [link](issues/P1-009-mark-all-read-no-confirm.md) |
| P1-010 | `faviconMode` changes require a reload | Correctness | ~30 min | [link](issues/P1-010-favicon-mode-frozen.md) |
| P1-011 | Server-synced `appTheme` is never re-applied at startup | Correctness | ~2h | [link](issues/P1-011-apptheme-not-reapplied.md) |
| P1-012 | Windows SmartScreen and macOS Gatekeeper block every install | Distribution | external cost | [link](issues/P1-012-unsigned-installers.md) |
| P1-013 | Data-shaped endpoints sit outside the `sensitive` group | Security | ~2h | [link](issues/P1-013-data-endpoints-outside-sensitive.md) |
| P1-014 | CORS trusts every loopback origin while reads are token-free | Security | ~1 day | [link](issues/P1-014-loopback-cors-token-free-reads.md) |

---

## Fastest wins (do these first for maximum visible improvement per minute)

| Issue | Why |
|---|---|
| **P1-008** | 10 minutes. Fixes the app actively teaching users a shortcut that does not exist. |
| **P0-001** | 1 character. Restores the safety net that makes every other fix verifiable. |
| **P1-010** | 30 minutes. A privacy setting that appears not to apply. |
| **P1-009** | 1 hour. Protects irreversible state destruction. |
| **P0-003** | Mechanical. Restores the meaning of every error message in the app. |

## Grouped themes

**Stop the app from lying to users** — P0-002 (shows onboarding to existing users), P0-003 (errors look like successes), P1-003 (`trayEnabled`), P1-007 (wrong empty-state copy), P1-008 (unbound shortcut), P1-010 (`faviconMode`), P1-011 (theme not applied)

**Close declare-without-implement gaps** — P1-001 (`active`/`Interval` built, no UI), P1-004 (S3 form, no backend), P0-005 (backup machinery, off by default)

**Protect the data — runtime, not just backup** — P0-005 (auto-backup), P0-004 (untested SSRF boundary), P1-002 (a number that lies), **P1-013/P1-014 (the auth boundary has holes in both directions)**

**Make mobile usable** — P1-005, P1-006 (+ P2-001 drawer, P2-002 orientation, recorded in the report)

**Make it installable** — P1-012

---

## P2 / P3 (recorded in the report, no issue files)

Not every finding deserves its own file. These are tracked in `AUDIT_REPORT.md` §7 and should be triaged into issues when the P0/P1 set is closed:

**P2** — mobile search drawer stays open over results (P2-001); orientation change discards the article being read (P2-002); no progress for export/sync/backup (P2-003); add-source vs OPML feedback asymmetry (P2-004); silent validation dead-end in `AddSourceModal` (P2-005); per-item POSTs in scroll mark-read (P2-006); nested `<button>` in `Sidebar.tsx:732-749` (P2-007); ~30 unnamed toggles and zero `htmlFor` (P2-008); OPML import has no dedup (P2-009)

**P3** — `icons.tsx:2` "Typestream" header (P3-001); ~6 ticket-ID schemes with 2 dangling (P3-002); dead file + 28 dead aliases + unused hook returns (P3-003); 16 zero-caller routes + 2 duplicates (P3-004); 3 dead fields, no-op migration v3, 45 flat settings keys with 16 tautological descs (P3-005); `readDesensitizedLog` does not desensitise (P3-006); five product identities in one tree (P3-007)

Security-only items recorded in [`DATABASE_SECURITY.md`](DATABASE_SECURITY.md) §F: corruption detection (SEC-03), crash-atomic restore (SEC-04), diagnostic leak + redaction (SEC-05), plaintext credentials in backup zips (SEC-06), Yandex + update-check disclosure (SEC-08), port file `0644` (SEC-10).

---

## Ordering constraint

**P0-001 gates the rest.** With the `internal/services` package not compiling in the working tree, behaviour-preservation cannot be verified for any backend change — which is exactly the condition under which a simplification becomes a regression. Fix it, confirm `go test ./...` is green, and only then begin work on P1-002 or anything that touches the schema.

**P1-013 and P1-014 are one design decision, not two.** Both follow from the same rule — *"returns user data ⇒ token required; only `<img>`-consumable proxies stay open"* — applied to writes and reads respectively. Ship them together or not at all; a half-applied boundary is worse than the current one because it looks fixed.