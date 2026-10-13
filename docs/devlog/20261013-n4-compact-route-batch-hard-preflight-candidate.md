# N4 strict compact native route key and batch preflight — candidate, 2026-10-13

## Authority and exact parent
Repository `lly8666/wobuzhidao`, branch `next/adaptive-fec-aes-tun-20261010`, expected parent HEAD `dfd00b82d35e66531c6cfa1a947f614b6aefee91` (remote ref verified). Parent `docs/STATUS.json` names N4 crash-durable native Windows Apply/Stop/Recover as next_task. Parent SOURCE includes B650 Wintun owner negative matrix PASS (Action 38353235782), Foundation PASS (38353235766), and doc-only HEAD Foundation 38353589710 SUCCESS; all are prior-source/scope evidence, not evidence for this candidate. No local Go build/test executed by design; GitHub Actions are the sole qualification environment.

## Code changes
- Build `N4BuildCompactNativeRouteKeys(plan,wintunIndex)` from `validateN4CompactOwned`: physical server /32 underlay escape first; only 0.0.0.0/1 and 128.0.0.0/1 plus explicitly configured DNS /32 captures; fixed route metric 5 and all capture next hops 0.0.0.0. Refuse identical physical and Wintun indices or missing adapter.
- Tighten `validateN4CompactOwned` on tunnel lease /32, physical next-hop, alias and declared underlay route kind.
- Reject malformed route batches in `N4OwnedRoutes.ApplyRoutes` *before* creating any persistent journal, arming guard or touching Windows OS. This blocks missing halves, default route /0, CN subnet routes, duplicate prefixes, order inversion, wrong physical interface and gateway, cross-capture interfaces, metrics and multiple escapes.
- New deterministic pure-Go negative/positive test matrix `TestN4CompactNativeRouteKeysPhysicalEscapeBeforeCapture`, `TestN4NativeTxnRejectsNoncompactRouteBatchBeforeGuard`. Existing fake backend still asserts idempotence, interruption, crash intent and foreign-state protection.

## Qualification plan / scope
Push to the named branch is expected to trigger `next-adaptive-n4-owned-native-transaction-matrix` and `next-foundation`; verify their exact new `head_sha`, Windows 2022/2025 tests, Linux -race and final job logs/artifacts. **At commit time: ACTIONS_PENDING; no PASS inferred.** No performance sample, seed, injected loss, helper modification or physical driver tested. No config/CLI/GUI parameter added. This helper is NOT wired into the running Windows client; the old PowerShell path and high route count are still active.

## Safety and follow-up
This step strengthens the precondition for eventual small-route activation but does not provide authenticated adapter creation or durable DNS/lease/MTU/WFP recovery. The caller still must validate a fresh created Wintun handle LUID, install appropriate persistent firewall/DNS guard, and persist rollback intents before mutations. Actions cannot imply real Wintun driver/native admin qualification. N2 C04/C05 efficiency, C10 GE p99 and ~80s S2C residuals remain OPEN. N6 packages NOT_STARTED; physical NOT_RUN. No deploy, mainline merge or qualification-ref movement.
