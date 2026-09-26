# M10-004 — WBD FEC no-go decision

Date: 2026-08-24

## Decision

**NO-GO for the current WBD architecture that tries to approach UDP/QUIC weak-network latency by layering recovery/FEC across multiple ordinary ordered TCP carriers.**

The experiment deliberately gave WBD FEC a favorable benchmark-only implementation before any production wire commitment: two independent real kernel TCP lanes, systematic erasure coding over 20 source shards, either 10 parity shards (1.5x) or 20 parity shards (2.0x), no reactive reinjection, no Auto policy and no production-protocol compatibility constraints. The receiver reconstructs a group as soon as any 20 shards arrive.

The codec itself passed exact erasure-recovery tests for both 20:10 and 20:20. The network result still failed the latency goal because parity shards are carried inside the same ordered TCP lanes. Once a lane experiences a retransmission-equivalent hold, parity queued behind that missing TCP sequence cannot arrive early enough to avoid the carrier HOL.

## WBD FEC result

Profile: 50 ms RTT, 200 logical chunks, 256 B/chunk, window 32, 200 ms extra carrier hold per impaired shard, three seeds; table values are seed medians.

| Impairment | 20:10 mean | 20:10 p99 | >100 ms | 20:20 mean | 20:20 p99 | >100 ms |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 0% | 52.1 ms | 52.4 ms | 0% | 52.1 ms | 52.9 ms | 0% |
| 1% | 132.1 ms | 252.0 ms | 40% | 86.9 ms | 250.5 ms | 20% |
| 2% | 166.6 ms | 252.0 ms | 60% | 116.7 ms | 251.8 ms | 40% |
| 3% | 191.8 ms | 252.0 ms | 70% | 161.6 ms | 252.2 ms | 60% |
| 5% | 191.8 ms | 252.3 ms | 70% | 181.5 ms | 252.3 ms | 70% |
| 8% | 191.9 ms | 252.6 ms | 70% | 206.6 ms | 252.5 ms | 80% |
| 10% | 251.7 ms | 252.3 ms | 100% | 211.9 ms | 252.8 ms | 80% |
| 12% | 251.6 ms | 252.0 ms | 100% | 226.8 ms | 252.4 ms | 90% |
| 15% | 251.8 ms | 252.7 ms | 100% | 231.8 ms | 252.2 ms | 90% |

Delivery ratio remained 100%; the failure is latency, not eventual reliability. 20:10 uses 1.5x logical payload bytes and 20:20 uses 2.0x.

## Strong external reference

Pinned reference: udp2raw `20230206.0` standard `--raw-mode faketcp` plus UDPspeeder `20230206.0`, mode 0, `20:20`, timeout 8 ms. The oracle was warmed before measurement and run with three seeds.

- Workflow run: `32735353872`
- Artifact: `9523056681`
- Artifact digest: `sha256:d08830c1f6f774a2f732f252fc65fb13969cc79f8e3bb68917f88f3b4a8261d1`
- udp2raw amd64 SHA-256: `c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c`
- speederv2 amd64 SHA-256: `f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a`

| Loss | mean | p99 | >100 ms | delivery |
| ---: | ---: | ---: | ---: | ---: |
| 0% | 70.7 ms | 71.6 ms | 0% | 100% |
| 1% | 71.0 ms | 71.5 ms | 0% | 100% |
| 2% | 71.0 ms | 71.4 ms | 0% | 100% |
| 3% | 71.0 ms | 71.3 ms | 0% | 100% |
| 5% | 70.9 ms | 71.3 ms | 0% | 100% |
| 8% | 71.1 ms | 71.4 ms | 0% | 100% |
| 10% | 70.8 ms | 71.3 ms | 0% | 100% |
| 12% | 70.8 ms | 71.4 ms | 0% | 100% |
| 15% | 70.7 ms | 71.1 ms | 0% | 100% |

The fault injection layers are not identical: WBD models a retransmission-equivalent TCP carrier hold preserving HOL, while the external oracle drops FEC shards before udp2raw encapsulation. Therefore this is not a final protocol shootout. It is sufficient for the architecture gate because the WBD experiment was specifically testing whether FEC placed above ordered TCP can bypass TCP HOL; it cannot, even at 2.0x.

## Why more lanes or more copies do not rescue the design

The earlier 2x/3x proactive replication ceiling probe already showed the same structural failure. Independent TCP connections create independent HOL domains, but each domain still queues later protection behind an earlier missing TCP sequence. Adding copies reduces the chance that one particular logical item is late but does not remove the ordered-carrier failure mode; at higher impairment several lanes accumulate independent HOL stalls and tail latency becomes unstable.

## Stop rule

Do not continue M11 adaptive tuning, production FEC framing, larger lane counts, 3x replication, REALITY/Vision integration or TUN/platform work on this architecture.

If the project resumes, it must start with a new architecture decision that changes a fundamental assumption, for example a genuinely unordered/datagram-capable carrier or a different product constraint. Re-tuning the current multi-TCP recovery stack is not an authorized continuation.
