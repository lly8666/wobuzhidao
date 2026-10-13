# N4 source-only crash-safe route transaction and Actions stress matrix — 2026-10-13

## Baseline
Exact parent HEAD 212b1aee467928d31d6700f73c79ae255492d98b; Foundation38344641973 completed SUCCESS. Previously scoped Windows DirectTCP userspace/compact planner SOURCEd29d Actions38343552100 SUCCESS; Go Windows native IP Helper route ABI SOURCEf70e Action38344213692/Factory38344213720 final SUCCESS. Historic native ABI source50172 Windows FAIL retained.

## Candidate implementation
`internal/windowsclient/native_owned_transaction.go` adds Go-only pure orchestrator atop abstract NativeRouteBackend and CrashPersistentGuard:
- Native route keys are fixed masked IPv4 prefix + interface, gateway, metric. In-memory China/CN tables are NOT host routes. Compact plan validator accepts only two /1 public captures and DNS /32 whitelisted exceptions with required physical server /32.
- Journal schema v1 is SHA256-integrity checked, stored with mode 0600; exclusive O_EXCL initial creation; each route writes **intent before Create**, then marks confirmed **owned** only after success. Crash after intent before confirmation is intrinsically ambiguous. Recovery deliberately halts without deleting foreign-maybe-owned routes when an unconfirmed key is present, retaining the journal and guard.
- Guard interface demands `Durable()==true` and verification before route modification, to reject temporary WFP dynamic sessions whose rules would vanish on process crash while host /1 capture remains.
- Cleanup walks reverse order, removes only exact matching owned routes, tolerates already missing owned routes, persists journal after each cleanup step, retains journal if guarded Delete fails. Prevents bypass on cancellation/foreign routes, duplicate Apply, corrupt journal.
- This is NOT production Apply. There is no shipped guard implementation, no Wintun interface index/lease preflight plumbing, no native DNS/IPv6 full lifecycle, and the existing PowerShell runtime is intact. No OS routes or external NIC modified.
- Tests are table/case driven: full success, replay, concurrent sessions, preexisting foreign route, ambiguous intent interruption, partial create, delete retry, corrupt journal, missing/volatile guard, interrupted arm, canceled context, wrong interface/prefix/gateway/metric, compact whitelist.
- New `.github/workflows/next-adaptive-n4-owned-native-transaction-matrix.yml`: Linux race 4 repeats plus classifier regressions, Windows2022+2025 three repeats with native ABI and Go code, negative scan for any NEW Powershell dependency in source, receipt artifact. Final Action success will be limited to fault-injected orchestration, not real driver or admin/physical traffic.

## Next gates
Build a real non-dynamic/durable WFP + native DNS guard with precise owned filters and DNS capability floor, then real admin Windows hosted with Wintun and bounded physical egress/rollback scenarios; move CLI to new scriptless transaction only after complete parity, and remove active PowerShell script from non-archive code and packaging. No deployment/merge, N6 NOT_STARTED, physical NOT_RUN, N2 efficiencies/tail remain unresolved.
