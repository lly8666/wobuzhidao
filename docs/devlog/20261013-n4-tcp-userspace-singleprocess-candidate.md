# N4 userspace TCP direct code imported into active single-process + route cap candidate — 2026-10-13

## Permission and scope
User explicitly prioritizes eliminating thousands of Windows China IPv4 route rules; accepts old project's TUN-internal CN classifier and userland tun2socks TCP Direct with physically bound socket, and explicitly requires NO PowerShell scripts as Windows runtime dependencies. Historical old/ files retained only as source/qualification archaeology.

## First gate SOURCE 96375cefea5b3254e8bd54c52f574871a9965b0b
- Dedicated Windows action 38343129203 job115118366065 COMPLETED/SUCCESS: Windows required native iphlpapi/fwpuclnt exports available and archived tun2socks source compiled. This is hosted API/source feasibility ONLY, no routes, no driver, no admin, no physical.
- Foundation 38343129100 FAILED repository-contract job115118365996: new workflow attempted `working-directory: old`, violating preservation rule, all other jobs skipped. Preserve this original failure! Fix is to port code into active module and test ONLY active source, not to relax check_repository.py.

## Disabled product candidate
- `internal/windowsclient/userspace_tcp_direct_windows.go`: Windows-only isolated tun2socks v2.6.0/gVisor userspace TCP, output via current PacketWriter; external DialContextWithOptions explicitly binds physical InterfaceIndex; scopes per session generation; first handler owns TWO TCP legs (internal virtual TCP and real physical TCP), by user decision. UDP stays existing NativeUDPDirect; no WFP kernel re-inject and no PowerShell.
- 1024 bounded non-blocking queue with copy ownership and Close; tests for scope, queue full, packet protocol rejection.
- `internal/windowsclient/compact_plan.go`: compact Windows capture 0.0.0.0/1 and 128.0.0.0/1 with optional DNS /32, underlay /32; DOES NOT materialize CN/direct prefixes into host route table. Tests inject 8192 candidate CN prefixes to demonstrate no complement explosion.
- Userland library pinned to old baseline v2.6.0 MIT; old full dependency graph and checksum records imported from old/go.mod and old/go.sum to make active reproducible on Windows.
- `.github/workflows/next-adaptive-n4-userspace-no-powershell-feasibility.yml`: fixed to only test new active module + non-elevated native export lookup via Go and Git Bash, no PowerShell or old runtime invocation. In-scope proof is build/queue API only.
- MAIN WINDOWS CLIENT still calls old `powershell.exe` via `runNetworkActionContext` and STILL constructs legacy complement routes. **Therefore this source is NOT scriptless product, NOT compact Windows deployment ready**. Do NOT wire these candidates or claim N4 complete until native-owned route/address/DNS/IPv6 firewall + cleanup/recovery is integrated and qualified in Actions; no interim physical tests.

## Further explicit gates
1. Pass exact-source Foundation Windows/Linux plus N4 userspace gate, record hard FAILs if any.
2. Replace hosted network-script Apply/Cleanup with Go native IP Helper API and WFP firewall (and supported native DNS strategy), full owned journal/rollback; enforce missing APIs fail closed rather than spawn PowerShell.
3. Route plan uses only two /1 + server escape/DNS small exceptions, direct CN tables reside in memory. Preserve physical socket source/interface and policy bypass DNS precedence; no route leak, generations and TCP half-close. All tests Actions first.
4. After N4 qualified, N6 same-source packages and manifest/hashes; no physical before three-target N6. Preserve N2 C04/C05/C10/80s residuals, old failure evidence, no deploy or merge.
