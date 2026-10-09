# FEC strategy experiment: real 15s pilot passes, run scoped 12-case A

2026-10-09, independent branch `experiment/fec-policy-sequential-20261009`. Parent `5acd2e81d8f89af7028a744a375e549c8400eeab`. Product SOURCE frozen at `a2db258b436a41fdee98c6c53abec9bab6ce600f`; no product implementation, FEC defaults, shadow repair credit/buffer changes.

## Verified exact Actions

- Historical [37948345570](https://github.com/lly8666/wobuzhidao/actions/runs/37948345570): FAIL, all 12 segments exited before any business; previous evidence and loss of valid measurement preserved.
- Historical [37949316396](https://github.com/lly8666/wobuzhidao/actions/runs/37949316396): FAIL, 15s FEC-off pilot at owned namespace validation; error BASH_ERR code1 line174; not product failure.
- [37949699339](https://github.com/lly8666/wobuzhidao/actions/runs/37949699339): SUCCESS, both actual 15s network UDP pilot cases FEC-off and FEC20:20 returned sample_exit=0, verified product flags, generated 15ms netem and traffic receipts, clean teardown. Only startup/network functional scope; not CPU qualification or full 120s duration.
- Current commit updates only exact `.github/fec-policy-batch.json` pilot -> batch_a nonce6, STATUS and this devlog. This push starts the **same runner single job** 12-case sequence: UDP/TCP/mixed × 0%/1% two-way seeded loss × off/on, alternating orders, 15ms one-way, 120s business +3s drain, 10Mbps each direction combined TCP+UDP budget, no profile/record timing/padding and product compiled once. Each case independent process and owned topology + numerical receipts.

## Interpretation safeguards

First repeated A only qualifies as exploratory screen if every case actually starts, verifies impairment and FEC, has valid delivery/probes/CPU window and owned cleanup; retain scoped failure or NOT_RUN otherwise. Do not infer CPU gains from fewer delivered bytes, or apply formal FEC20:20 staged5205 thresholds to 1% off. No automatic FEC policy change. Other-agent E1 progress and Game4 local socket drops and ~80s S2C OPEN remain in STATUS. Physical NOT_RUN.

## Next

Inspect Action batch logs and compact artifact; compute per-case/per-pair CPU seconds per delivered GiB and per submitted GiB, throughput, loss/delays, probe denominators and p99, HTTP/TCP, byte/PPS and local drop evidence only if observed. If new failure, leave the run/receipt unchanged and update helper in a new commit, no cherry-picking success.
