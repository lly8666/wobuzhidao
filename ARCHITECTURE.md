# Architecture v2.1

> **Status: ACTIVE EXPERIMENTAL MAINLINE.** V1 multi-ordinary-TCP is permanently rejected by M10-004. ADR-0002 changed the carrier to unordered FakeTCP/FEC. **ADR-0003 supersedes the old Xray/WireGuard composition and makes WBD itself the DTLS 1.3 security/session owner.**

## Core stack

The V2.1 public weak-network path is:

```text
Application / TUN / lab UDP
        ↓
WBD native packet/session layer
        ↓
UDPspeeder-compatible FEC
  normal / 20:10 / 20:20
        ↓
DTLS 1.3 security wrapper
  one independent DTLS association per raw lane
        ↓
udp2raw-compatible FakeTCP lane(s)
        ↓
public network
```

Receive direction reverses the stack:

```text
FakeTCP packet
  → DTLS record verification/decryption
  → authenticated FEC source/repair datagram
  → FEC decode/reorder
  → WBD packet/session
  → application/TUN
```

The critical property is unchanged from ADR-0002: protected payload is never committed to an ordinary kernel TCP byte stream on the public weak-network path.

## Why DTLS 1.3, not TLS 1.3 over a reconstructed stream

TLS 1.3 assumes a reliable ordered byte stream. Reconstructing such a stream above FakeTCP would recreate head-of-line blocking at the WBD layer.

DTLS 1.3 keeps the TLS 1.3 security model while operating on unordered/lossy datagrams. WBD therefore gets real certificate authentication, ephemeral key exchange, AEAD records, anti-replay and standard key lifecycle without reintroducing a reliable stream below FEC.

WBD does **not** implement a custom protocol that merely imitates TLS records. The security layer is a real DTLS implementation. Detector-specific record sizing, timing, browser fingerprinting or traffic-shaping is outside the engineering scope.

## Pinned DTLS implementation candidate

Initial pin:

- repository: `wolfSSL/wolfssl`
- tag: `v5.9.2-stable`
- commit: `ac01707f552c611fbd135cc723b2682b3e7f80f2`
- required protocol: DTLS 1.3 client and server

The release has native DTLS 1.3 support. Product verification must use wolfSSL's native peer-verification path rather than a manual OpenSSL-compatibility certificate verification path.

The pin is architectural until V2-M2 locally builds and qualifies the exact source/binary SHA. `deps/security-lock.json` records this state.

## Connection establishment

One-lane initial sequence:

```text
1. udp2raw-compatible FakeTCP lane becomes reachable
2. WBD DTLS client sends a real DTLS 1.3 ClientHello through that datagram service
3. WBD DTLS server completes the standard DTLS handshake
4. server supplies a real X.509 certificate for an operator-controlled hostname
5. client validates trust chain + hostname; optional SPKI pin is additive
6. Finished completes; application traffic begins
7. all subsequent WBD/FEC traffic remains DTLS application data
```

There is no "TLS-looking handshake, then switch to custom encryption" state transition.

The first product milestone disables 0-RTT. Resumption and KeyUpdate are admitted only after the full-handshake path and replay semantics are tested.

## Certificate model

The server owns a normal hostname and private key. Certificate issuance can use ordinary CA/ACME tooling outside WBD. WBD loads the resulting key/certificate and sends the certificate chain during DTLS handshake.

Client policy:

- verify peer certificate;
- verify expected hostname;
- reject expired/untrusted/mismatched certificates;
- optionally enforce an operator-configured SPKI pin;
- never use a third-party site's certificate without its private key;
- never disable verification on the product path.

Optional user/account authentication happens only after DTLS Finished, inside encrypted application data.

## FEC placement

V2.1 deliberately places the FEC encoder **before each DTLS application-record encryption**:

```text
WBD packet(s)
   ↓
UDPspeeder / FEC encoder
   ↓
source shard S0
source shard S1
repair shard P0
   ↓ each shard independently
DTLS application datagram
   ↓
FakeTCP
```

Consequences:

- every transmitted source or repair shard is independently AEAD-authenticated;
- FEC generation/symbol metadata is encrypted inside DTLS application data;
- losing one DTLS record does not block verification of later records;
- a repair shard can arrive and be authenticated even when the corresponding source shard was lost;
- the FEC decoder only receives plaintext shards that already passed DTLS authentication.

For the initial composition, UDPspeeder remains unmodified: its encoded UDP datagrams are fed into a local WBD DTLS shim, which emits DTLS datagrams to udp2raw. On receive, the shim decrypts and forwards the original UDPspeeder datagram to the decoder.

DTLS handshake records are not required to use proactive FEC in V2-M2; native DTLS retransmission is the starting behavior. Handshake protection can be benchmarked later without changing application-data semantics.

## One-lane reference composition

```text
UDP/TUN source
    ↓
UDPspeeder client
    ↓ encoded UDP source/repair datagrams
WBD DTLS 1.3 client shim
    ↓ encrypted DTLS datagrams
udp2raw FakeTCP client
    ↓
public network
    ↓
udp2raw server
    ↓
WBD DTLS 1.3 server shim
    ↓ authenticated source/repair datagrams
UDPspeeder server/decoder
    ↓
UDP/TUN sink
```

V2-M1 qualifies the raw/FEC path **without** DTLS first so the known-good baseline remains measurable. V2-M2 inserts the DTLS shim and measures its incremental cost.

## Optional two-lane design

Two lanes are optional and are admitted only after one secured lane is stable.

Each lane is independent:

```text
                 FEC encoder
          source/repair datagrams
                    ↓
              lane dispatcher
              /           \
             /             \
      DTLS association 0   DTLS association 1
             ↓                    ↓
        FakeTCP lane0         FakeTCP lane1
```

Receiver:

```text
FakeTCP0 → DTLS0 ─┐
                  ├→ authenticated datagram merger → one FEC decoder
FakeTCP1 → DTLS1 ─┘
```

Rules:

- independent raw 4-tuples;
- independent DTLS epochs/record-number spaces and keys;
- the same server certificate may authenticate both associations;
- total source+repair byte budget is shared across lanes;
- no ordered stream is constructed across lanes;
- outbound FEC datagrams can initially be deterministic round-robin/interleaved; smarter symbol placement requires benchmark evidence;
- mandatory comparisons include independent loss, same-time correlated loss and burst loss.

## Kernel-anchor / real-return-packet experiment

The ADR-0002 experiment remains valid and is independent of DTLS:

```text
kernel anchor socket                 raw lane engine
--------------------                 ---------------
real OS TCP handshake                DTLS datagram bytes
keeps TCP state/4-tuple alive        raw capture/injection
helps RST/control behavior           unordered payload delivery
```

The anchor is never the application payload stream. Packet-capture qualification must prove sequence/ACK consistency, no spurious RST/challenge-ACK loop, and no kernel retransmission/HOL dependency for DTLS payload packets.

If the hybrid fails, use classic udp2raw FakeTCP behavior rather than forcing it.

## Native WBD session responsibilities

Xray is removed. WBD itself eventually owns only the minimum product functions not already provided by DTLS/FEC:

- configuration/version negotiation inside DTLS application data;
- optional username/password or token authorization after Finished;
- tunnel/session identity and keepalive;
- IP/UDP packet framing and MTU handling;
- FEC mode negotiation (`normal`, `20:10`, `20:20`, later Auto);
- lane health and statistics;
- reconnect/resume policy;
- TUN ingress/egress on Linux/OpenWrt and Windows.

Do not duplicate certificate authentication, AEAD, anti-replay or key derivation in custom WBD crypto.

## Platform roles

- OpenWrt/Linux: preferred server or either endpoint; classic raw FakeTCP baseline.
- Linux desktop/server: privileged raw baseline and kernel-anchor research target.
- Windows: client target using upstream multiplatform/easy-faketcp and Npcap; later Wintun or equivalent for VPN ingress.
- Android: out of scope.

## Removed dependencies and mechanisms

V2.1 does not use on the mainline:

- Xray;
- VLESS / Vision / REALITY;
- WireGuard as an inner transport;
- ordinary TCP lane pools;
- V1 logical ACK/GAP reinjection above kernel TCP;
- V1 rescue TCP lane or RBC state machine;
- FEC above ordered kernel TCP;
- Android/no-root constraints.

Old code/docs remain historical evidence and may be reused only when carrier/security agnostic.

## Benchmark authority

M10-004 remains the historical control. Qualification sequence:

1. V2-M1: reproduce pinned one-lane udp2raw + UDPspeeder without DTLS.
2. V2-M2: insert real DTLS 1.3 and measure incremental latency/CPU/bytes at 0/1/5/10/15% impairment.
3. Only then add WBD native session/account features, kernel-anchor experiments and two lanes.

Core metrics:

- p50/p95/p99 and maximum delivery delay;
- delivery/completion ratio;
- FEC recovered vs unrecovered symbols;
- DTLS handshake time and retransmissions;
- intentional source/repair bytes;
- DTLS and raw framing bytes;
- CPU/RAM;
- per-lane packet counts;
- independent vs correlated impairment.

The success criterion is low weak-network tail latency with real authenticated encryption, not resemblance to any specific third-party traffic fingerprint.
