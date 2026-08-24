# ADR-0002: Restart on unordered FakeTCP + FEC carrier

Status: **ACCEPTED FOR V2 EXPERIMENTAL MAINLINE** (2026-08-24)

## Decision

The M10-004 result permanently rejects the V1 idea of emulating UDP-like weak-network latency above ordinary kernel TCP carriers. V2 changes the carrier assumption.

V2 uses an unordered/datagram data plane:

```text
upper VPN / Xray / test traffic
        ↓
point-to-point datagram or IP link
        ↓
FEC / redundancy layer
        ↓
1 or 2 independent udp2raw-compatible FakeTCP lanes
        ↓
public network
```

The first qualified upstream baseline is pinned to:

- `wangyu-/udp2raw` tag `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63`.
- `wangyu-/UDPspeeder` tag `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454`.
- FEC mode 0; reference ratios `20:10` (1.5x) and `20:20` (2.0x).

These exact versions already beat the rejected V1 real-TCP/FEC design in the repository's M10-004 benchmark and therefore become the V2 reference implementation, not merely an external oracle.

## Platform scope

V2 deliberately drops Android/unprivileged portability as a requirement.

Primary supported topologies are:

1. OpenWrt/Linux ↔ Linux using privileged raw packet access.
2. OpenWrt/Linux server ↔ Windows client using the upstream multiplatform/easy-faketcp path with Npcap where practical.

Windows server mode is not assumed. The upstream multiplatform implementation is client-oriented, so OpenWrt/Linux remains the preferred server side.

## Why this changes the failed assumption

udp2raw FakeTCP is TCP-shaped on the wire but is not an ordered kernel TCP byte stream for payload delivery. Its documented data path supports real-time/out-of-order delivery without kernel TCP retransmission or congestion-control HOL. FEC repair symbols therefore are not forced to wait behind a missing byte in a kernel TCP send/receive stream.

The exact M10-004 repository evidence is the admission gate: at 1% impairment V1 20:20 FEC was already about 250 ms p99, while the pinned udp2raw + UDPspeeder reference stayed about 72 ms p99 and 100% delivery through the tested 15% FEC-shard loss range.

## Kernel TCP anchor / real return-packet experiment

V2 will also investigate a hybrid lane inspired by upstream `easy-faketcp` / `--easy-tcp`:

```text
kernel TCP anchor socket
  - real OS three-way handshake where supported
  - keeps a kernel TCP state/4-tuple alive
  - assists with RST suppression / control-packet behavior

raw unordered data plane
  - carries protected UDP/FEC payload
  - never writes application payload into the kernel TCP byte stream
```

This is an **experiment gate, not an assumed fact**. Upstream code explicitly describes the dummy socket as being used for handshake and blocking RST; that does not prove that kernel-generated ACKs can safely acknowledge arbitrary raw-injected payload sequence space. Before product use, packet-capture tests must prove sequence/ACK consistency, no kernel retransmission/HOL dependency for payload, and no spurious RST/challenge-ACK behavior.

If this hybrid fails, V2 falls back to proven classic udp2raw FakeTCP with the normal privileged firewall/RST handling on Linux/OpenWrt and upstream easy-faketcp behavior on Windows.

## FEC policy

Start with the proven upstream semantics rather than designing a new codec:

- `normal`: no proactive FEC unless benchmarked otherwise.
- `weak-1.5x`: UDPspeeder-compatible `20:10` reference.
- `weak-2x`: UDPspeeder-compatible `20:20` reference.
- `auto`: deferred until fixed modes are qualified on real packet paths.

The first self-built FEC work, if any, must reproduce the pinned reference byte-for-byte or benchmark-equivalently before adding new coding schemes.

## Two-lane option

Two lanes are an optional enhancement, not a default assumption.

Each lane has a distinct public 4-tuple and independent raw connection state. FEC generations are striped/interleaved at symbol granularity so source and repair symbols are distributed across both lanes. Do not create a higher-level ordered stream across lanes.

Admission gate for two lanes:

- compare 1 lane vs 2 lanes under the same total intentional byte budget;
- test independent loss, correlated loss and burst loss;
- require a repeatable p95/p99 improvement;
- keep total intentional overhead at or below 2.0x;
- if two lanes do not help under correlated impairment, keep one lane as default.

## Xray placement

Stock Xray remains useful, but it must sit **above/inside** the V2 protected virtual link rather than becoming the public outer carrier.

Recommended product stack:

```text
TUN / application
      ↓
stock Xray client
VLESS + Vision + REALITY
      ↓
normal TCP connection addressed over a private point-to-point link
      ↓
WireGuard or equivalent minimal L3/UDP link
      ↓
FEC (UDPspeeder baseline)
      ↓
udp2raw FakeTCP lane(s)
      ↓
public network
      ↓
reverse stack
      ↓
stock Xray server
```

This lets lower-layer FEC/raw delivery hide public-path loss before the inner Xray TCP connection sees it. Making stock REALITY/Vision/TCP public-outermost again is not the V2 mainline because that would restore the rejected ordered-TCP HOL bottleneck.

WireGuard is the initial L3 glue candidate because udp2raw + UDPspeeder already has established UDP-VPN usage. A custom TUN/IP layer is not authorized until the simpler composition is benchmarked.

## Engineering boundary

V2 engineering focuses on correctness, packet-state consistency, FEC recovery, latency, platform integration and interoperability. It does not tune packet timing, headers or lane behavior against specific DPI/detector implementations.

## Historical boundary

PR #2 and `docs/benchmarks/m10-004-fec-no-go.md` remain immutable evidence for why ordinary-TCP V1 was abandoned. V2 must not import V1 lane/RBC/reinjection machinery unless a component is carrier-agnostic and independently justified.
