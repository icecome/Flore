# P1-004 — S3 sync option is unimplemented but collects credentials

**Priority**: P1
**Category**: Security / Trust
**Labels**: `security`, `privacy`, `dead-code`, `ui`, `p1`
**Difficulty**: Low to remove (~1h); high to implement
**Risk**: Low to remove

---

## Evidence

**The UI is complete and fully designed:**
- `SettingsSyncTab.tsx:23-26` — `FILE_PROVIDER_OPTIONS` includes `{ value: 's3', label: 'S3 兼容存储' }`
- `SettingsSyncTab.tsx:440-500` renders six credential inputs for S3: `Endpoint` (`:447` placeholder `https://s3.example.com`), `Region`, `Bucket`, `Access Key` (`:482`), `Secret Key` (`:495`), prefix
- Section copy at `SettingsSyncTab.tsx:384` promises: **"将本地数据库整包备份到自有云端存储"**
- All six values are submitted and stored (`SettingsSyncTab.tsx:158-159` → `PUT /api/settings` → `sync_file_access_key`, `sync_file_secret_key`, …)

**The backend refuses to do anything:**
- `sync_engine.go:255-256` — `case "s3": return fmt.Errorf("file provider \"s3\" not supported yet")`
- The **only** consumer of any `sync_file_*` key is `TestFileConnection` (`sync_engine.go:250-262`)
- For WebDAV that function performs a single `PROPFIND` probe (`sync_engine.go:266-295`) — **there is no code path anywhere that reads or writes package data to WebDAV**
- `internal/sync/provider.go:173-185` declares a `FileProvider` interface with `FetchPackage`/`StorePackage` — **zero implementations**
- `provider.go:131-170` declares `ReaderPackage` + `PackageFolder`/`PackageSource`/`PackageItem`, referenced **only inside that dead interface**
- The code says so itself: `provider.go:130` "（P3 实现，此处仅定义契约）", `provider.go:172` "（P3 实现）", `sync_config.go:52` "（P3 交付，预留枚举）"

**And the error leaks raw English into a Chinese UI**: `sync_engine.go:256` returns `file provider "s3" not supported yet`, surfaced verbatim by `SettingsSyncTab.tsx:237`.

## Why it matters

Collecting a user's S3 access key and secret, persisting them unencrypted in SQLite, and then never using them — while the UI says the database will be backed up to their cloud — is worse than having no such option. It creates a false expectation of protection, and it leaves credential material at rest for a feature that does not exist.

The credentials are stored in plaintext: `sync_config.go:72-76` reads them directly from the `Setting` table; `maskSecret` (`:65-76`) only masks on **read** for display. `GET /api/settings` (sensitive, but unauthenticated by default per `handlers/reader.go:181-183`) returns them raw.

## Expected behavior

Either implement file-based sync, or remove the UI that promises it.

## Acceptance criteria (recommended: remove now, implement later)

- [ ] `s3` removed from `FILE_PROVIDER_OPTIONS`
- [ ] The six S3-only fields removed from `SettingsSyncTab.tsx:440-500`
- [ ] The "将本地数据库整包备份到自有云端存储" section replaced with an honest one-line statement that file sync is not yet supported (or removed entirely)
- [ ] `internal/sync/provider.go:131-185` deleted: `ReaderPackage`, `PackageFolder`, `PackageSource`, `PackageItem`, `FileProvider`
- [ ] `sync_config.go:52` `s3` removed from `syncFileProviders`
- [ ] No user-visible English error strings (`sync_engine.go:256,258,269,285,294`)
- [ ] Existing `sync_file_*` Setting rows cleaned up or left harmless — **verify** which, given `UPDATE /settings` upserts by key

## Acceptance criteria (option B: implement)

- [ ] `FileProvider` has at least one real implementation
- [ ] `StorePackage` writes; `FetchPackage` reads; a round-trip test passes
- [ ] S3 **or** WebDAV is dropped from the options, and the option matches what is implemented
- [ ] Credentials documented in the UI as stored unencrypted locally

## Implementation notes

- Prefer removing. There is a real question of whether file-based sync is the right feature at all for a local-first reader whose users may have hundreds of subscriptions — the package format does not exist yet, and designing it is a project, not a patch.
- If removing, keep `webdav` in the options **only if** WebDAV gains real read/write. A "test connection" button that proves credentials work, for a feature that then transfers nothing, is not useful.
- Separately: disclose in the sync UI that credentials are stored locally in plaintext. That applies to the working `api_*` credentials too, and is a one-line honesty fix.

## Verification

Manual: open Sync settings — no S3 option, no S3 credential fields, and the file-sync section states the truth. `grep -r 's3' apps/web/src server/go/internal/sync` returns nothing but unrelated matches.