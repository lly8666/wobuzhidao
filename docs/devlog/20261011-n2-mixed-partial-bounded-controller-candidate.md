# N2 mixed-partial k/r and bounded source-qualified controller — SOURCE candidate

Date 2026-10-11, branch `next/adaptive-fec-aes-tun-20261010`. Parent HEAD `cd39f5fb7d49c7a5482a6aa29a832ac870eeeb70` and product source `9b196358b464198a325acf2e9c802113757935d2` verified before editing. This candidate commit SHA is the next SOURCE; **no Action verdict is claimed in this candidate**.

## Implementation
- Keep V3 FECAuto/AutoAggressive **UNSUPPORTED**; no production mode/parity switches, no ACK/fresh-path, KDF, peer MTU, 32ms or 3s changes.
- Added finite [3][21] closed-block k/r/size-class stats from real FastBlockEncoder parity closure and SizeClassEncoder group merges, respecting r=min(k,R), not counting a still pending partial.
- Added isolated controller `EvidenceFirstAttemptBounded` input with lower/upper damage, strict same-source closed-group record count, age/generation/sequence/disjoint PN/pressure and max 5 percentage-point interval width gates; rejected inputs never increase the 512-record count or EWMA. Pricing weights actual partial group source counts instead of assumed k=20; aggressive retains ordinary+one rung; bounded last 32 accepted source windows for shape and Wilson upper, not lifetime zero-loss confidence.
- These are **separate evidence primitives**. The present runtime quality ledger reports successfully emitted KindLINK PNs including FEC parity; cumulative encoder block k/r has no per-source-PN cohort attribution. They must NOT be joined as if matching. Production adapter/source correlation, off-mode shape and live profile switching still required. The synthetic qualified-controller tests are not authentic runtime-WAN evidence.

## Verification dispatch / limitations
Go compilation, unit/race, Linux/Windows, native and performance are **Actions only**. Source commit triggers guarded `next-foundation`; record its exact run/job outcome in a subsequent receipt. No local Go/test execution, no performance sample and no physical testing. Tests added for closed full/partial group k/r, real length classes and unchanged immediate systematic / deadlines, mixed model, replay/overlap/wide/pressure/age/generation and normal/aggressive lowering/raising. Previous N2 source9b19635 scoped PASS and historical FAIL remain unchanged.

## Next
1. On Actions failure, fix and re-run with exact new SOURCE; do not mark PASS from a green workflow title alone.
2. Build source-PN-correlated mixed shape witness from **actual successful first-emits** including parity exclusion, block completion/window overlap and off-mode virtual grouping. Without authenticated cohort coverage HOLD.
3. Wire safe block-boundary switching, unified decoder budget, protected per-client admission, directional feedback and N2 realpath single-config waveform cases.
4. Then N3 AES, N4 Windows TUN/direct, N5 Chinese GUI, N6 gated same-SOURCE packages. Physical stays NOT_RUN. FEC-off default-MTU ~2.73s returning probe p99/missing and legacy ~80s S2C/late problems remain OPEN.
