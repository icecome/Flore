# P0-006 — No corruption detection and no recovery path

**Priority**: P0
**Category**: Data durability
**Labels**: `data-loss`, `durability`, `sqlite`, `recovery`, `p0`
**Difficulty**: Low for detection (~3h); Medium for recovery UX (~2 days)
**Risk**: Low

---

## Evidence

**No integrity checking exists anywhere:**

```
$ grep -rn 'integrity_check\|quick_check' server/go
(0 hits)
```

- `database.go:52-87` — `Init` opens the file and sets DSN pragmas. No check.
- `backup.go:42-79` — `validateSQLite` checks only the magic header, `PRAGMA schema_version != 0`, and that an `Item` table exists. A page-corrupt database with an intact header passes.
- `backend/backend.go:92-108` — if `AutoMigrate`/`RunMigrations` fail, `Start` returns an error.
- `cmd/main.go:10-13` — the process **exits 1**.
- `apps/desktop/backend.go:75-79` — port file never written → "failed to obtain backend port"; `:298-328` — health check fails, backend marked down.

**What the user sees:** a generic "backend not ready" — or, per P0-002, the first-run onboarding screen. Nothing says the database is damaged.

**What they cannot do:** there is no UI to (a) learn the DB is corrupt, (b) find the `reader.db.bak.<timestamp>` snapshots that `backup_restore.go:42-53` writes before every restore, or (c) trigger a restore — the restore endpoints (`reader.go:261-282`) require the app to be running. The only discoverable fix is deleting `reader.db`, which destroys everything.

**Aggravating**: auto-backup is off by default (P0-005), so for most users there is no backup to recover *from* either.

## Why it matters

This is the highest expected-loss item in the product. The promise is "your reading history lives here, on your machine, and it is yours." There is no runtime invariant enforcing that the data is readable, and no path back if it stops being readable.

Probability is low per run, but the app performs heavy write churn — `VACUUM` after every auto-backup (`scheduler.go:223-227`), `VACUUM INTO` on restore (`backup_restore.go:46,103`), constant upserts — and the failure is **total and silent**.

The detection half costs milliseconds: `PRAGMA quick_check` at startup.

## Expected behavior

1. At startup, after opening the DB: run `PRAGMA quick_check`. If it fails, **do not exit silently** — surface a recovery screen.
2. The recovery screen offers: restore from the most recent backup, restore from a `.bak.<timestamp>` snapshot, or open the data folder.
3. The failure is logged with a distinguishable marker so support can identify it.

## Acceptance criteria

- [ ] `PRAGMA quick_check` runs at startup; measured cost on a realistic DB (<100MB) is recorded in the PR
- [ ] On failure the app starts into a **recovery mode** rather than exiting — the backend must not `exit 1` on a checkable condition
- [ ] Recovery UI lists available backups with timestamps and sizes, and offers a one-click restore
- [ ] If no backup exists, the UI says so explicitly and offers "打开数据文件夹" so the user can act manually or seek support
- [ ] The pre-restore `.bak.<timestamp>` files are visible in the recovery UI (they are the last line of defence and are currently invisible)
- [ ] The recovery path works when the main DB is unreadable (it must not depend on the app's normal startup)
- [ ] `validateSQLite` (`backup.go:42-79`) additionally runs `quick_check` on the **restore source** before replacing the live DB — see P1-020
- [ ] A corrupt-DB startup is covered by an automated test

## Implementation notes

- **`quick_check` over `integrity_check`**: `integrity_check` does full page-by-page verification and is slow on large DBs; `quick_check` skips index-vs-table cross-checks and is the standard startup choice. Run the full check only when the user explicitly asks.
- **Do not exit on failure.** `cmd/main.go:10-13` currently exits; the recovery mode needs the HTTP server up (so the frontend can render recovery UI) even when the DB is unusable. Consider a minimal recovery server or a dedicated frontend route that talks to a degraded backend.
- **The `.bak` files are the safety net that already exists** and that nobody can see. Surfacing them in the recovery UI may be more valuable than the integrity check itself.
- Sequence this with P1-020 (crash-atomic restore) — both concern the same "the DB is the product" invariant, and P1-020 prevents the most likely way a DB becomes corrupt in the first place.
- This is also a prerequisite for making P0-005 (auto-backup on by default) meaningful: backups are only useful if there is a way to restore from them.

## Verification

```bash
# corrupt a copy of the DB (flip bytes in the middle of a page, keep the header)
python -c "
d=bytearray(open('reader.db','rb').read()); d[len(d)//2]^=0xFF; open('reader-bad.db','wb').write(d)"
# point DATABASE_URL at it, start the app
# expect: recovery screen, not a silent exit and not the onboarding screen
```
