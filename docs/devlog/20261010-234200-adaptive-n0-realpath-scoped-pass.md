# N0 controlled single-client realpath functional PASS; N0 not complete

## Exact source / branch
- Source under test: `b52ed0edad655baaed0411c594f85fe3d82bb79a`, tree `d15936d8de44a56139dae7074c802a5887b42577`, branch `next/adaptive-fec-aes-tun-20261010`.
- This documentation-only STATUS/evidence commit has its own later Git SHA and must **not** be represented as the tested product SOURCE; tested bits are pinned to b52ed0e and Actions GITHUB_SHA. One STATUS, one new devlog, separate evidence.
- Previous parent run source `5f0ebeb9c964bdeff5e8d6cef282885eeb34ba44` [38063769207](https://github.com/lly8666/wobuzhidao/actions/runs/38063769207) remains **FAIL** (functional validator rejected any 300ms RTT-path retransfer; real S2C repair 407B, while every initial unique payload delivered). Historical FAIL and pcap hashes kept.

## Corrected SOURCE Actions receipt
- [next-adaptive-n0-realpath run 38064016024](https://github.com/lly8666/wobuzhidao/actions/runs/38064016024), job 114247879050: **SUCCESS**, artifact 11673829774 (summary, manifest, runner preflight, hashes; no raw pcap or binaries). One scenario/sample/measurement job with guard.
- Real five Linux netns, formal client/server binaries, TLS/uTLS admission V3, FakeTCP AF_PACKET, 300ms netem each direction, OpenWrt/Linux TPROXY, shared server TUN and real UDP socket peers. Server legacy FEC flag 20:20 deliberately conflicts with client protected V3 fixed 20:4, yet both directions deliver authenticated payload from first traffic; tests corroborate per-client policy implementation. This still infers effective parity from implementation/config/arrival; separate independent actual profile/cipher snapshot remains needed.
- C2S and S2C each sent 987 packets / 500080 bytes, delivered 987 first-unique / 500080 bytes, no corruption, duplicate or application send failures. Captured kernel drops zero; qdisc median around 300ms each direction. RTT probes 8/8 with returned-only p99 600636366ns, not end-to-end application latency p99 under stress.
- Product source exact `b52ed0e` [next-foundation run 38064015992](https://github.com/lly8666/wobuzhidao/actions/runs/38064015992): **SUCCESS**; repository contract, Linux/Windows Go tests/build, Linux race/fuzz, p2 fallback and p4 OpenWrt/shared TUN kernel jobs success. This cannot validate Windows direct TUN without native test.
- This second sample observed **zero** TCP outer repair bytes both ways; earlier 407B sample remains separately recorded and cannot be erased to claim a universal zero repair rate.

## Limits and next
- N0 remains IN_PROGRESS. In-memory simultaneous off / fixed4 / Game fixed20 plus standalone one-client netns are not a three-real-client process/netns same-server-port run. Require exact user-mode first S2C-before-C2S (current probe was bidirectional once traffic started), independent on-wire/effective policy/record suite, rotation collisions and negative admission limits; do not generalize this short lossless sample to weaknet, CPU/p99, burst, DNS, lifecycle, IPv6 or MTU.
- No N1 quality feedback, N2 automatic FEC, N3 AES, N4 Windows native direct, N5 GUI/catalog, N6 P6 yet. Physical, release label, and all capacity-dependent performance remain NOT_RUN.
- No local builds/tests (Actions only), auto deployment, merge, secrets or large pcaps; 80-second downlink and multi-second late remain open/deferred by user request.
