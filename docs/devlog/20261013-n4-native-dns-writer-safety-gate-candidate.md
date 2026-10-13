# N4 Go native DNS Set request and guard fail-closed gate (2026-10-13)

## Recovery and exact starting SOURCE
Recovered remote HEAD ef9afb815e14f4926f5485695b87c93f4052396b after session interruption. Its actions all FINAL SUCCESS:
- N4 Windows2022+Windows2025 real host DNS_INTERFACE_SETTINGS V1 **READ-ONLY** + Linux race Action38348307184, artifact11789504423 SHA256 sha256:71b8bdc2bba2852267578c829405d2aa8e3b57fa778975f51dcc4d90e51181e1.
- Foundation38348307083 all seven required jobs SUCCESS (repo contract, Windows/Linux Go and race, P2, P4), artifact11789419397 SHA256 sha256:f1db20a429ac4e8e45055052dfaf8855d42556144dcd1ec1f8ef246b281aaf2e. P5/P6 skipped NOT_RUN.
- Lifecycle38348307211 FINAL SUCCESS.
No product DNS write, network route Apply/Cleanup, WFP filter, elevated Wintun driver or physical machine tested. Previous original failures remain preserved in earlier devlogs.

## Current actual changes
- `internal/windowsclient/native_dns_write_windows.go`: native Win32 `SetInterfaceDnsSettings` V1 x64 64-byte ABI, constructs only `DNS_SETTING_NAMESERVER (0x2)` IPv4 static addresses (canonical comma UTF-16, bounded max 8, unique valid global IPv4). No suffix, profile, LLMNR/mDNS, IPv6 or other flag mutation, and keeps Go backing buffer live across system call. Microsoft's SDK requires fields not covered by flags be zero; verified from docs.
- Writes are guarded by `CrashPersistentGuard` present, `Durable()==true` and `Verify(ctx)==true`; canceled, missing/unarmed/volatile guard return error **before any native write**. No product caller activates this primitive.
- `n4ReadOwnedDNSBaseline` enforces an otherwise empty, exclusively owned Wintun DNS baseline, rejecting prior static/profile/search/domain/DNS flags. This is an additional ownership precondition; it is NOT evidence Wintun ownership is already established. A production DNS state transaction and journaled restoration are **still required**.
- Windows unit tests verify the exact request and ABI flags/pointers, storage, all invalid IPv6/duplicate/broadcast/multicast/empty/overflow cases, foreign DNS baseline refusal, canceled/invalid/no durable guard fail-closed; **zero successful SetInterfaceDnsSettings calls** in Actions. Two Windows runner editions plus Linux -race+Foundation/lifecycle in exact-source GitHub workflow. CI validation may still fail; record exact jobs and correct without altering historical source.

## Critical hold: scriptless N4 is NOT DONE
A persistent WFP IPv6 and non-Wintun DNS anti-leak filter, with owned GUID recovery after process crash and no dynamic WFP session lapse, is not implemented. Neither is native Wintun DHCP/static IPv4/MTU ownership and reversible DNS setting journal. The active Windows CLI still runs `powershell.exe` / `scripts/windows_client_network.ps1` and writes large route complements; the new compact /1 and userspace TCP Direct remain intentionally disabled. No Wintun live/admin, physical or N6. Finish guards, native owned network Apply/Cleanup/Recovery and Actions before switching runtime, removing active PowerShell, N6 three-target same source artifacts/hashes, and then separate physical acceptance. Old N2 80s/efficiency/GE remains OPEN.
