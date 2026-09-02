# V2 transport-only 20:20 RTT/loss characterization

Status: **CURRENT CAMPAIGN**

This campaign intentionally excludes TUN. It measures the carrier/security/FEC stack directly:

```text
UDP application datagrams
  → UDPspeeder mode0 20:20
  → DTLS 1.3
  → udp2raw FakeTCP
  → impaired underlay
  → reverse stack
  → UDP echo
```

## Why TUN is excluded

TUN/OpenWrt/Windows integration is still required for final platform qualification, but it is not needed to answer the current questions:

1. Does the outer carrier remain stable and TCP-shaped across weak networks?
2. Does the inner data path retain datagram/no-HOL behavior?
3. What CPU and memory cost does fixed `20:20` impose as RTT/loss rise?
4. Where are the establishment, delivery and resource cliffs?

Removing TUN prevents driver/routing/MTU implementation differences from contaminating the first transport surface.

## Frozen matrix

- FEC: UDPspeeder mode 0 `20:20` only.
- RTT: `20, 50, 100, 200, 400, 600 ms`.
- Loss: `0, 1, 5, 10, 20, 30, 40%` independently on each direction.
- Seeds: `260825, 260826, 260827` initially.
- Fresh namespace + FakeTCP + DTLS + FEC processes per case.
- Impairment is active before connection establishment.
- Initial workload: 512 application datagrams × 1200 bytes, window 256.

Nominal matrix size: **126 cases**.

## Fixed implementation pins

- udp2raw `20230206.0`, amd64 SHA-256 `c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c`.
- UDPspeeder `20230206.0`, amd64 SHA-256 `f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a`.
- wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
- DTLS shim source must match the previously qualified M2 source SHA. A fresh benchmark binary SHA is recorded separately from the historical qualified binary SHA when the build environment changes.

## TCP-like metrics

The harness captures/parses actual FakeTCP outer packets and records:

- packets by direction;
- SYN / SYN-ACK / ACK / PSH / FIN / RST counts;
- ACK-shaped payload behavior;
- duplicate logical packets;
- sequence/ACK progression diagnostics;
- establishment success/failure;
- pathological RST/control behavior.

"TCP-like" here means the selected FakeTCP carrier remains structurally coherent. It does not mean WBD uses ordinary TCP reliability.

## UDP-like metrics

For each application datagram the harness records send/return timing and sequence identity, then reports:

- delivery ratio;
- p50/p95/p99/max RTT;
- goodput;
- out-of-order completion;
- later-datagram bypass events/rate.

A bypass event is direct evidence that a later datagram completed while an earlier one was still missing/delayed. Sustained bypass under impairment is evidence against an ordered-byte-stream HOL dependency.

## Resource metrics

Sample `/proc` for all product processes and report:

- CPU seconds/ms by udp2raw, UDPspeeder and DTLS;
- total product CPU;
- CPU ms per delivered MiB;
- peak RSS by process/component;
- aggregate peak RSS;
- application payload bytes and observed outer/wire bytes.

CPU and RSS from GitHub-hosted runners are characterization data, not an OpenWrt hardware promise. Later real-device tests must repeat representative points.

## Interpretation

The first matrix is exploratory but frozen. Do not tune during the run.

After aggregation:

1. locate the FakeTCP/DTLS establishment cliff;
2. locate the application delivery/p99 cliff;
3. locate CPU-per-delivered-MiB and RSS inflection points;
4. verify whether RST/control anomalies appear;
5. verify whether UDP-like bypass persists;
6. choose only a few targeted follow-up experiments around those boundaries.

Potential follow-ups: `20:10`, burst loss, MTU, UDPspeeder timing, or a smaller workload/throughput sweep. None should become another full factorial matrix without evidence.

## Automation

- Harness: `scripts/bench_v2_transport_20x20.py`
- Workflow: `.github/workflows/transport-20x20-matrix.yml`
- Final artifact name: `transport-20x20-matrix-final`

The final artifact should contain `summary.csv`, `report.md`, and `receipt.json` with the exact benchmark build manifest.
