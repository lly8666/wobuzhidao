# P6 release-candidate packaging candidate

- Date: 2026-09-21
- Parent: `9ff53b57f641dd0bfa899dc2acad87cc7982322c`
- Milestone: P6
- Reuse from old: none
- Physical qualification: NOT_RUN

## Scope

P6 begins with a new active packaging path rather than reactivating any archived release script.

New build identity:
- `internal/buildinfo` owns only `Version` and `SourceSHA`.
- development defaults remain `dev` / `unknown`.
- P6 builds inject exact values with Go `-ldflags -X`.
- `wbd-client --version` and `wbd-server --version` expose program/version/source SHA/toolchain target identity.
- this package does not alter protocol wire format, runtime policy, FEC, padding, or lifecycle behavior.

## Independent target jobs

One GitHub Actions VM/job builds one target:

1. linux/amd64
   - wbd-client
   - wbd-server
   - native `--version` verification

2. linux/arm64
   - wbd-client
   - wbd-server
   - hosted cross-build
   - target verified using `go version -m` and embedded exact-SHA bytes
   - native/physical ARM64 execution remains P7

3. windows/amd64
   - wbd-client.exe
   - windows_client_network.ps1
   - native `--version` verification
   - Windows server is explicitly UNSUPPORTED, not replaced with a stub artifact

## Manifest and receipt

`tools/p6_build_release.py` generates `manifest.json` containing:
- exact source SHA
- release-candidate version
- target GOOS/GOARCH
- runner/toolchain identity
- trimpath/buildvcs/CGO/ldflags
- every bundled file size and SHA256
- IMPLEMENTED/PENDING_VALIDATION/NOT_RUN status separation
- platform-specific known limits

`tools/check_p6_release.py` independently:
- recomputes every file size/SHA256
- verifies binary bytes include exact source SHA and version
- uses `go version -m` to verify target GOOS/GOARCH
- executes `--version` for native hosted targets
- checks Linux client+server vs Windows client-only capability inventory
- writes `actions-receipt.json` only after all checks pass

The receipt reports:
- IMPLEMENTED
- ACTIONS_PASS
- PHYSICAL = NOT_RUN
- RELEASE_QUALIFIED = NOT_RUN

No hosted P6 result may be described as physical Windows/Npcap or final release qualification.
