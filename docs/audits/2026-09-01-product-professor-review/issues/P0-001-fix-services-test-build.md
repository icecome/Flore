# P0-001 — Fix the untracked test build before it turns CI red

**Priority**: P0
**Category**: Reliability / CI
**Labels**: `bug`, `ci`, `tests`, `p0`
**Difficulty**: Trivial (1 character)
**Risk**: None

---

## Evidence

```
$ git ls-files --error-unmatch server/go/internal/services/item_service_test.go
error: pathspec '...' did not match any file(s) known to git

$ git status --porcelain | grep test
?? apps/desktop/internal/updater/updateinfo_test.go
?? server/go/internal/services/filter_test.go
?? server/go/internal/services/folder_service_test.go
?? server/go/internal/services/item_service_test.go
?? server/go/internal/services/source_service_test.go

$ cd server/go && go vet ./...          # working tree
internal\services\item_service_test.go:116:2: declared and not used: normal
[exit 1]

$ git worktree add --detach /tmp/head HEAD && cd /tmp/head/server/go
$ go vet ./... && go test ./...
ok  github.com/rss/go-server/internal/services   8.243s
ok  github.com/rss/go-server/internal/sync       6.318s
[exit 0]
```

`item_service_test.go:116` — `normal, _ := seedItems(t, s)` declares `normal`, which is never used (the test queries via `db.Where("title = ?", "n1")` instead).

`.github/workflows/ci.yml` order: **1** `gofmt -l` → **2** `go vet ./...` → 3 `go test -cover` → 4 `tsc --noEmit` → 5 `go build` → 6 desktop compile check → 7 updater tests → 8 `build:web`.

## Why it matters

HEAD is green, so this is **latent, not active** — and that is exactly what makes it dangerous. The four untracked test files are the only coverage for `filter`, `folder_service`, and `source_service` (≈1,900 lines). The natural next developer action is to commit them.

The moment they land, `go vet` fails at **step 2 of 8**. Steps 3–8 stop executing on every push and every PR — including the frontend typecheck and the frontend production build. The four new test files never run in CI, so the coverage they were written to provide silently does not exist. A developer would see "CI failed" and reasonably conclude their tests were bad, not that a helper's return value was unused.

Compounding: `internal/services` currently has 22.2% measured coverage on HEAD — it is the highest-value package in the repo, and it is one typo away from being untested again.

## Expected behavior

- `go vet ./...` and `go test ./...` pass in the working tree.
- All 4 untracked test files are committed and execute in CI.
- CI's 8 steps all run.

## Acceptance criteria

- [ ] `cd server/go && go vet ./...` exits 0 in the working tree
- [ ] `cd server/go && go test ./...` exits 0, with `internal/services` reported as `ok` (not `[build failed]`)
- [ ] The 4 untracked test files are added to git and appear in `git ls-files`
- [ ] A CI run on the resulting commit completes all 8 steps
- [ ] `git status` for `server/go/internal/services/` shows no untracked test files

## Implementation notes

Change line 116 from:

```go
normal, _ := seedItems(t, s)
```

to either:

```go
_, _ = seedItems(t, s)
```

or, better, assert on `normal` — the test seeds two sources (`n1`, `n2`) and the helper's return value was presumably intended for that. Check what `seedItems` returns and use it rather than discarding it; silently discarding a seed helper's output is how the bug arose.

**Do this first, and separately from any other change.** It is the only step that restores the safety net that makes every other fix in this audit verifiable. Do not bundle it with refactoring.

## Verification

```bash
cd server/go
go vet ./...          # must exit 0
go test ./...         # must exit 0, internal/services => ok
git add internal/services/*_test.go
git status --porcelain | grep '??'   # must be empty for this dir
```
