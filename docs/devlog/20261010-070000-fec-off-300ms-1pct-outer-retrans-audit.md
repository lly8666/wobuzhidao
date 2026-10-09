# 300 ms / 1% outer FEC-off real retransmission measurement — 2026-10-10

**Evidence**: https://github.com/lly8666/wobuzhidao/actions/runs/37999822837 SUCCESS, job114054897297, artifact https://github.com/lly8666/wobuzhidao/actions/runs/37999822837/artifacts/11648718654, uploaded archive SHA256 29447ec403d735def9f0de53fe38a892c30d1cec533acd6e4ac39718e451b509. Helper 9e74a5e1b7ef104dd526894f42b2699b4ae86caf. Frozen product SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f (binary sha in artifact). Analysis "VALID_OBSERVATION", zero issues, sample exit 0, owned PID/netns cleanup clean. Detailed compact per-side source counts and derivations: docs/evidence/fec-off-300ms-retrans-37999822837.json. This is one **forensic sample**, not a 300s qualification or physical PASS.

## Actual controlled 120-second realpath sample
Five Linux netns, real TPROXY+FakeTCP+TUN and HTTPS independent pre-existing fixture, no simulator, Normal one lane, 10Mbps logical per direction TOTAL, **UDP business**, FEC off actual --fec-parity=0 at BOTH endpoints, outer MTU1400, seed1910, 300ms ONE-WAY, netem 1% loss on BOTH directions, 120s business+3s impaired drain, profile OFF; same frozen source, no changes to 4096/repair/new product defaults, single Actions job and single sample. Measured qdisc loss c2s 4892/492721 = 0.992854%; s2c 5060/492473 = 1.027467%. Receiver-verified business UDP: c2s missing1633/176280, s2c1604/176280, total **3237/352560**; independent probes missed 6/600 c2s and 3/595 s2c, so combined returned1186/1195; within1s1176/1195; within3s1186/1195.

## Actually measured OUTER TCP-shaped data retransmission
Separate read-only router ingress AF_PACKET header observer rcli(c2s) and rsrv(s2c), filters TCP src/dst 198.18.0.2/198.18.0.6, data non-empty only, dedupe by same TCP seq/flow/payload length AND ciphertext digest, no raw packet/secret saved. Both AF_PACKET kernel dropped=0, no sequence conflict, no IP fragmentation/malformed; c2s 493924 packets vs qdisc attempted492721 (100.244% reference due slight time-edge/observer inclusion), s2c493646 vs492473 (100.238%). Full observer counters are valid for data repeat estimate; standalone legacy rotating pcap analyzer still labels full-window wire bytes NOT_COLLECTED, and these observer totals are NOT physical NIC observation.

| metric | c2s | s2c |
|---|---:|---:|
| first outer DATA packets | 247961 | 247958 |
| repeated outer DATA packets | **1269** | **1435** |
| fresh outer DATA payload bytes | 171215703 | 171215130 |
| repeat outer DATA payload bytes | **895873** | **1012268** |
| retrans / first DATA payload | **0.523242%** | **0.591226%** |
| retrans / all DATA payload | 0.520519% | 0.587751% |
| repeated DATA IP bytes | 946633 | 1069668 |
| **repeat DATA IP bytes / all DATA IP bytes (ACK/control excluded)** | **0.519897%** | **0.587074%** |
| repeat DATA IP bytes / all OUTER IP bytes including ACK/control (supplemental, distinct denominator) | **0.472323%** | **0.533606%** |
| ideal retrans / first payload for independent p=1% geometric | 1.010101% | 1.010101% |
| implied ideal retrans payload bytes (counterfactual, not budget) | 1729452 | 1729446 |
| actual retrans below that theoretical volume | **48.20%** (833579 bytes) | **41.47%** (717178 bytes) |

Combined fresh payload342430833 B, actual replay payload1908141 B, ideal geometric3458897 B: **44.83% below ideal in combined byte accounting**. Ideal p/(1-p) assumes independent equal-size DATA loss, all lost packets discoverable and unlimited repeats until success; it is NOT the required current policy retry rate, nor does it account for bidirectional ACK/control loss or variable sizes. Literal difference in payload rate is 1.010101%-0.523242%=0.486859 percentage points up and 1.010101%-0.591226%=0.418875 down. Full IP byte percentages have a different denominator and must NOT be compared to 1.01% unadjusted. ACK/control traffic was separately observed (c2s 244694 packets / 18339920 IP bytes; s2c 244253 packets / 18257008 IP bytes) and **excluded** from both all-DATA payload and all-DATA IP denominators. Original DATA IP bytes c2s 181134143, s2c 181133450. Business delivery is **incomplete** despite no source/receiver corrupt or malformed payloads reported; this forensic VALID_OBSERVATION must not be interpreted as a full business integrity PASS.

## Why 4096 not proven causal
PRODUCT CODE runtimeowner.MaxOutstandingRecords=4096 is a **retained shadow record cap, not a fresh TCP send window**; when full may evict backup or send fresh with no backup, not automatically block sending. Other independent limitations: repair credit fresh/5, 128KiB cap, 2/4/8x repeat retry cost, 3s horizon, ACK/SACK/RACK feedback and reorder gating. At one-way300ms RTT ~600ms before any queuing, consuming many available feedback/retransmission windows. This sample profile OFF does not publish PeakOutstanding, FreshWindowBypass, RepairMetadataEvicted, RepairDeferred, RepairExpiredSkipped, or per-record retry selections. Therefore **NEITHER '4096 IS TOO SMALL' NOR '4096 HAS NO EFFECT' IS ESTABLISHED**. The observed actual repeat deficit establishes a quantity, not exact reason. Next smallest causal diagnostic would read existing transport counters under one separately labelled diagnostic-on 120s real case (not comparable CPU), without modifying product default or buffer; do not enlarge 4096 on hypothesis alone.

Prior A 15ms 12/12 observations, B 300ms s01–s04 valid/s05 TCP integrity stop and s06–12 NOT_RUN stay unchanged; mainline E1/Game4/E7 history untouched. This audit updates docs only and deliberately does not trigger a new test.
