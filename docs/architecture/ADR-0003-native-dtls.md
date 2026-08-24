# ADR-0003: Native DTLS 1.3 security over FakeTCP/FEC; remove Xray/WireGuard

Status: **ACCEPTED FOR V2.1 MAINLINE**

Supersedes the Xray/WireGuard composition sections of ADR-0002. ADR-0002 remains authoritative for the unordered FakeTCP/FEC carrier decision and the V1 no-go boundary.

## Context

M10-004 rejected ordinary kernel TCP as the public weak-network recovery domain. ADR-0002 therefore moved V2 to udp2raw-compatible FakeTCP + UDPspeeder FEC.

The previous V2 composition still proposed nesting stock Xray/VLESS/Vision/REALITY and WireGuard above that carrier. The project now removes those dependencies and directly implements the useful security/session properties inside WBD.

The desired properties are:

- real server identity based on X.509 certificates;
- standard authenticated key exchange;
- forward-secret traffic keys;
- AEAD protection and anti-replay;
- standard handshake retransmission for an unreliable carrier;
- later key update and session resumption;
- no reliable ordered byte-stream requirement below FEC;
- no custom "TLS-looking" cryptography or detector-specific fingerprint shaping.

## Decision

Use **DTLS 1.3** as the WBD security layer.

Initial implementation pin:

- `wolfSSL/wolfssl`
- tag `v5.9.2-stable`
- commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`
- required capability: DTLS 1.3 client and server with native X.509 peer verification

This release is selected because it has an explicit maintained DTLS 1.3 implementation and portable/custom-I/O support suitable for Linux/OpenWrt and Windows-oriented integration. The exact source/binary is not considered locally qualified until V2-M2 verifies SHA-256 and runs it in the local sandbox/host.

## Why DTLS 1.3 instead of TLS 1.3

TLS 1.3 assumes a reliable ordered stream. Building such a stream over FakeTCP would recreate application-level head-of-line blocking.

DTLS 1.3 operates over datagrams. Independent records can be lost, reordered, retransmitted and anti-replay checked without requiring record N to arrive before record N+1 is processed.

Therefore WBD can retain the FakeTCP/FEC unordered recovery property while reusing a standard TLS-family certificate/key/AEAD protocol.

## Layering decision

V2-M1 remains unchanged and first qualifies:

```text
UDP source
  → UDPspeeder
  → udp2raw FakeTCP
  → network
```

V2-M2 then inserts DTLS **between UDPspeeder and udp2raw**:

```text
WBD / UDP source
        ↓
UDPspeeder FEC encoder
        ↓
source/repair UDP datagrams
        ↓
WBD DTLS 1.3 shim
  one application datagram per FEC source/repair datagram
        ↓
udp2raw FakeTCP
        ↓
network
```

Receive side:

```text
udp2raw
  → DTLS verify/decrypt
  → authenticated UDPspeeder source/repair datagram
  → UDPspeeder decoder
  → WBD / UDP sink
```

This ordering is intentional.

FEC shards are created before DTLS encryption, but every transmitted shard is independently protected by a DTLS application-data record. The FEC decoder receives only shards that have already passed DTLS authentication.

A missing DTLS record does not block later records. A repair shard can therefore arrive, authenticate and participate in recovery even when a source shard is missing.

## Handshake behavior

Initial one-lane connection setup:

1. Establish or make reachable the udp2raw-compatible FakeTCP lane.
2. Run a real DTLS 1.3 handshake through that lane's local datagram interface.
3. Server presents an X.509 certificate for a hostname controlled by the WBD operator.
4. Client verifies trust chain and expected hostname with the DTLS library's native verification path.
5. Optional SPKI pinning may strengthen operator policy but does not replace normal chain/hostname validation by default.
6. After Finished, WBD/UDPspeeder datagrams are carried as DTLS application data.

There is no post-handshake transition to a custom cipher or custom public handshake.

Certificate issuance and renewal may use normal CA/ACME tooling outside the WBD transport in the first product version. WBD only needs secure key/certificate loading, peer validation and explicit reload/restart behavior.

## Initial security policy

- DTLS 1.3 only on the mainline.
- 0-RTT disabled initially because application replay semantics have not been designed.
- Native wolfSSL peer-verification APIs are required; do not use a manual OpenSSL-compatibility `X509_verify_cert` flow for product authentication.
- Certificate verification failures are fatal in product mode.
- Optional username/password/token authentication, if retained, occurs only inside encrypted DTLS application data after Finished.
- Do not invent a second AEAD/key schedule inside WBD.
- Do not tune record length, timing, handshake extensions or raw-lane behavior to imitate a specific browser/service/DPI target.

## Two-lane consequence

If two raw lanes are later admitted, use **two independent DTLS associations**, one per raw 4-tuple:

```text
                UDPspeeder encoded datagrams
                           ↓
                     lane dispatcher
                     /             \
              DTLS association 0   DTLS association 1
                     ↓             ↓
                FakeTCP lane0  FakeTCP lane1
```

On receive, each lane first authenticates/decrypts its records. The resulting UDPspeeder datagrams are merged into one FEC decoder.

This avoids coupling DTLS record-number/anti-replay state to path migration and preserves independent lane failure domains. Both associations may use the same server certificate but derive independent traffic keys.

## Kernel-anchor independence

ADR-0002's kernel TCP anchor / real-return-packet experiment remains separate. The anchor may assist handshake/RST/control behavior for a raw lane, but DTLS/FEC payload is still carried by the raw engine and never by `send()` on the kernel TCP socket.

A kernel-anchor failure must not invalidate the classic udp2raw + DTLS path.

## Xray and WireGuard removal

The V2.1 product path no longer contains:

- Xray;
- VLESS;
- Vision;
- REALITY;
- WireGuard as inner glue.

The valuable properties previously sought from Xray are supplied as follows:

| Desired property | V2.1 mechanism |
| --- | --- |
| server identity | real X.509 certificate in DTLS 1.3 |
| authenticated key exchange | DTLS 1.3 handshake |
| AEAD encryption/integrity | DTLS 1.3 application records |
| anti-replay | DTLS record/epoch logic |
| key update | DTLS/TLS-family key lifecycle, later milestone |
| session resumption | DTLS resumption, later milestone |
| user account auth | optional WBD application frame after DTLS Finished |
| VPN ingress/egress | native WBD TUN/L3 integration, later milestone |

## Admission gates

### V2-M1

Reproduce the exact pinned udp2raw + UDPspeeder one-lane baseline locally before implementing DTLS.

### V2-M2

A DTLS-secured one-lane baseline is admitted only when local tests prove:

- exact pinned wolfSSL source/binary identity;
- successful DTLS 1.3 handshake with a real test certificate chain and hostname verification;
- rejection of invalid/mismatched certificate cases;
- source and repair datagrams both cross as DTLS application data;
- FEC recovery still works when individual DTLS application records are lost;
- no ordered-stream HOL is introduced;
- p50/p95/p99/delivery/CPU/bytes measured at 0/1/5/10/15% impairment;
- security overhead is recorded separately from FEC overhead;
- local execution, not GitHub Actions, is the qualification authority.

Only after V2-M2 passes may the project add native WBD session/account features, kernel-anchor work or two lanes.

## Historical boundary

PR #2 and `docs/benchmarks/m10-004-fec-no-go.md` remain immutable evidence for why ordinary-TCP V1 was abandoned. V2.1 must not recreate a reliable ordered public stream merely to make a TLS library convenient.
