# N0 V3 realpath fixed4 functional adapter — one sample, Actions pending

## Resume and exact provenance
- Working branch: next/adaptive-fec-aes-tun-20261010, verified parent HEAD `7aad939c81db1d7b90cf48ba0238d825a0e314dd` (Git tree `183c552239d85f4960d043bcb86bffaf954aa094`). Exact new source is the SHA of this devlog's own commit, known only after Git hash creation and printed by GitHub and Actions `GITHUB_SHA`; no self-referential false SHA.
- `docs/STATUS.json` is the unique active source of next_task. Earlier archive/history are not instructions.
- Parent N0 source 7aad Actions: next-foundation [38062107532](https://github.com/lly8666/wobuzhidao/actions/runs/38062107532) **SUCCESS** (repository-contract, Linux/Windows active Go test/build, Linux race/fuzz, p2 kernel fallback and Linux TUN/OpenWrt); next-lifecycle [38062107479](https://github.com/lly8666/wobuzhidao/actions/runs/38062107479) core **SUCCESS**. Includes three-client same-port in-memory segment carrier with TLS/uTLS + FakeTCP + runtime FEC + logical router. This is not a privileged real-netns three-client/Windows driver result.
- Prior failed first-source/receipt-contract Actions remain FAIL and recorded under separate evidence, never rewritten.

## This commit
- Dedicated branch-bounded single-job push-trigger workflow `.github/workflows/next-adaptive-n0-realpath.yml`. Exactly one SOURCE, config, seed, lossless 300ms scenario, and one sample-claim, no matrix, no source cross-ABBA.
- `tools/adaptive_n0_realpath.py` derives a deterministic five-netns script from the existing official calibration template with checked anchors: **client V3 Normal fixed 20:4** while the **server legacy configured FEC=20:20**, thereby testing that each lane's protected V3 profile overrides global settings from the first business records in both directions. The original calibration script and historical analyzer remain unchanged; derived script/template SHA256 captured.
- Dedicated new validator reads actual formal-binary Biz/Target UDP/hash/unique delivery, raw TCP capture underlay, qdisc delay, process liveness, injected counts, capture drop logs, mismatched legacy-parity configuration. Original generic calibration analyzer is not used to mislabel a V3 20:4 result as 20:20. No extra per-packet record bytes or instrumentation added to product.
- Preflight saves runner CPU/quota/PSI when available, but without in-sample product CPU accounting there is no throughput/CPU PASS. Captures hashed and deleted after independent parsing; upload patterns also exclude pcaps, executables and keys even when validation fails.

## Strict limits and next
- **NOT_RUN_PENDING_ACTIONS** on new SOURCE when authored. One Normal client; do not pretend 3 real-netns clients, Windows Wintun/native direct, AES, auto, quality, GUI, native P6, loss bursts or p99 qualification.
- This is a finite functional test only; 8s 0.5Mbps UDP lossless, 300ms one-way each direction. Deliberately require no corrupt/loss under controlled zero-netem-loss in this case; it does not impose zero loss on future genuine weak-net scenarios. The qdisc/capture criteria are narrowly scoped to this existing calibration test; no old checker changed.
- Actual policy and algorithm are inferred from codepath/config plus successful encrypted bidirectional delivery, **not** independently decoded by external pcap; collect protected actual effective snapshots in subsequent N0 work. Missing sample/CPU capacity evidence cannot be called qualified.
- No local compile/test; no deployment, merge, remote devices, secrets, large pcap or qualification ref updates. N0 remains IN_PROGRESS; N1..N6 NOT_STARTED. Approx 80s S2C and historic multi-second late remain OPEN_DEFERRED.
