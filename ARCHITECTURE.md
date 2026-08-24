# Architecture v2

> **Status: ACTIVE EXPERIMENTAL MAINLINE (2026-08-24).** V1 multi-ordinary-TCP is permanently rejected by M10-004. The active decision is `docs/architecture/ADR-0002-raw-fec-restart.md`.

## Core stack

The V2 public weak-network carrier is unordered/datagram-like:

```text
upper payload (VPN / WireGuard / Xray-over-private-link / lab UDP)
        ↓
FEC / redundancy
  1.0x / 1.5x / 2.0x
        ↓
raw lane mapper
  lane0 required
  lane1 optional
        ↓
udp2raw-compatible FakeTCP packet carrier
        ↓
public network
```

Unlike V1, product payload is not placed in an ordinary kernel TCP byte stream. A missing raw packet therefore does not block later FEC/source symbols in a kernel TCP receive queue.

## Reference implementation first

The initial implementation is composition, not reinvention:

```text
UDP source
  → UDPspeeder 20230206.0
  → udp2raw 20230206.0 / faketcp
  → network
  → udp2raw
  → UDPspeeder
  → UDP sink
```

Pinned identities live in `deps/oracle-lock.json` and ADR-0002. This exact combination is already locally qualified as the M10-004 upper-bound reference and becomes the V2 baseline.

## Lane 0: classic correctness baseline

Linux/OpenWrt first uses classic udp2raw FakeTCP with privileged raw access and the upstream-required firewall/RST handling. This is the baseline that all hybrid and Windows variants must match for correctness.

No custom WBD session/RBC from V1 sits above this lane.

## Kernel-anchor / real-return-packet experiment

A V2 lane may optionally have two cooperating pieces:

```text
kernel anchor socket                 raw data engine
--------------------                 ---------------
real OS TCP handshake                encrypted/FEC UDP payload
keeps TCP state alive                raw packet capture/injection
helps suppress unwanted RST          unordered delivery
control/return behavior              no kernel payload queue
```

The upstream `easy-faketcp`/`--easy-tcp` dummy-socket mechanism is the reference for this idea.

Important boundary: **the anchor is not the payload carrier**. We do not send protected application bytes with `send()` on the anchor socket. Doing so would restore HOL.

Before this mode is admitted, packet capture must establish:

- stable three-way handshake and 4-tuple ownership;
- no spurious RST/challenge-ACK loop;
- sequence/ACK evolution is internally consistent;
- raw payload can arrive out of order;
- losing one raw payload packet does not delay later raw payload at the receiver;
- kernel retransmission state is not required for product payload recovery.

If those conditions fail, use classic udp2raw behavior instead of forcing the hybrid.

## FEC layer

First fixed profiles:

- 1.0x: no proactive repair;
- 1.5x: mode 0 `20:10`;
- 2.0x: mode 0 `20:20`.

FEC is below any inner reliable TCP/Xray stream. Repair must be able to arrive independently of the missing source shard.

Do not design a new FEC wire format until the upstream baseline and its exact impairment results are reproduced locally.

## Optional two-lane design

Two lanes mean two independent raw sessions / public 4-tuples, not two ordinary TCP streams.

```text
FEC generation
  source/repair symbols
        ↓ deterministic interleaver
   ┌────┴────┐
 lane0      lane1
   ↓          ↓
FakeTCP     FakeTCP
```

Rules:

- total source+repair traffic budget is shared across both lanes;
- source and repair symbols should be spread so one lane-specific stall does not remove an entire repair opportunity;
- receiver merges symbols by FEC generation/symbol identity, not by ordered lane sequence;
- no cross-lane reliable byte stream is created;
- one lane remains default unless two lanes measurably improve p95/p99 under the same total bytes.

Mandatory comparisons include independent loss, same-time correlated loss and burst loss. Since both lanes may share one ISP/radio bottleneck, correlated loss is a first-class failure case.

## Xray and VPN composition

The preferred first full product composition is:

```text
Application / TUN
        ↓
stock Xray client
VLESS + Vision + REALITY
        ↓
TCP to a private tunnel address
        ↓
WireGuard point-to-point IP link
        ↓
UDPspeeder / FEC
        ↓
udp2raw FakeTCP lane(s)
        ↓
public network
        ↓
reverse stack
        ↓
stock Xray server
```

Why this ordering matters: the Xray TCP connection is **inside** a lower virtual link whose public losses can be repaired by FEC. If stock REALITY/Vision/TCP is put back on the public outside, its kernel TCP HOL again becomes the outer recovery domain and recreates V1's structural failure.

WireGuard is only L3 glue in this plan. It is selected first because it is mature and UDP-based and because udp2raw/UDPspeeder have established WireGuard/OpenVPN-style use. If this composition works, do not write a custom TUN stack.

## Platform roles

- OpenWrt/Linux: preferred server or either endpoint; classic raw FakeTCP baseline.
- Linux desktop/server: privileged raw baseline and kernel-anchor research target.
- Windows: client target using upstream multiplatform/easy-faketcp and Npcap; do not assume server mode.
- Android: out of scope for V2.

## What is intentionally removed from V1

V2 does not carry forward as main-path mechanisms:

- ordinary TCP lane pool;
- logical ACK/GAP reinjection above kernel TCP;
- rescue TCP lane;
- V1 RBC state machine;
- FEC above ordered TCP;
- Android/no-root constraints.

Old code remains for evidence and can be reused only when clearly carrier-agnostic.

## Benchmark authority

The M10-004 no-go result is the historical control. V2 must first reproduce the pinned one-lane reference before modifying it.

Core metrics:

- p50/p95/p99 and maximum delivery delay;
- delivery/completion ratio;
- FEC recovered vs unrecovered symbols;
- intentional source/repair bytes;
- raw packet retransmission (should be none at payload layer unless explicitly designed);
- CPU/RAM;
- per-lane packet counts;
- independent vs correlated impairment.

The ultimate comparison is not "looks TCP-like" but whether the unordered raw/FEC path retains low tail latency when packets are lost.
