# N2 source-scoped closure provenance — Actions SCOPED PASS

Date 2026-10-11. Product SOURCE `98c7c090ef4a7e68531f40e1e11977502207e259`, from previously corrected controller+shape SOURCE `8d8692e`. This is a docs-only receipt; historical red run `38125145784` remains original FAIL and is not rewritten. SOURCE `8a149f4` test oracle originally requested overlap on invalid 257-PN span, test-only correction `8d8692e` passed foundation `38125294807` / lifecycle `38125294826`.

## Concrete implementation and evidence

Real sender-local, source-window-correlated k/r closure witness is now carried in `datapath.WireRecord.FEC`, attached to the original PN only after actual successful single/batch FakeTCP emit, and snapshotted along with the authenticated V4 source report. Counts separately actual full/partial closed groups, max-shard-length bins, source/parity, off and unclassified. A group counts only when its complete k source + actual min(k,R) parity shards are in **that same** successfully emitted source window; cropped groups or unknown emit provenance are NOT treated as k=20 or 0% first-attempt loss. It is read-only in `QualityFirstAttemptBound.Shape`. No wire/admission change or packet-delivery waiting.

All five workflows for exact SOURCE completed successfully:
- [foundation run 38125834451](https://github.com/lly8666/wobuzhidao/actions/runs/38125834451), all 7 enabled jobs PASS; Linux/Windows Go, unit/race, repository checks, Linux kernel/TUN/TPROXY scoped jobs. P5/P6 **SKIPPED**, not PASS. Foundation receipt artifact `11697600923` SHA256 `df2b4f8f024397b62c161b7992e9af39465439af222d28ab3eeccc7312f7d458`.
- [lifecycle 38125834287](https://github.com/lly8666/wobuzhidao/actions/runs/38125834287) core job `114429890595` PASS.
- [N1 codec/race 38125834458](https://github.com/lly8666/wobuzhidao/actions/runs/38125834458) job `114429891216` PASS.
- [N0 same ciphertext 38125834263](https://github.com/lly8666/wobuzhidao/actions/runs/38125834263) job `114429890681` PASS.
- [N0 three-native-client hosted Linux functional 38125834281](https://github.com/lly8666/wobuzhidao/actions/runs/38125834281) job `114429890553` PASS; artifact `11697286577`, SHA256 `287e2a985abb769982d21aecf2a926bfd90a526b74d69b9a2dd425e458d914d6`. Not N2 switch/performance or Windows Wintun.

Full machine-readable run/job/artifact receipt in `docs/evidence/adaptive-n2-source-shape-provenance-actions-20261011.json`. No user-machine deployment, local Go/compilation/tests or new performance run. No comparison across different runner CPU; CPU improvement is not claimed.

## Remaining N2 hard gates

The quality transmitter tracks all KindLINK successful PNs **including parity**, while mixed controller shape count currently represents systematic source count; an incomplete edge group cannot be guessed. Do not feed those mismatched denominators into an auto decision. The first-attempt interval remains a conservative bound; wide/insufficient/stale/overloaded/unknown HOLD. Implement an explicitly justified normalized mixed shape + cohort coverage rule, mixed-parity shared-budget decoder and no-fresh-HOL TX profile switch, per-client protected auto admission + feedback direction, generation/idle and asymmetric live tests. For future performance, one SOURCE/config/seed/scenario/job per Actions and auto MTU; Game fixed only. N3-N6 NOT_STARTED; physical NOT_RUN; N1 historical proof and off-v2 MEASURED_NOT_QUALIFIED unchanged, default off ~2.73s p99/missing and historical ~80s S2C OPEN.
