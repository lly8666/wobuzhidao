# N4 WinTun created-vs-opened owner and LUID fence: exact Actions evidence + dedicated matcher correction (2026-10-13)

## Source and source-scoped final receipts
SOURCE 153e33e089e52fcf6b781b897f986cc9ab4d32f9:
- Dedicated N4 native Actions38352792622 FINAL SUCCESS all four jobs Windows2022, Windows2025, Ubuntu -race and receipt, artifact11791965009 sha256:2378cf20e9d876256dfa5c54810660c6583cb14451595c0407701d903c568a34. However, the dedicated Windows matcher was `Test(N4Native|N4Scriptless|N4WFP|N4Windows|N4TCP|N4Compact|BuildCompact)` which omits new `TestN4Wintun*`! **Do not claim dedicated matrix tested Wintun owner behavior.**
- Full Foundation38352792644 FINAL SUCCESS seven required non-skipped jobs, **Windows active Go unfiltered did run TestN4Wintun***, artifact11790614647 sha256:2800c2526ce83e9de632b267be1da1a57cfd51e9203e31fb57ef42960c520f40.
- Lifecycle38352792631 core SUCCESS.
- Before that native /32 lease and DAD source a6ee Actions38352058023/Foundation38352058035 SUCCESS; guarded MTU CAS source9f Actions38352420841/Foundation38352420896 SUCCESS. No successful NIC write tested in any source.
- All previous failures as recorded in STATUS/devlog remain retained, including WinDivert-only NO_GO and native API/route SDK tests early failures.

## Correction only
Change the dedicated Windows-runner `go test -run` expression to include `N4Wintun` explicitly; include `tun_windows.go` in shell-absence static source scan. All other existing tests, correct proof boundary, source test files and code are unchanged. Next exact-source Foundation+N4 matrix will run true Wintun owner negatives 3x each Windows edition and Go Linux-race. No Wintun DLL driver or adapter is loaded or mutated by these mock negative tests.

## Latest safety and remaining tasks
The current Windows Wintun driver wrapper records `createdByProcess` ONLY after WintunCreateAdapter, not when opening a foreign existing Wintun by name. `NativeIdentity` binds the real live adapter handle's NET_LUID via WintunGetAdapterLuid -> native index and `IsCurrentOwnedWintun` requires newly-created and live; tests reject already-opened, canceled, closed, invalid/stale interfaces. The owner proof does not survive crash, nor verify a real Wintun instance without later Actions isolated admin run.
**No final N4**: production cmd/wbd-client still calls powershell.exe and installs large route complements; no fully journaled Win32 DNS/DHCP/MTU/route and broad persistent WFP DNS/IPv6 guard with correct multi-Wintun policy, no real guarded Apply/Stop in Windows admin runner, no N6 artifact/physical machine. Direct TCP userspace and compact plan intentionally disabled. N2 GE/efficiency/80s residual OPEN. No deploy/merge/local testing.
