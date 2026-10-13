# N4 userspace Direct + scriptless Windows native API initial Actions gate (2026-10-13)

## New user permission and requirement
The user changed the operative N4 constraint: the critical property is that Windows route table NOT be flooded with domestic IP prefixes. The already implemented historical `old/internal/tunsplit` policy and `tun2socks/v2 v2.6.0` are allowed and preferred, even though userspace TCP direct terminates the inner TCP and opens an outbound physically bound Windows socket. User additionally requires **no PowerShell scripts in the product**: Windows route/address/DNS/IPv6/cleanup must use Go native API, not a shell dependency.

## Incoming state
Verified exact remote HEAD 1b28d9209bbb279bfe809bb84e0e3531089614d6; docs/STATUS.json previous active N4 WinDivert NO_GO (Action38341416320), last Foundation38341789232 final SUCCESS. Old WinDivert finding remains unchanged, merely no longer targets chosen architecture.

## Historical source read
- old/internal/windowsruntime/routing_policy.go: all eight CN/LAN/Other routing policies map to host Full capture; per-packet choice inside TUN, not thousands of host routes.
- old/scripts/windows_tun_route.ps1 (historical read-only): captured 0.0.0.0/1,128.0.0.0/1 with optional small LAN/DNS/underlay exception; script is NOT approved as new runtime mechanism.
- old/internal/tunsplit/classifier.go: in-memory merged CN IPv4 intervals.
- old/internal/tunsplit/direct_windows.go: tun2socks v2.6.0/gVisor stack + physically bound DialContextWithOptions(InterfaceIndex), two half-duplex pumps for TCP, bounded UDP sockets.
- old/internal/windowsruntime/routing_policy_test.go: host route policy invariants (including all eight policies); new current client's network_plan.go still emits massive complement routes and PowerShell. Do not call that final.

## Exact first gate
New `internal/windowsclient/n4_native_api_gate_windows_test.go` (Windows non-elevated export probe only) and dedicated `.github/workflows/next-adaptive-n4-userspace-no-powershell-feasibility.yml`. Workflow uses only Go and bash, compiles old Go userspace Direct and old route behavior in Actions Windows; queries DLL exports from iphlpapi/fpwuclnt for planned Go native owned networking. All gates SOURCE-scoped. It does not load Wintun, create routes or firewall entries, start DNS, transmit physical TCP, run as admin, or modify OS. Action SUCCESS will be scoped feasibility only, never end-to-end proof. Do not claim no PowerShell in final product until old script path is physically eliminated.

## Next
Gate then (only on passing) implement generation-scoped bounded Direct stack, copy-free TUN ring ownership, route cap, Go-native iphlpapi route/address/MTU + WFP firewall, native DNS strategy/owned journal, cancellation and reverse cleanup; eliminate legacy PowerShell from non-archived product packaging and workflows. Preserve N2 residual issues, N0/N1/N3/N5 qualified scope, N6 NOT_STARTED, physical NOT_RUN.
