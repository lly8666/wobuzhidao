# N4 native DNS writer gate — Windows compile failure and exact test fix (2026-10-13)

## SOURCE and original CI failure
SOURCE 54be2454e7a4ff6e4756a1c3492d13a5099a72a2 added Go-only n4OwnedDNSWrite with mandatory CrashPersistentGuard durable/verified gate, Win10 build19041+ SetInterfaceDnsSettings V1 minimal DNS_SETTING_NAMESERVER, strict at-most-8 unique global IPv4, foreign DNS baseline preflight, cancellation/invalid config tests. It is NOT connected to cmd/wbd-client and no SetInterfaceDnsSettings is invoked on a successful test path.

Initial dedicated N4 Action38349407066 FAILED Windows2022 job115138114099 and Windows2025 job115138114143: native_dns_write_windows_test.go line84 had stale '_=strings.TrimSpace' line after its strings import was removed. Foundation38349407046 FAILED Windows active-go job115138154400 for same compilation error; lifecycle38349407067 SUCCESS. Previous source ef9afb DNS baseline Actions38348307184 and Foundation38348307083 remain PASS, do not rewrite original failures.

## Fixed candidate
Delete exactly the stale line in the native DNS test file. Do not suppress the test, alter runtime or bypass go.mod readonly. Re-run exact-source Foundation, dedicated N4 multi-OS matrix and lifecycle. Scope remains solely Go ABI/input validation and fail-closed guard check: **no successful native DNS write, no installed persistent WFP, no real Wintun or host routing.**

## Next major engineering
Implement crash-persistent Windows Filtering Platform DNS+IPv6 filters and owned GUID verified recovery, native Wintun leased IPv4/DHCP/MTU transaction, DNS Before/After persistent journal with WFP verified before any Wintun capture; only then replace old PowerShell script with native network controller and activate compact /1 and userland Direct TCP. N2 80s/GE/efficiencies OPEN, N6 NOT_STARTED, physical NOT_RUN, no deploy/mainline changes.
