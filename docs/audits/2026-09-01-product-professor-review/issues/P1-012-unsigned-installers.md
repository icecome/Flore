# P1-012 — Windows SmartScreen and macOS Gatekeeper block every install

**Priority**: P1
**Category**: Distribution
**Labels**: `distribution`, `signing`, `macos`, `windows`, `p1`
**Difficulty**: High (requires paid accounts + certificate procurement)
**Risk**: Medium — signing changes the build pipeline and can invalidate the update path

---

## Evidence

**Windows — no Authenticode signing anywhere:**
`.github/workflows/release.yml` contains no signing step; the NSIS installer (`installer.nsi`) is built and published unsigned. Every download triggers SmartScreen "Windows 保护了你的电脑 — 未知发布者".

**macOS — ad-hoc signing only, and the workflow says so:**
`release.yml` header comment lines 16-19:

> **注意**：macOS 包未做 Apple 开发者签名/公证，下载后首次打开会被 Gatekeeper 拦截，用户需右键「打开」或 `xattr -d com.apple.quarantine <文件>` 放行。

Build matrix produces `flore-setup-darwin-arm64-<ver>.dmg` and a portable zip; the dmg is ad-hoc signed (`release.yml:143-147`) and not notarised.

**Interestingly, the updater IS properly secured** — a contrast that makes the gap look like an oversight rather than a philosophy:
- `apps/desktop/internal/updater/verify.go:14` — Ed25519 signature verification against an embedded public key
- `release.yml` signs `update.json` per asset and **fails the build** if `FLORE_UPDATE_SIGNING_KEY` is absent
- Desktop updater tests cover signature tampering, zip-slip, and version comparison, and run in CI

So binaries delivered *after* install are cryptographically verified; the binary the user *first* downloads is not signed at all.

## Why it matters

For a desktop product whose installation story is "download, extract, run" (`README.md`), an unsigned binary is the first thing a user meets and the most likely point of abandonment. The macOS case is worse than an extra click: ad-hoc signed apps can be reported as "已损坏" rather than merely blocked, which reads as a broken download.

This is the blocker most likely to make a launch look unsuccessful for reasons that have nothing to do with the code.

## Expected behavior

A signed, notarised build that opens without a security warning on both platforms.

## Acceptance criteria

- [ ] **Minimum**: a published `SHA256SUMS` file plus prominent install instructions on the release page (partially mitigates but does not remove the warning)
- [ ] **Windows**: Authenticode-signed `.exe` and portable `.zip` contents; SmartScreen no longer shows "未知发布者"
- [ ] **macOS**: Developer ID Application certificate, hardened runtime, `codesign --deep --force --options runtime`, then `notarytool submit ... --wait` and `stapler staple` on the dmg
- [ ] CI holds certificates in Secrets and the signing steps fail loudly if a secret is missing (**do not sign silently with a fallback**, matching how `update.json` signing already behaves)
- [ ] The signed artifact is what the updater downloads and verifies
- [ ] Release-page copy updated to remove the Gatekeeper workaround instructions once signing works

## Implementation notes

- **Cost**: Windows Authenticode requires an EV or OV code-signing certificate (annual cost, and EV requires hardware token or cloud HSM); Apple requires a paid Developer Program membership. Both are real, recurring, and outside what a solo project usually budgets — which is likely why this is unfixed. That makes it a **decision to make explicitly**, not a task to silently defer.
- **Sequencing**: do the checksums + instructions first (an afternoon, immediate benefit), then pursue certificates. Do not let "we need signing" block the other P0/P1 fixes.
- **CI plumbing**: add signing as a conditional step keyed on secret presence, and make the release *fail* rather than publish unsigned when the secret is expected. The existing `update.json` signing already models this pattern — mirror it.
- macOS notarisation cannot be tested locally without the certificate; budget for one or two CI round-trips.
- If signing is out of scope for now, say so in `README.md` explicitly rather than only in a workflow comment — the workflow comment is invisible to users.

## Verification

Windows: download the release on a clean VM with no prior trust; SmartScreen must not warn.
macOS: download the dmg on a clean machine; `spctl -a -vvv -t install <app>` must report `accepted`, and opening must not require right-click → Open.