# P0-004 — Add tests for the SSRF boundary (`urlpolicy.go`)

**Priority**: P0
**Category**: Security
**Labels**: `security`, `tests`, `ssrf`, `p0`
**Difficulty**: Medium (~4h)
**Risk**: Low — tests only, no behaviour change

---

## Evidence

`server/go/internal/services/urlpolicy.go` is 258 lines implementing the project's most valuable correctness asset:

1. Scheme allow-list http/https (`:20-23`, enforced `:139,177`)
2. Cloud-metadata deny-list (`:72-78`) — `metadata.google.internal`, `metadata.azure.com`, `metadata.tencentyun.com`, `169.254.169.254`, `in-addr.arpa`
3. Private/reserved IP rejection (`isPrivateIP:83-97`) — loopback, RFC1918, link-local, multicast, unspecified, explicit `0.0.0.0/8`, IPv4-mapped IPv6 via `To4()`
4. **The DNS-rebinding fix** (`newSSRFDialContext:208-243`): resolves DNS **once** via `lookupHostWithCache`, validates every returned IP, then dials the **validated IP string** (`:235 net.JoinHostPort(ip, port)`) rather than re-resolving the hostname — closing the TOCTOU window between validation and connection.

Applied on the direct transport (`fetcher.go:258`, `readability.go:183`) **and the proxy branch** (`fetcher.go:247-253`, with an explicit comment stating that omitting it would allow a proxy-mediated LAN pivot).

**Test coverage: zero.** No `urlpolicy_test.go` exists. Measured coverage for `internal/services` is 22.2%, and the file is not part of it. `internal/handlers`, `internal/database`, `internal/models`, `backend`, `pkg/paths` all have **0% and no test files**.

## Why it matters

This is the code that prevents a malicious RSS feed or a crafted `?url=` parameter on `/image-proxy` from reaching internal network services. It is subtle, it is correct, and it is exactly the kind of code a future refactor breaks silently — for example, by "simplifying" the dialer to use `addr` instead of the validated `ip`, which reintroduces the rebinding window while leaving all existing tests green.

The project's own review (`docs/planning/hai-razor-frontend-review.md`) identified this same gap. It remains unfixed.

## Expected behavior

A `urlpolicy_test.go` covering each layer, with a test that would fail if the dialer reverted to hostname dialling.

## Acceptance criteria

- [ ] `isPrivateIP` table-driven test: loopback (v4/v6), RFC1918 (all three ranges), link-local (`169.254.x.x`), multicast, unspecified, `0.0.0.0/8`, IPv4-mapped IPv6 (`::ffff:127.0.0.1`), and at least one valid public IP asserted **allowed**
- [ ] `ValidateURL` tests: `file://`, `gopher://`, `http://` rejected; metadata hostnames rejected; suffix-matching verified (`evil.com/metadata.google.internal` must be allowed, `metadata.google.internal.evil.com` must be rejected)
- [ ] `newSSRFDialContext` test using a fake resolver: a hostname that resolves to a public IP then a private IP must **not** connect to the private IP
- [ ] Redirect test: a server responding `302 → http://127.0.0.1` must be blocked at dial time
- [ ] All tests run in CI (`go test ./...` green)

## Implementation notes

- The rebinding test needs a controllable resolver. `lookupHostWithCache` (`:40`) is the seam — inject a lookup function, or test the dialer directly with a stub.
- For the redirect test, use `httptest.NewServer` returning a 302 to a loopback address, then assert the client errors rather than connecting.
- Do **not** weaken the loopback exemption in `handlers/reader.go:93-96` while doing this — that exemption is a correct local-first decision, deliberately documented.

## Verification

```bash
cd server/go && go test ./internal/services/ -run 'SSRF|PrivateIP|ValidateURL|Redirect' -v
```
