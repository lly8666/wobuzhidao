# FEC batch A real run failed before traffic; preserve and isolate adapter

2026-10-09, branch experiment/fec-policy-sequential-20261009, parent c32d9139af2fc8cc9d8e421062a255d05b309a35.

Actions [37948345570](https://github.com/lly8666/wobuzhidao/actions/runs/37948345570), artifact 11624034758: product binaries built, then **all twelve generated scripts returned code 1 within ~0.8s each**. For s01..s12 business input JSON and stage receipts missing; analyzer returned FileNotFoundError/INVALID and ledger invalid. All six off/on pairs incomparable, CPU/recovery/throughput NOT_COLLECTED. No interpretation as FEC failure. Prior static-only preflight [37947894729](https://github.com/lly8666/wobuzhidao/actions/runs/37947894729) was successful but insufficient.

Implemented narrow *real* 15s off/on UDP functional pilot using the same five-netns generated fixture, pinned SOURCE and binary build, on the exact branch config-push workflow (no parallel jobs or loads), **not** a 120s performance result. Adds safe Bash ERR line/exit diagnostics and error categories into compact receipts; stores only log hash, NOT private stdout/stderr, credentials or keys. Batch aborts on first underlying sample error instead of proceeding through 12 invalid cases. Pilot verifies both endpoints emitted traffic, FEC runtime flags and duration/15ms manifest. Legacy strict workflow/300s defaults unchanged; product code untouched.

Next: review actual pilot Actions run and determine precise startup error. If pilot passes then launch new A. Prior E1, Game4 socket loss, 80s S2C OPEN and physical NOT_RUN preserved in STATUS.
