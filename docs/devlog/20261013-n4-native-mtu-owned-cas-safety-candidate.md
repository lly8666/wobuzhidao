# N4 native Windows MTU write/restore gated on exact Wintun owner and current-row CAS (2026-10-13)

## Baseline
Exact remote head a6ee98c31a87761b4ec4bc18cc18505fd193862c:
- Native IPv4 lease ABI/DAD guarded invalid/foreign tests N4 Actions38352058023 FINAL SUCCESS four jobs Win2022/2025 and Ubuntu race/receipt; artifact11790848634 sha256:f3b0849ac01208062caffd6e9a03acd15125ab7e79924e7e6c3904e28318819f. **No successful Windows address Create/Delete**.
- Foundation38352058035 FINAL SUCCESS all seven mandatory jobs; artifact11791690418 sha256:ffe05a301029d1dd15a7593a062a1fcf5bdc97425900cf091ccc42c5ed905854.
- Lifecycle38352058045 FINAL SUCCESS.
N4 persistent WFP guard still restricted to unconnected exclusive-single-Wintun backend and TEST-NET isolated filter Actions. Product remains PowerShell-based.

## New low-level native MTU writer (NOT PRODUCT-CONNECTED)
`internal/windowsclient/native_mtu_write_windows.go` adds Win32 SetIpInterfaceEntry using actual entire MIB_IPINTERFACE_ROW from GetIpInterfaceEntry (never initialized-zero row for mutation). Requires an external proof from a *live currently owned Wintun handle* (not mutable display name/index alone) plus a verified crash-persistent WFP guard. Validates original IPv4 interface snapshot and bounded target 576-9000, checks live same index, MTU, metric, connected, weak host behavior, automatic metric, disable default routes before modifying. Refuses stale or foreign baseline; for IPv4 explicitly requires/sets SitePrefixLength 0 as Microsoft documents. Sets only NlMtu and re-reads for confirmation. A successful write returns an in-process receipt for owner-only Restore; Restore checks exact after-state before writing the original MTU. Cancellation/foreign/unsafe guard deny before Set; a parent durable journal is still MANDATORY because post-write failure may leave an ambiguous state after process crash.

`native_mtu_write_windows_test.go` covers denied missing or dynamic guard, foreign Wintun, unarmed guard, canceled write, zero or unconfirmed restore token, invalid MTU, exact snapshot identity and stale metric/weak-host behavior. It NEVER calls SetIpInterfaceEntry successfully or changes runner configuration. N4 owned transaction Windows2022/25 matrix includes tests; Linux -race and Foundation/lifecycle run on exact source.

## Remaining blockers
In-process MTU receipt and leased address token are NOT adequate crash recovery, and no live Wintun owner implementation exists. Need durable journal and real NIC identity/foreigner checks, native WFP DNS+IPv6 filter with correct multi-Wintun semantics or exclusive-only, native DNS read/write/restore, Wintun lease/DHCP/MTU rollback and real admin isolated Actions before enabling 2 x /1 capture and tun2socks DirectTCP. Remove PowerShell and .ps1 from ACTIVE Windows runtime only then. N2 open efficiencies/GE/80s tail and N6/physical NOT_RUN. No local build, merge or deploy.
