# N2 one-budget mixed-parity decoder: standalone candidate

2026-10-11, parent docs HEAD `ed813022dbb3bd0928910cdad334b0225c2ea418`; qualified preceding PRODUCT SOURCE `98c7c090ef4a7e68531f40e1e11977502207e259` five scoped Actions PASS; exact evidence in `docs/evidence/adaptive-n2-source-shape-provenance-actions-20261011.json`.

## Implementation

- Add constructor `fec.NewAdaptiveBlockDecoder(codec,maxPacketSize,maxBlocks)` to use **ONE** heavy reconstruction budget, completed history and compact registry for all allowed 20:4/8/10/12/16/20 wire profiles.
- Preserve existing `NewBlockDecoderWithParity` immutable fixed profile semantics. Dynamic decoder permits profile variance **between** BlockIDs only; first streaming/final shard seals each block's parity, and wrong-parity shards with the *same* BlockID fail `ErrHeaderMismatch` before modifying active or compact retired state.
- Extra constant-size parity field in active/retired state, no extra heavy cache per profile, no new retransmission/HOL/MTU/KDF/record-wire changes.
- Focused tests create actual `FastBlockEncoder` wire for every profile including shortened min(k,r), inject lost sources and parity in reverse profile order, verify immediate new systematic and late old-profile recovery, shared 2-heavy cap under 100 changing blocks, active/compact mixed-geometry collision fail-closed and existing fixed decoder rejection.
- This decoder is **not yet installed in real `linkdata.FECPath` or production admission**: FEC-off bare LINK strict format dispatch, new-block-only active encoder transition with old 32ms deadline and 3s retirement, directional controller and V3 per-client policy remain work. Auto still `UNSUPPORTED`, physical NOT_RUN.

## Actions

Commit triggers standard GitHub foundation and lifecycle checks; inspect precise new SOURCE jobs, fix any red tests, then record receipts. There is no local Go execution. N2 weaknet/10Mbps latency/CPU and Windows Wintun remain NOT_RUN; archived failure, default-off p99 ~2.73s and S2C ~80s problem unchanged.
