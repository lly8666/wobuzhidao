# 5305: same-source FEC20:12 vs20:20 conclusion (Actions-only)

Date: 2026-10-08. Diagnostic branch only: `diagnostic/normal-5305-fec20-counterfactual-20261008`. Main and frozen product `c853935e5356a9bc99d380befb5a8ac8e0c08d97` remain unchanged. Physical machines were NOT tested.

## Test identity

- Control: FEC20:12 explicit screen, Normal one lane, 10Mbps each direction, 5305 pre/stress/post 5/30/5%, seed1508, Actions run [37692499392](https://github.com/lly8666/wobuzhidao/actions/runs/37692499392), summary artifact 11513199128.
- Counterfactual: FEC20:20 formal, identical product SHA, mode, load, seed and netns topology, Actions run [37698070990](https://github.com/lly8666/wobuzhidao/actions/runs/37698070990), summary artifact 11516725946, raw artifact 11516945510. Build worktree explicitly detached at `c853935e`; repository and 5 category gates PASS.

## Results (30% stress, c2s / s2c)

| Metric | 20:12 | 20:20 |
|---|---:|---:|
| Actual injected random loss | 29.857 / 29.935% | 29.838 / 29.912% |
| Unique app packet loss | 6.308 / 6.498% | 0.205 / 0.151% |
| Path-delay-aligned app Mbps | 9.174 / 9.150 | 9.976 / 9.977 |
| Probe stress received | 56/60 | 60/60 |
| Successful-probe p99 | 2178.685ms | 612.790ms |
| Socket and local-link drops | 0 | 0 |
| Sender fresh outer segments in stress | 315883 / 315883 | 394852 / 394878 |
| Sender 'Abandoned' optional repair records | 0 / 0 | 116015 / 116179 |
| Client/server process CPU seconds (whole run) | 66.53 / 66.70 | 83.59 / 83.76 |

## Attribution and residual risk

**Confirmed for this matched controlled workload**: increasing parity strength eliminates most 5305 residual loss and removes the sampled multi-second returned-probe tail without any product implementation changes. The 20:12 profile is near its algebraic limit: for IID 30% erasure a complete 20-source+12-parity block has 5.7724% expected per-source unrecoverable probability; the 20:20 complete-block reference is 0.1301%. These are *FEC-only* timely references, not guaranteed final business loss or arbitrary burst recovery. Actual 20:20 size-class partial blocks retain nonzero loss.

**Resource/algorithm tradeoff**: 20:20 increases outer packet rate. At ~6581 fresh segments/s and path RTT ~600ms, ~3949 records can be in flight, close to 4096 optional repair shadow entries. `Abandoned` counts loss of optional repair backing/expiry/eviction, not 116k lost business datagrams. It is a real optional-recovery reserve pressure worth optimizing if later paired tests show unacceptable CPU, RAM or losses; it is **not evidence of a new business correctness bug** here. Hosted process CPU seconds increased in one sample, not an ARM cost projection. Fresh systematic forwarding/ no HOL preserved, all 5 checks PASS. Repair profile differences cannot be interpreted as independent additive FEC benefit.

**Not closed**: across-seed stochastic tail guarantees, 70-sample/18-sample stress, 1800s/resource soak, physical Windows→Linux/arm64 native AF_PACKET SO_RCVBUF drop cause, inner MTU9000 UDP8973 fragment delays and physical acceptance. Original Windows/arm64 P6 source remains fixed and untouched. No physical run or production release qualification.

Evidence: [`docs/evidence/normal5305-fec12-vs-fec20-c853935e-actions-20261008.json`](https://github.com/lly8666/wobuzhidao/blob/diagnostic/normal-5305-fec20-counterfactual-20261008/docs/evidence/normal5305-fec12-vs-fec20-c853935e-actions-20261008.json).
