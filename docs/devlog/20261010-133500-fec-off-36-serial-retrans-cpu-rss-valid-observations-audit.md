# Audited 36-case realpath FEC-off outer retransmission, CPU and RSS/HWM — 2026-10-10

## Verified action and artifact
GitHub Actions [run 38021635895](https://github.com/lly8666/wobuzhidao/actions/runs/38021635895), job **114123695438**, completed **SUCCESS 2026-10-10T05:13:15Z**. Single existing branch experiment/fec-policy-sequential-20261009, source/helper commit `7ad35deabfcd1da8973d9b7da0f4c31dcf0b5255`, product frozen `a2db258b436a41fdee98c6c53abec9bab6ce600f`. The [numeric artifact 11660861035](https://github.com/lly8666/wobuzhidao/actions/runs/38021635895/artifacts/11660861035) has ZIP SHA256 `cac4afede5e563b1882df86a9f11dd00d5139e079cd40dea53fe0c9ba95f8b0c`. Frozen client SHA256 `303495af95d02f8644af6c3d49a45f5c9c093a3fd9e23b307e31dc1ef0079c67` and server `4b165f41862f98cf502549f2db75eb5cf9801ea088bf6b922d8bc9d3c46af69a` match earlier FEC samples.

Read and audited all `sweep-manifest.json` 36 rows AND per-case `s01..`s36/{case-receipt,summary,retrans-wire,runtime-flags,efficiency-ledger}.json`: **36/36 VALID_SCOPED_REAL_OBSERVATION, 0 NOT_RUN**; all analytic issues empty, sample/analyzer/ledger exit=0, each case owned cleanup clean and no source binary mismatch. Each case truly Normal1, FEC parity0 at BOTH endpoints, outer MTU1400, 120s business +3s impairment drain; delay **50/100/150ms one-way** × fixed netem two-way **1/5/10%** × actual sent **10/20/30/50Mbps per direction**. Ordinary UDP only; case seed scheme on plan. Precisely ONE runner and ONE job, sequential new five netns and teardown for each case, 3s cooldown, no matrix. No product change, no buffer/repair/4096/FEC default change. User requested the all-36 measurement, not product shipping qualification.

**Actual injection:** All 72 direction measurements delivered sender injection >=99% target, measured sent Mbps near 9.9999, 19.9999, 29.9999, 49.9998. Business *goodput* fell substantially under loss; do not mistake requested injection for goodput. All 72 router ingress AF_PACKET observers **zero kernel capture drops, zero hash/seq ciphertext conflicts**, netem qdisc coverage ratio **100.03%–100.13%** (observer window slightly differs at boundaries), realized loss for 1/5/10% as expected, overall **0.964%–10.076%**. Remaining raw circular pcap is NOT a full NIC wire capture; observed outer traffic is pre-netem TCP-shaped IP, not actual physical NIC PPS/byte qualification.

## External DATA replay bytes, not pure ACK

Replay estimate counts same flow, TCP sequence, payload-length and ciphertext digest at router-inbound underlay pre-netem. Independent two-way observed replay payload/fresh payload range and equal-sample arithmetic mean across four rates × three delays (12 cases, 24 directional entries per nominal loss):

| nominal loss | replay/fresh DATA payload min..max | mean replay/fresh payload (equal direction/sample weight) | theoretical geometric p/(1-p) | observed mean / theoretical |
|---:|---:|---:|---:|---:|
| 1% | 0.628%–0.955% | **0.804%** | **1.0101%** | ~79.6% |
| 5% | 2.042%–3.456% | **2.981%** | **5.2632%** | ~56.6% |
| 10% | 2.910%–4.678% | **3.568%** | **11.1111%** | ~32.1% |

Ideal is independent equal-size per-transmission packet erasure with all losses detected and unlimited retries; product only has bounded repair credit/retry time and never promises geometric rate. Counterfactual shortfall does NOT prove whether 4096 shadow backup cap, 3s repair expiry, SACK feedback, CPU host interference, or other cause dominates. At 10%, 20Mbps replay peak ~4.6% versus 50Mbps ~3% suggests *nonmonotonic response to rate*; business UDP still missing, not repaired to perfection.

## Product CPU cost, memory and delivered UDP caveat
Product process **client+server CPU seconds per 120s sample** ranged **50.57s–192.88s** across 36 cases; at 50Mbps CPU seconds per sample were typically ~160–193, at 10Mbps ~51–57. Client process sampled peak VmHWM ~**27.86–36.76 MiB**, server ~**28.25–35.21 MiB**. Distinguish VmRSS versus HWM and client/server from aggregate sum (per-process maxima can occur at different times). Full exact per-case client/server CPU-s, average cores, CPU-s/delivered GiB, peak VmRSS/VmHWM, actual goodput, all-sent probe deadlines, per-size UDP missing, retrans DATA payload and full outer TCP-shaped IP bytes, qdisc realized loss, host CPU PSI and CPU quota are in ZIP `s*/sweep-row.json` and `s*/{summary,efficiency-ledger,retrans-wire}.json`. All 36 samples sent 1195 independent probes, not all returned; within the 36-case suite UDP business missing ranged ~0.55% to ~11.18% depending on impairment/rate. Do not treat observation validity as lossless business success.

Hosted runner identified **AMD EPYC 9V45 96-Core Processor**, 4 logical CPUs; cgroup CPU quota **UNKNOWN**, some CPU PSI avg10 maxima up to **41.9** (strong scheduling pressure differences across segments), so raw CPU comparisons are *descriptive on this runner, not performance qualification or causal comparisons*. External router observer CPU is not included in the product process CPU-s; its host contention can still affect samples. Frozen binaries did not change.

## Immutable history and next
Previous 300ms 1% single forensic run37999822837 and 300ms 5% single run38019925294 stay valid and separate; A 15ms batch all 12 observations, B 300ms batch first four UDP valid, TCP fifth integrity stop and rest NOT_RUN, no false requalification; E1/Game4/E7 original history untouched. Physical / true NIC remained NOT_RUN/NOT_COLLECTED. No 20:4/C batch was run. **The 36-case job is finished; no automatic retransmission or speculative product changes.** Existing automation now disabled.

This audit is a **docs+STATUS** atomic record. The verified Actions artifact contains every detailed per-case numeric value, not an invented aggregate. User-specific xlsx/CSV for interactive filtering are delivered in this conversation and not checked into product source.
