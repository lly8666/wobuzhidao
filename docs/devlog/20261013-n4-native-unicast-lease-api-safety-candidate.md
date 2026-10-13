# N4 native Windows unicast IPv4 lease low-level ABI and fail-closed tests — 2026-10-13

## Recovered remote exact baseline
Incoming HEAD e754857035a92814f6d0630da332cbf796cb1faf, verified all completed Actions:
- N4 Win2022/Win2025 native IPv4 MTU GetIpInterfaceEntry read-only + Linux race matrix38351451182 FINAL SUCCESS, artifact11790822647 sha256:a3243ffcb24dee9fc97b9c7ff2d3255f5b886b45cbee96c58b708eecb3cb359a.
- Foundation38351451270 FINAL SUCCESS all seven mandatory jobs (Windows/Linux Go, P2, P4, repo contract), artifact11790822525 sha256:6ee4f495e72c304681cd6537e64b91ee2fe5c0873fd2b3fa311cc290ee8c5c9a. P5/P6 extended skipped.
- Lifecycle38351451111 FINAL SUCCESS.
- Previous exact source404403c1 persistent WFP safety **TEST-NET ONLY** lab Actions38351024667 and native guard matrix38351024676 SUCCESS, Foundation38351024665 SUCCESS. The unconnected multi-Wintun-unsafe guard requires ExclusiveSingleWintun; DO NOT use in production before network owned setup is qualified. No physical/user machine changed.

## New source-only candidate and no-mutation Actions
`internal/windowsclient/native_lease_windows.go` adds Go-native MIB_UNICASTIPADDRESS_ROW Win32 APIs Initialize/Get/Create/Delete. Strict canonical /32 IPv4 WBD host lease and InterfaceIndex; explicitly sets SOCKADDR_INET AF_INET and OnLinkPrefixLength=32 so it never broadens an onlink capture route. Reads DAD state and refuses Tentative/Duplicate/Deprecated; never calls an address Preferred before native OS Get confirms it. Under `N4WindowsLeaseWriter`, creates only when a durable verified CrashPersistentGuard is present and the exact Wintun interface has no preexisting IPv4 (including foreign APIPA). Delete needs a private in-process token from successful Create and again verifies native address identity, prefix, interface, DAD and guard. Invalid index/IPv6/broad CIDR/broadcast/link-local/foreign lease/canceled/missing guard fail BEFORE OS mutation.

No successful Create/Delete will run on hosted CI: test asserts row ABI, canonical IPv4, DAD state safety, foreign subnet/skip-as-source refusal, nonexistent NIC native read-only behavior and denial before Win32 writes. Dedicated N4 workflow includes new files and script-reference check, exact-source Windows2022/25 + Ubuntu-race + Foundation/lifecycle.

**Critical limitation:** in-process success token is not crash-durable; low-level writer has not been connected to full owned address/MTU/DHCP/DNS persistent transaction or verified Wintun adapter identity. It is NOT an N4 complete/fail-safe production Apply implementation and needs an intent-before-Create persistent journal, post-create DAD wait, crash ambiguity hold and multi-Wintun-safe persistent WFP before activation. Live cmd/wbd-client still invokes powershell.exe and old complement rules; no physical until N6 three-target same-source artifacts. N2 loss/efficiency/GE/80s unresolved. No local tests/merge/deploy.
