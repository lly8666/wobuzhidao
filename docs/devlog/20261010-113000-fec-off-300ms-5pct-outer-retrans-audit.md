# FEC OFF real outer retransmission at 300ms one-way and 5% symmetric loss — observed

Date: 2026-10-10. Independent ONE-case five-netns TPROXY/FakeTCP/TUN Actions [#38019925294](https://github.com/lly8666/wobuzhidao/actions/runs/38019925294) **SUCCESS**, job114118440596, [artifact 11657449301](https://github.com/lly8666/wobuzhidao/actions/runs/38019925294/artifacts/11657449301), ZIP SHA256 `3a727365842df6f4c8bde7b9bc77c163981cff04d8149def2becba2b17f83ad5`. Frozen product SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f, helper 941489c37fa4f39b44194496d6e7c71eb32cfc05; exact frozen client/server binary hashes in artifact and structured docs/evidence/fec-off-300ms-5pct-retrans-38019925294.json. All original 1% figures stay in the previous audit.

## Controlled realpath receipt
UDP business only, Normal one lane, **FEC parity0 on both running endpoints**, profile/CPU profiling off, 10Mbps TOTAL per direction, fixed seeded netem **5% BOTH directions**, one-way delay 300ms, 120s effective business+3s drain, no changes to product source, 4096 shadow, repair budgets, MTU1400 or queue. Source/client/server/hash/mode/seed1910 validated; analyzer "VALID_OBSERVATION" zero issues, sample/analyzer/ledger exits 0; all five owned namespace/PID teardown clean, socket/interface extra drops absent. Independent ingress AF_PACKET observer on rcli and rsrv had **ZERO capture drops**, ZERO malformed/fragmentation or ambiguous sequence/ciphertext, observed 492271 packets c2s against qdisc attempted491140 (100.230% edge coverage) and 492125 s2c vs491016 (100.226%). Small >100% due startup/end-window observer vs qdisc time boundary; preserves truthful time-scope caveat. Actual qdisc netem c2s 24801/491140 = **5.04968%**, s2c 24814/491016 = **5.05360%**. This is useful numeric router-ingress outer observation but NOT a physical NIC byte receipt.

## DATA retransmissions (strict same outer flow/sequence/length/ciphertext)

| Metric | c2s | s2c |
|---|---:|---:|
| Original outer TCP-shaped DATA packet count | 247945 | 247948 |
| Repeated outer DATA packet count | **5041** | **4967** |
| Original outer DATA payload bytes | 171212647 | 171213220 |
| Repeated outer DATA payload bytes | **3468597** | **3439080** |
| **Repeat / original DATA payload** | **2.025900%** | **2.008653%** |
| Repeat / ALL data payload | 1.985672% | 1.969101% |
| Repeat DATA IP bytes | 3670237 | 3637760 |
| Repeat DATA IP / all outer IP (including pure ACK/control) | **1.808681%** | **1.792908%** |
| Independent iid 5% geometric ideal repeat / original payload | **5.263158%** | **5.263158%** |
| Actual repeat as percentage of geometric ideal | **38.492%** | **38.164%** |
| Fraction BELOW geometric ideal | **61.508%** | **61.836%** |

Combined original DATA payload 342425867 B; actual repeat DATA payload **6907677 B**; theoretical ideal repeat 18022414 B. Combined repeat/fresh **2.017277%**, **61.672% lower than the ideal required volume**. Whole-IP and DATA-only denominators are different; never compare the "repeat IP / ALL outer IP" figure directly with ideal 5.263% of fresh payload. ACK-only/control account for much of the outer IP packet count.

## Business reliability (avoid mistaking observation validity for business success)
- C2S UDP business missing **9615/176280**, S2C **9373/176280**, BOTH **18988/352560 (5.3869%)**, with complete per-size breakdown in artifact. Large 4068-byte UDP is especially vulnerable to fragmentation; losses c2s3248/17620, s2c3231/17620. No corruption flagged, but substantial residual business loss.
- Independent probes ALL SENT c2s600/s2c595. Never returned c2s28/s2c32 = **60/1195**. Returned within 1s c2s550/s2c533 = **1083/1195**; within 3s 557/542 = **1099/1195**. Also 15/21 **returned after 3s**; don't hide these or never-returned behind survivor p99. Returned-only p99: c2s~3025.83ms, s2c~3018.17ms, not all-sent latency.
- qdisc drops are actual outer packet impairment, NOT product-replay count. Each outer record's backed-up state and explicit Repair* counters were not captured at profile-off. The `4096` shadow cap is a retention limit, not a sender congestion window. **No causal attribution to 4096**, credit, ACK/SACK feedback, 3s horizon, host pressure or physical path established by these aggregate shares.

## Forensic artifact field exception (must disclose)
The 5% run's `s04/retrans-wire.json` has one **legacy theoretical metadata field** `theory_repeat_fraction_all_data_percent: 1.0`, hardcoded from its original 1% test. **It is incorrect for p=5%, where the correct ideal fraction of all transmissions is 5.0%.** The same raw receipt correctly recorded `theory_p=0.05`, `theory_repeats_over_fresh_percent=5.263157894736842`; the verified `retrans-report.json` used that correct geometric ratio and no calculation relied upon the stale field. The audited structured JSON here explicitly corrects the theoretical label and preserves the original artifact, not quietly modifying it. No second test is justified to repair metadata alone. Do not use the stale field in analysis.

## Comparisons and next
Previous 1% single sample run37999822837 gave repeat/fresh c2s **0.523242%**, s2c **0.591226%**, vs ideal1.010101%; new 5% one sample c2s2.025900% s2c2.008653% vs ideal5.263158%. These are two **separate Actions hosted-run VMs**, so compare descriptive directionality, not CPU efficiency or identical host pressure. The higher loss caused more attempted repair but a *larger fraction of ideal left unrepaired*; no proof that increasing 4096 would cure it. Do not run a 12-case C cohort or 20:4 or tweak product default/repair under this authorization. Mainline E1 progress, Game4 socket-drop FAIL, ~80s S2C OPEN, B 300ms TCP integrity stop, physical NOT_RUN all preserved. This is an independent forensic observation, **not a physical PASS or blanket product qualification**.
