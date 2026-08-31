# Architecture v3.0

> **Status: V3 CANDIDATE / HARD INVARIANTS FROZEN.** WBD is a personal weak-network VPN whose public transport is one TCP-shaped raw FakeTCP association. The first bounded phase is real TLS 1.3 / Reality-like setup on that same association; steady state is DTLS 1.3 + FEC + packet/datagram transport with no ordinary-TCP head-of-line blocking.

## Machine-persisted V3 contract

The recovery contract uses these exact phrases to prevent later sessions from drifting back to V2 semantics: **one public raw FakeTCP flow**, **one raw SYN**, **same public 4-tuple**, **Reality-like TLS 1.3 bootstrap**, **encrypted switch**, **DTLS 1.3**, **no-HOL**, and **sole public** WBD owner. These are names for the architecture already specified below, not a second implementation layer.

## Non-negotiable public-flow invariant

For one WBD session, the public network must observe exactly one TCP-shaped flow:

```text
client-ip:client-port  <====================>  server-ip:WBD_PORT
         SYN ... TLS-looking setup ... DTLS/FEC data ... close
```

From the first SYN until session teardown:

- the same client/server 4-tuple is retained;
- FakeTCP owns the raw TCP-shaped sequence space;
- no second Reality/TLS TCP connection is dialed;
- no second SYN is used to enter the data plane;
- no FIN, RST, TLS `close_notify`, or kernel-TCP handoff occurs at the setup-to-data boundary;
- the official server has exactly one public owner for `WBD_PORT`: `wbd-faketcp-mux`;
- an independent kernel `wbd-reality-front :WBD_PORT` listener is forbidden in the V3 product composition.

A ticket/session identity may bind higher layers, but it must never be used to correlate two different public connections because V3 does not create two public connections.

## Public phase model

```text
raw FakeTCP SYN / SYN-ACK / ACK
        ↓
SAME RAW ASSOCIATION / SAME SEQUENCE SPACE
        ↓
bounded ordered bootstrap presentation
        ↓
real TLS 1.3 Reality-like ClientHello/SNI/marker
        ↓
username/password admission + one-time ticket
        ↓
encrypted TLS application-data SWITCH_REQ / SWITCH_ACK
        ↓
NO FIN / NO RST / NO close_notify / NO NEW SYN
        ↓
discard ordered bootstrap assembler
        ↓
DTLS 1.3 datagrams
        ↓
FEC / LINK / VPN packet datagrams
```

The mode-switch controls are encrypted inside TLS 1.3 application-data records. A public observer must not see a plaintext WBD switch magic immediately after the TLS setup.

## Reality-like fidelity

The setup phase must use real TLS 1.3 wire grammar over the caller-owned raw FakeTCP association. It includes a valid ClientHello, SNI, certificate handshake, WBD Reality-like route marker, and authenticated admission.

The design goal is to be as close as practical to an ordinary Reality/browser-style setup for the first few seconds. Fidelity work may improve ClientHello/TCP option fingerprints, extension order, timing, record sizing, or certificate presentation, but it must obey these rules:

1. fidelity code runs over the existing raw association;
2. it must never dial a second public socket;
3. it must never move steady-state VPN payload into ordinary kernel TCP;
4. it must never retain an ordered TLS/TCP stream after the switch barrier.

Current Go `crypto/tls` setup is real TLS 1.3 but is not claimed to be byte-for-byte browser/uTLS fingerprint identical. That is an explicit fidelity gap, not permission to reintroduce a second connection.

## HOL boundary

A short ordered presentation is necessary because TLS is a byte-stream protocol. Therefore bootstrap may temporarily buffer out-of-order FakeTCP payload until contiguous TLS bytes exist.

That ordered assembler is **bootstrap-only**. It is destroyed immediately after the encrypted switch ACK is successfully processed. After that point:

- later datagrams may complete while earlier datagrams are lost or delayed;
- no stream reassembly waits for missing earlier data;
- DTLS records/FEC shards remain independently deliverable/authenticated;
- ordinary TCP retransmission and kernel receive queues never control sustained VPN payload.

Qualification must prove this boundary, not merely infer it from code structure.

## Steady-state stack

```text
TUN / IP packet
        ↓
WBD packet/session layer / LINK
        ↓
release FEC: off or fixed systematic 20:20
        ↓
DTLS 1.3 security wrapper
        ↓
first-arrival FakeTCP datagram carrier
        ↓
same public TCP-shaped 4-tuple
```

DTLS 1.3 is the steady-state security authority. The pinned implementation remains wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`, unless a separately qualified security-lock update replaces it.

## Server composition

The official Linux V3 runtime must launch:

- one raw `wbd-faketcp-mux` public listener;
- Reality-like TLS/auth inside each raw association;
- one DTLS worker per admitted association;
- shared loopback LINK mux;
- shared loopback platform proxy.

`wbd-reality-front` may remain in the repository for legacy/diagnostic comparison, but it is not installed or executed as a public listener by the V3 release bundle.

A future normal-web fallback on the same IP/port must be implemented inside the raw-front ownership model. It must not be implemented by restoring a competing kernel TCP listener on the same port.

## Client composition

Windows V3 performs underlay discovery, then starts the one raw FakeTCP association. Reality-like TLS/auth happens inside that process/association. The ticket produced by that setup is injected into LINK only after the encrypted switch barrier and DTLS readiness chain. No standalone `reality-bootstrap` public connection exists in the V3 product path.

The readiness dependency remains:

```text
FakeTCP raw association + Reality-like setup + switch
        ↓
DTLS READY
        ↓
LINK READY
        ↓
TUN READY
        ↓
IPv6 kill-switch / routes
        ↓
connected
```

Routes must never be applied merely because child processes were spawned.

## FakeTCP semantics

"TCP-shaped" means WBD emits structurally coherent IPv4/TCP packets and owns its raw-lane state. It does not mean application `send()` on a kernel TCP socket, kernel retransmission of sustained product payload, kernel byte-stream delivery, or OS TCP sequence ownership.

The raw carrier may use SYN/ACK/PSH-like packets, ACK progression, bounded recovery, and packet-level retransmission needed for its selected semantics. Steady-state delivery authority remains first-arrival/datagram oriented.

## FEC and DTLS ordering

```text
WBD packet datagram
   ↓
FEC source/repair shard
   ↓
DTLS application datagram
   ↓
FakeTCP raw packet
```

Every DTLS/FEC unit is packet-preserving. Losing one earlier record must not force later independent records to wait. FEC must never reconstruct an ordered aggregate byte stream.

## Qualification requirements

A V3 release candidate is not qualified by unit tests alone. Automated qualification must include a network-namespace/NAT environment that proves all of the following on the same association:

- exactly one public SYN/session 4-tuple;
- real TLS 1.3 Reality-like setup occurs before DTLS;
- switch request/ack plaintext does not appear on the captured public wire;
- no second SYN, FIN, RST, or TLS close-notify is used at the transition;
- DTLS 1.3 becomes ready on the same raw association;
- bidirectional payload succeeds through LINK/echo;
- an intentionally delayed/lost earlier steady-state unit does not block a later datagram;
- repeated reconnect/dirty-exit cases do not revive a second-listener or stale-association design.

Physical Windows/Npcap and real-server tests remain final platform qualification because hosted CI cannot perfectly emulate the Windows packet driver, NIC, home NAT, or ISP middleboxes.

## Platform roles

- Linux/OpenWrt: preferred server/either endpoint; native TUN + privileged raw path.
- Windows 11 x64: portable client; Npcap raw path + Wintun/equivalent.
- Android: out of scope.

## Deferred work

Do not add lanes, new FEC ratios, or other transport complexity until the one-flow V3 invariants and no-HOL qualification remain green. Reality-like fingerprint fidelity may be improved independently as long as it never violates one-flow ownership or steady-state no-HOL semantics.