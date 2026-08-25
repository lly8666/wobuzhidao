# Architecture v2.2

> **Status: ACTIVE MAINLINE.** V1 multi-ordinary-TCP is permanently rejected. V2.2 is a personal OpenWrt/Linux ↔ Linux/Windows VPN with a TCP-shaped FakeTCP carrier, UDP-like data semantics, FEC, DTLS 1.3, native TUN/L3, and an optional browser-like TLS Persona bootstrap.

## Product intent

The product requirement is intentionally narrower than "be a real TCP implementation":

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

Receive direction reverses the stack:

```text
FakeTCP packet
  → DTLS verification/decryption
  → authenticated FEC source/repair datagram
  → FEC decode/reorder
  → WBD IP packet
  → TUN
```

The critical property is: **product payload is never committed to an ordinary kernel TCP byte stream on the weak-network path.**

## What "TCP-shaped" means

FakeTCP owns the public packet shape and its own raw-lane state. It may emit SYN/ACK/PSH-shaped packets as required by the chosen upstream-compatible carrier implementation.

It does **not** imply:

- application `send()` on a kernel TCP socket;
- kernel TCP retransmission of product payload;
- kernel byte-stream receive queues;
- kernel sequence-space ownership for WBD payload;
- a requirement to preserve a real OS `ESTABLISHED` TCP socket.

Linux/OpenWrt may use privileged raw sockets plus firewall/RST handling. Windows may use Npcap/easy-faketcp or another qualified raw packet path.

## FEC and DTLS ordering

The qualified V2.1 ordering remains:

```text
WBD packet datagram
   ↓
FEC encoder
   ↓
source/repair shard
   ↓
DTLS application datagram
   ↓
FakeTCP raw carrier
```

Every transmitted source/repair shard is independently AEAD-authenticated. Losing one DTLS record does not block later records or repair symbols.

## DTLS security

DTLS 1.3 remains the security authority for the steady-state data lane:

- real X.509 certificate;
- native trust-chain and hostname verification;
- ephemeral key exchange;
- AEAD confidentiality/integrity;
- anti-replay;
- no custom post-handshake cipher.

Initial pin:

- `wolfSSL/wolfssl`
- tag `v5.9.2-stable`
- commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`

M2 already qualified the pinned implementation path.

## Native WBD packet/session layer

M3 already provides the minimum control state:

- HELLO/ACCEPT version negotiation;
- optional AUTH/AUTH_OK static token;
- PING/PONG liveness;
- CLOSE/reconnect policy;
- session statistics;
- fixed protection-mode CONFIG/CONFIG_OK.

For product L3, one decoded WBD data datagram maps to exactly one IP packet. The data framing must be bounded, exact-length and preserve packet boundaries; it must never reconstruct an ordered byte stream.

## Optional TLS Persona bootstrap

TLS Persona is independent from the data lane.

Reference shape:

```text
client
  ├─ optional standard TCP/TLS 1.3 preflight
  │    ├─ operator-controlled SNI/certificate
  │    ├─ browser-like ClientHello profile (optional)
  │    └─ returns/derives a short-lived bootstrap binding
  │
  └─ WBD FakeTCP lane
       → DTLS 1.3
       → WBD auth/config
       → steady-state FEC/IP datagrams
```

Persona goals:

- browser-compatible ClientHello profiles;
- ordinary TLS 1.3 certificate verification;
- optional session/bootstrap binding;
- measurable handshake size/fragmentation/latency.

Persona is not allowed to:

- turn steady-state VPN traffic into an ordered TLS/TCP stream;
- replace DTLS security;
- silently weaken certificate verification;
- require third-party certificates/private keys.

Xray REALITY/uTLS code may be studied for ClientHello profile handling and handshake engineering. Vision's stream/TLS-in-TLS optimization is not part of the WBD data plane because WBD does not carry its VPN data as nested ordered TLS streams.

## Kernel-anchor research status

The previous kernel TCP anchor / real-return-packet experiment is **retired from the product roadmap**.

Historical packet-capture work showed why mixing a real kernel TCP state machine with independently raw-injected payload creates sequence/ACK ownership problems. That research can remain as evidence, but there is no product requirement to make the local kernel accept or acknowledge WBD raw payload as ordinary TCP data.

Classic udp2raw-compatible FakeTCP remains the product carrier baseline.

## Optional two-lane design

Two lanes remain optional. Admission requires a repeatable same-total-byte-budget p95/p99 improvement.

If admitted:

```text
             FEC datagrams
             /           \
       DTLS assoc 0    DTLS assoc 1
           ↓               ↓
      FakeTCP lane0    FakeTCP lane1
```

The receiver authenticates each lane independently and merges plaintext FEC datagrams before one decoder. No ordered aggregate stream is created.

## Platform roles

- OpenWrt/Linux: preferred server or either endpoint; raw sockets/firewall handling + native TUN.
- Linux desktop/server: native TUN and privileged raw path.
- Windows: required client; Npcap/easy-faketcp-compatible raw path + Wintun/equivalent.
- Android: out of scope.

## Qualification philosophy

The product is optimized for weak-network tail latency and complete packet delivery, not for reproducing ordinary TCP reliability semantics.

After the one-lane Linux/OpenWrt and Windows core paths work, run broad sweeps over:

- FEC mode/ratio;
- shard grouping/buffer timing where supported by the pinned upstream;
- MTU;
- loss/burst models;
- RTT;
- DTLS record overhead;
- reconnect/liveness timers;
- optional TLS Persona fingerprint and ClientHello size/fragmentation.

Any optimization must be justified by reproducible measurements.
