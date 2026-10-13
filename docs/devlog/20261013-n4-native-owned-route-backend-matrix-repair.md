# N4 Windows native route backend + shell-reference gate correction — 2026-10-13

## First transaction Action failure records
- SOURCE132e80d5 initial Action38345945687 Linux/windows matrix FAILED due reversed Go map value/existence assertion in test; corrected SOURCE4fd6e6d.
- SOURCE4fd6e6d Action38346113126: Linux hosted race job115127810185 SUCCESS across four repetitions and classification checks. Windows 2022 and 2025 Go N4 owned-transaction/ABI/TCP tests finished successfully but scan `grep -iE 'powershell|...'` incorrectly matched harmless Go code comment `No PowerShell`, producing Windows job failures; receipt skipped. This was scanner mistake, not claimed PASS. Fix scan to execution names `powershell.exe`, `exec.Command`, and script filename `.ps1`; rerun both hosted Windows versions and Linux race on new SOURCE.

## Windows Go backend
`internal/windowsclient/native_route_backend_windows.go` links the guard-required generic owned route journal to documented native `GetIpForwardEntry2`, `CreateIpForwardEntry2`, `DeleteIpForwardEntry2` calls. `Lookup` compares exact interface index, network and prefix length, next hop, metric, not loopback, not published; a pre-existing or wrong-owner route is never considered WBD-owned and deletion rejects it. `CreateIfAbsent` refuses any existing entry and obeys context cancellation. Tests use layout/match mismatch cases and read/cancellation only; hosted Windows DOES NOT execute Create/Delete or mutate routes. The native source itself contains no PowerShell calls.

## Still blocking production activation
No native crash-persistent DNS+IPv6 WFP guard, strict Wintun address/DHCP ownership and journal, or full native DNS interface settings/NRPT parity. `cmd/wbd-client` still contains `powershell.exe` and active script and uses complement host routes. This is NOT compliant with user's final script-free product requirement yet. Do not delete old protective script until parity exists; do not enable new /1 capture on physical machine. Actions only. N6 and physical NOT_RUN; old FAIL records retained.
