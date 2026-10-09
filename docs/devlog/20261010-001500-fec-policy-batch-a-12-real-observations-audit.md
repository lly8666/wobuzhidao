# FEC Batch A scoped audit, 2026-10-10

Evidence: https://github.com/lly8666/wobuzhidao/actions/runs/37952897144 (SUCCESS; job 113895974446); artifact https://github.com/lly8666/wobuzhidao/actions/runs/37952897144/artifacts/11628960240, GitHub digest sha256:5d197709e9b84f1eff137e10c9419e9a768fb98b66cc74db2c34045e788d67d4. Product SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f; helper 4c5bb337f7b45d5b31abc9bb5a67c07796add851. Inspected 12 compact summary/manifest/receipt/ledger/flags, 60 resident SHA256 checks matched; generated shell itself omitted. All 12 VALID_OBSERVATION, exit 0, clean owned namespace/PID teardown. Previous startup/syntax/pilot failures remain FAIL.

One VM/job, AMD EPYC 9V45, 4 visible CPUs; real five-netns TPROXY/FakeTCP/TUN, Normal single lane, 15ms one-way, bidirectional 0%/1% seeded netem, 10Mbps combined TCP+UDP per direction, 120s business+3s drain, profile off. Serial cases: s01/02 UDP0 off/on seed1909; s03/04 UDP1 on/off seed1910; s05/06 TCP0 off/on seed1911; s07/08 TCP1 on/off seed1912; s09/10 mixed0 off/on seed1913; s11/12 mixed1 on/off seed1914.

CPU-s/delivered GiB off/on: UDP0 169.50/251.00 (off 32.47% lower); UDP1 179.76/302.88 (40.65%); TCP0 49.54/115.11 (56.96%); TCP1 59.22/135.92 (56.43%); mixed0 151.72/231.64 (34.50%); mixed1 170.82/276.39 (38.20%). CPU includes both product processes, normalized by receiver-verified bytes. Goodput UDP 9.9951–9.9961Mbps, TCP 9.9762Mbps, mixed 9.9719–9.9723Mbps per direction.

## Delivery, integrity and deadlines

UDP 1% off: s04 8/352560 missing by sizes 96/256/512/1000/1372/4068B = 1/0/0/1/1/5; mixed 1% off s12 14/276578 = 8/1/2/1/1/1. All FEC20:20 cases and all 0% cases had zero UDP missing; no corruption. TCP/mixed each case 124 hash-verified flows/direction, zero mismatches. TCP >10ms write backpressure events s05–s12 (both directions): 14,12,15,11,13,15,12,12. HTTP(S) 160/160; HTTPS certificate verified 80/80.

Independent probes: 600 c2s and 595 s2c per case, 14340/14340 returned and ALL met both 1s and 3s deadlines. Returned-only p99 ms c2s/s2c at 1%: UDP off 75.6/74.7 vs on 52.5/47.3; TCP off 253.4/107.2 vs on 53.9/62.7; mixed off 107.2/80.7 vs on 65.8/66.6. No timeout masking, but no proof of no-HOL. Max process peak RSS MiB off client/server 30.70/30.23, on 37.25/37.00. Socket additional drops and five-netns interface drops all zero.

## Network and environment caveats

Off/on derived server TUN MTU 1329/1273: whole-policy comparison, not equal-MTU microbenchmark. Client/server binary SHA256 fixed across all cases: client 303495af95d02f8644af6c3d49a45f5c9c093a3fd9e23b307e31dc1ef0079c67; server 4b165f41862f98cf502549f2db75eb5cf9801ea088bf6b922d8bc9d3c46af69a. Qdisc attempted outer PPS, NOT physical NIC PPS, about 2.04–2.53x on/off: e.g. UDP0 c2s 3018/6157; TCP1 c2s 2274/5112; mixed1 c2s 3803/8344. Full-window outer IP wire bytes NOT_COLLECTED; physical NIC PPS NOT_COLLECTED. Cgroup CPU quota UNKNOWN, max CPU PSI some avg10 30.51, host steal max 0%. Hosted VM contention and time order can confound. No cross-VM CPU inference or physical qualification.

Decision: six same-job pairs show off lower measured product CPU-s/GiB, BUT 1% off has residual UDP loss and worse probe p99. Not a default-policy recommendation; no product defaults, bounded repair, queue or buffer modified. Next separately authorized one-job serial 300ms one-way cohort NOT_STARTED; 5% and 20:4 only after further evidence. Preserve E1 mainline progress, Game4 socket-drop FAIL, ~80s S2C OPEN, physical NOT_RUN. Docs-only commit must not trigger another workload.
