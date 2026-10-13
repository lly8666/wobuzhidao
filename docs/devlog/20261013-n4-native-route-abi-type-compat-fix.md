# N4 pinned x/sys Windows native ABI type compatibility — fix candidate 2026-10-13

## Original failure retained
Native Go Win32 IP Helper facade SOURCE50172e3362094431db950bfe9540729d8cc2d806 is NOT PASS:
- Dedicated Action38343998563 job115121094856 Windows FAIL: `undefined: windows.RawSockaddrInet` in internal/windowsclient/native_route_windows.go. Current pinned golang.org/x/sys v0.33.0 has older types, not the latest upstream types referenced by general online docs.
- Lifecycle Action38343998549 core job115121094736 FINAL FAIL same compile cause.
- Foundation Action38343998570 Windows active-go-tests job115121134248 FAIL same cause; Linux tests unrelated. Full Foundation final conclusion must be checked separately.

No routes, DNS/firewall or Wintun were touched by these build failures.

## Small compatibility fix
Replace the unavailable `windows.RawSockaddrInet` with an identical local SDK `SOCKADDR_INET` Go union: `Family uint16; Port uint16; Data [6]uint32`; ABI size 28 bytes. The enclosing MIB_IPFORWARD_ROW2 layout still uses Windows documented 104-byte size, offsets 12/44/100. Does not bump x/sys, patch upstream or alter product traffic. The existing exact-source Windows ABI layout and non-elevated read-only lookup test must execute on Actions before any further native code.

## Still OPEN
Fully script-free Windows product remains blocked on owned native address/DHCP, DNS, IPv6/DNS WFP guards, crash journal/revert and replacing `cmd/wbd-client` PowerShell call sites. The current source contains only disabled N4 userland Direct TCP and IP helper route primitives. No real Wintun/NIC or physical test, no N6 package, no merge/deploy. Do not call full qualification PASS from a successful native ABI test.
