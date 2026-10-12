# P0-005 — Enable auto-backup by default

**Priority**: P0
**Category**: Data durability
**Labels**: `data-loss`, `defaults`, `local-first`, `p0`
**Difficulty**: Trivial (1 default) + small UX follow-up
**Risk**: Low — disk space is bounded by the existing retention policy

---

## Evidence

- `settings.ts:217` — `backupAutoEnabled: false`
- `scheduler.go:186-188` — `if !sch.service.GetSettingBool("backupAutoEnabled", false) { return }`
- The backup machinery itself is **complete and well-built**: compressed zip with zip-slip/zip-bomb guards (`backup.go:103-110,134,144`), WAL-checkpoint before snapshot (`backup_restore.go:25`), restore-then-validate sequence, cleanup by count and age (`scheduler.go:215-221`).
- Defaults that already exist and are sane: `backupAutoInterval: 1440` (daily), `backupMaxKeep: 10`, `backupMaxDays: 30` (`settings.ts:215-218`).
- Context: the product is distributed and self-described as local-first (`PROJECT.md`); `README.md` offers portable installs. A user's entire reading history lives in one SQLite file with **no other protection**.

## Why it matters

This is the one default that can cost a user everything. The product's entire value proposition is "your data is yours, stored locally." Shipping with the safety mechanism off means the first disk failure, filesystem hiccup, or bad restore permanently destroys a user's library — and the app never once warned them.

The machinery is finished. The scheduler runs. The retention policy bounds disk usage. The only thing between this being safe and unsafe is a boolean default.

## Expected behavior

A fresh install creates a backup on the first scheduler tick and retains up to 10 daily backups (~30 days). Power users who object can turn it off; that is what the toggle is for.

## Acceptance criteria

- [ ] `backupAutoEnabled` defaults to `true`
- [ ] Fresh install: first backup appears within the first scheduler tick after `backupAutoInterval` elapses — or immediately on first launch (see implementation note)
- [ ] Disk usage bounded: `backupMaxKeep` × size; verify with the existing `CleanupBackups` path
- [ ] Users who previously disabled the toggle (if any exist) are **not** re-enabled by the default change — defaults only apply to keys absent from the `Setting` table (verify `loadSettingsFromServer` merge order in `settings.ts:394-401`)
- [ ] The Data tab's toggle reflects the new default

## Implementation notes

- **Immediate first backup is better than waiting a day**: change the scheduler to run `maybeRunBackup` once at startup when no backup exists yet (check `ListBackups()` emptiness), then resume the interval cadence. A user who installs and crashes the same evening still has a snapshot.
- **Existing-user compatibility**: `DEFAULT_SETTINGS` only applies where the `Setting` table lacks the key (`settings.ts:397`), so flipping the default does not override users who explicitly turned it off. Users who never touched the toggle (the vast majority) get protection. This is the desired behaviour, but verify it — it is also the one way this change could surprise someone.
- Surface the new default in the Data tab's copy: "自动备份已默认开启" so the change is discoverable rather than silent.

## Verification

```bash
# fresh DB
rm -rf %LOCALAPPDATA%/Flore   # (dev machine only!)
cd server/go && go run ./cmd/main.go
# wait one tick, or trigger via scheduler start; then:
ls "%LOCALAPPDATA%/Flore/backups"   # should contain a .zip within the first interval
```
