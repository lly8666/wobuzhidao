# Native WFP read-only layer probe candidate, after source a455 full scoped Actions PASS — 2026-10-13

## Verified previous gate
Incoming exact HEAD a4558ce9a6d4cc25c3b76606bacfbe22b00ba195, all scoped test Actions final:
- N4 owned native transaction matrix 38347146604 SUCCESS Linux race, Windows2022, Windows2025 and receipt. Artifact11789203022 SHA256 sha256:497491152359e7d08ad9fedc75b4199b73cec78f2a24d9a55410e92f5a385266. Windows native strict owner route test now includes Microsoft InitializeIpForwardEntry correct flags and passes.
- Userspace TCP/native ABI Action38347146607 SUCCESS, artifact11789081916 SHA256 sha256:c05edb0356b298ec24e4e00ae6a4615c5c221a601b035b9fc2e3d7fdfcf1ecb8.
- Foundation38347146586 SUCCESS all seven required (Linux/Windows active, P2, P4, repo contract); artifact11789546119 sha256:779eea90dcc79109bb0fbe885073208541201bcd9de926b9e5628d2a4a621eb9. P5/P6 extended skipped/not run.
- Lifecycle38347146511 SUCCESS.
- Historical WinDivert NO_GO, archive reactivation workflow FAIL, reversed map test FAIL, broad grep false positive FAIL, native SOCKADDR type FAIL and Microsoft Win32 route initializer FAIL remain preserved, not reclassified as PASS.

## Next precise no-shell Win32 capability probe
`internal/windowsclient/native_wfp_readonly_windows.go` now uses Go lazy `fwpuclnt.dll` calls (FwpmEngineOpen0 -> FwpmLayerGetByKey0 -> FwpmFreeMemory0 -> FwpmEngineClose0) to VERIFY the WFP IPv6 outgoing ALE_AUTH_CONNECT_V6, IPv6 incoming ALE_AUTH_RECV_ACCEPT_V6 and IPv4 outgoing ALE_AUTH_CONNECT_V4 layer GUIDs. No persistent or dynamic WFP filter is installed, nor any network route/DNS/driver changed. Windows2022/2025 matrix contains tests for read-only native WFP engine access. Availability is necessary, not sufficient for effective leak prevention. The runtime `CrashPersistentGuard` remains deliberately ABSTRACT and cannot be used without a native owned persistent implementation.

## Current hard hold
User requirement no PowerShell in FINAL product still UNMET: Windows CLI retains original PowerShell networking calls, DNS/IPv6 old rules and complement routes. New small route plan/Go native Direct TCP/UDP/owned route journal are not production-wired. Need authenticated/native WFP rules with crash persistence and exact cleanup, DNS interface native Set/Get with older OS failclosed, Wintun lease/DHCP address state ownership, and hosted admin real Apply/Cleanup before switching, no leakage allowed. N6 NOT_STARTED; physical NOT_RUN; no deploy or merge.
