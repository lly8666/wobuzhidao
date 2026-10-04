# Architecture v2.2

> **Status: ACTIVE MAINLINE.** V1 multi-ordinary-TCP is permanently rejected. V2.2 is a personal OpenWrt/Linux ↔ Linux/Windows VPN with a TCP-shaped FakeTCP carrier, UDP-like data semantics, FEC, DTLS 1.3, native TUN/L3, and an optional browser-like TLS Persona bootstrap.

## Product intent

- the **outer wire packets** should be TCP-shaped enough for the selected FakeTCP carrier;
- the **inner transport behavior** must stay packet/datagram-oriented and avoid ordinary TCP HOL/retransmission;
- both endpoints are operator-controlled and privileged;
- the local kernel does not need to believe the raw payload lane is a genuine TCP byte stream.

Therefore the product does not require a kernel TCP anchor.

## Core stack

```text
TUN / IP packet
        ↓
WBD packet/session layer
        ↓
UDPspeeder-compatible FEC
  normal / 20:10 / 20:20
        ↓
DTLS 1.3 security wrapper
        ↓
udp2raw-compatible FakeTCP raw lane
        ↓
public network
```

The critical property is: **product payload is never committed to an ordinary kernel TCP byte stream on the weak-network path.**

## What "TCP-shaped" means

FakeTCP owns the public packet shape and its own raw-lane state. It may emit SYN/ACK/PSH-shaped packets as required by the selected carrier.

It does not imply application `send()` on a kernel TCP socket, kernel retransmission of product payload, kernel byte-stream receive queues, kernel sequence-space ownership for WBD payload, or a required real OS `ESTABLISHED` payload socket.

Classic udp2raw-compatible FakeTCP remains the product carrier baseline.

## FEC and DTLS ordering

```text
WBD/application packet datagram
   ↓
FEC encoder
   ↓
source/repair shard
   ↓
DTLS application datagram
   ↓
FakeTCP raw carrier
```

Every source/repair shard is independently AEAD-authenticated. Losing one DTLS record must not block later records or repair symbols.

## DTLS security

DTLS 1.3 remains the steady-state security authority: real X.509, native trust-chain/hostname verification, ephemeral key exchange, AEAD, anti-replay, and no custom post-handshake cipher.

Initial pin: wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`. M2 already qualified this implementation path.

## Native WBD packet/session layer

M3 provides HELLO/ACCEPT, optional AUTH/AUTH_OK, PING/PONG, CLOSE/reconnect, stats and fixed protection CONFIG/CONFIG_OK. Data framing remains bounded and packet-preserving; it must never reconstruct an ordered byte stream.

## Qualification split

The product architecture deliberately separates **transport semantics** from **platform ingress/egress**.

### A. Transport-only characterization — current priority

```text
packet/datagram generator
  → UDPspeeder mode0 20:20
  → DTLS 1.3
  → FakeTCP
  → impaired underlay
  → FakeTCP
  → DTLS 1.3
  → FEC decode
  → packet/datagram echo
```

TUN is intentionally absent from this campaign. That makes CPU/RSS, loss tolerance and latency measurements attributable to the carrier/security/FEC stack rather than a platform driver.

The first immutable matrix is:

- RTT `20/50/100/200/400/600 ms`;
- symmetric independent random loss `0/1/5/10/20/30/40%` per direction;
- FEC fixed at `20:20`;
- at least three deterministic seeds;
- fresh namespaces and fresh FakeTCP/DTLS/FEC association for every case;
- impairment installed before connection establishment.

The matrix must measure two architectural properties together.

**TCP-like outer:** real IPv4/TCP-shaped FakeTCP packets remain structurally coherent across impairment. Capture/parse flags, SYN/SYN-ACK/ACK behavior, RST/FIN, duplicate packets and sequence/ACK progression. This is not a claim of ordinary TCP semantics.

**UDP-like inner:** later independent datagrams may complete while earlier datagrams are lost/delayed; delivery must not inherit an ordered byte-stream HOL dependency. Record delivery, out-of-order/later-datagram bypass evidence, p50/p95/p99/max and goodput.

Resource accounting is mandatory: per-component CPU, total CPU, CPU per delivered MiB, per-component peak RSS, aggregate peak RSS, and wire bytes.

### B. Platform qualification — later / external real-device work

Real TUN/OpenWrt/Linux/Windows testing validates TUN/Wintun/Npcap integration, MTU, routes, firewall rules and device-specific resources. A platform failure does not silently change the already-measured transport semantics.

## Optional TLS Persona bootstrap

TLS Persona is independent from the data lane.

```text
client
  ├─ optional standard TCP/TLS 1.3 preflight
  │    ├─ operator-controlled SNI/certificate
  │    ├─ browser-like ClientHello profile
  │    └─ optional bounded bootstrap binding
  └─ WBD FakeTCP lane
       → DTLS 1.3
       → WBD auth/config
       → steady-state FEC/datagrams
```

Persona must not turn steady-state VPN traffic into an ordered TLS/TCP stream, replace DTLS security, silently weaken verification, or require third-party private keys/certificates.

Xray REALITY/uTLS may be studied for ClientHello profile handling. Vision stream/TLS-in-TLS semantics are not part of the WBD data plane.

## Kernel-anchor research status

The previous kernel TCP anchor / real-return-packet experiment is **retired from the product roadmap**. Historical packet-capture evidence may remain, but no further product work is required.

## Optional two-lane design

Two lanes remain deferred. Admission requires same-total-byte-budget evidence after one-lane characterization. If ever admitted, use one independent DTLS association per raw lane and merge authenticated plaintext FEC datagrams before a shared decoder; never create an ordered aggregate stream.

## Platform roles

- OpenWrt/Linux: preferred server/either endpoint; native TUN + privileged raw path.
- Linux desktop/server: native TUN + privileged raw path.
- Windows: required later client; Npcap/easy-faketcp-compatible raw path + Wintun/equivalent.
- Android: out of scope.

## Optimization rule

The current transport matrix is deliberately narrow: `20:20` only. Do not add more FEC ratios, MTUs, burst models, timers, Persona profiles or lanes until the first RTT/loss/resource surface identifies where additional experiments are informative.
