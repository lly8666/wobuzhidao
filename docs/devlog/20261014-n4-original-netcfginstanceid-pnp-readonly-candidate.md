# N4 exact original Wintun PnP device identity and status: Windows read-only native candidate (2026-10-14)

Repo `lly8666/wobuzhidao`; branch `next/adaptive-fec-aes-tun-20261010`. Parent code SOURCE `9c1983b2a25ca58d66312fcbfcbfd175c42a60b4`. New code SOURCE must be verified by new Actions; do not inherit PASS.

## Baseline exact Actions evidence

Parent SOURCE N4 matrix38470517894 Windows2022/2025+Linux race FINAL SUCCESS, real created Wintun38470517830 both Windows workflow SUCCESS, Foundation38470517944 seven required SUCCESS, Lifecycle38470517902 SUCCESS. Both Win2022 and Win2025 real child-crash subtests remained **SKIP UNSUPPORTED_EXACT_ORIGINAL_IPV4_ROW_UNAVAILABLE**, while bounded native original GUID/LUID/IPv4-index GAA showed exact original adapter still present; both independently read AF_INET=MISSING_1168 AND AF_INET6=MISSING_1168 after TerminateProcess. WAL retained and test-only fake WFP guard armed; there was no native rollback PASS. Detailed run/job/ZIP SHA256 are in `docs/devlog/20261014-n4-ghost-both-ip-family-rows-native-actions-final.md`. Original historical Windows FAIL and C04/C05/C10/80s residuals unchanged.

## New strictly read-only PnP investigation

Adds `n4ProbeOriginalWintunPnP`, a bounded native Windows SetupAPI/ConfigManager reader. Under exact original GAA triple verification before and after, it enumerates at most 256 **SWD\\Wintun** devices in `GUID_DEVCLASS_NET`, reads each Wintun driver-key `NetCfgInstanceId`, parses its GUID, and matches only the exact WAL original GUID. It rejects absent/ambiguous/duplicate matches or failure to read registry evidence; does NOT trust alias, a similarly named device, or reused IPv4 index. For a unique match, it obtains the PnP device-instance identifier and emits only its SHA256 hash (not the raw instance string), and reads `CM_Get_DevNode_Status` into diagnostic status `EXACT_DEVNODE_PROBLEM` or `EXACT_DEVNODE_NO_PROBLEM`. **Neither status permits OS modification, WFP disarm, WAL delete, SetupDiRemoveDevice or driver reinstall.** No host-wide device scan beyond the Wintun enumerator. No accounts, DNS strings, packet contents, keys or sensitive registry values logged.

Win2022/Win2025 real driver test compares the exact original device-instance hash while child is live and after TerminateProcess (the latter only when old GUID/LUID/index still exists with both protocol MIB rows missing). If PnP after-crash is unproven, it records `UNSUPPORTED_NATIVE_PNP_READBACK`, not a positive recovery. Unit tests reject cancellation, invalid GUID and synthetic unowned identity. New file paths trigger N4 Windows/Linux-race, real Win2022/2025, Foundation and Lifecycle on new code SHA. On Wintun's upstream implementation, orphan cleanup uses SetupAPI and ConfigManager; this feature intentionally does not invoke any global cleanup routine.

## Still incomplete

Even a matching current `NetCfgInstanceId` and device-instance hash alone are not a persisted original-generation capability. Before any future device teardown, the exact PnP device-instance identity must be durably bound into the original WAL at creation, independently verified immediately before/after mutation, and subjected to tested foreign-device and crash/retry gates in genuinely isolated Windows Actions. Windows native WFP Arm still disabled; five-stage assemblyOnly, official PowerShell/.ps1 Windows CLI and legacy routes active; full protected server /32 + two /1 capture, real direct TCP/UDP return path, N5 integration and N6 exact SOURCE 3-target P6 packages/manifest/hashes remain OPEN. Shared GitHub-hosted runner must NOT install host-global WFP or capture routes. User's future physical may assume only project-owned Wintun but Actions must finish first. `ACTIONS_READY_FOR_PHYSICAL` unset, physical `NOT_RUN`. No deployments, merges, qualification ref movements, raw pcaps or secret uploads.

## New SOURCE results

PENDING until exact Win2022/25 logs, jobs and artifacts are checked. Failures/UNSUPPORTED must be preserved without relabeling.
