# ADR-0003: Native DTLS 1.3 security over FakeTCP/FEC

Status: **ACCEPTED; AMENDED BY ADR-0004**

ADR-0003 remains authoritative for the steady-state WBD data-plane security decision. ADR-0004 changes the product-scope statements that previously prohibited any TLS/browser persona work and retires the kernel-anchor experiment from the product roadmap.

## Decision

Use **DTLS 1.3** as the WBD steady-state security layer over the unordered FakeTCP/FEC carrier.

Initial implementation pin:

- repository: `wolfSSL/wolfssl`
- tag: `v5.9.2-stable`
- commit: `ac01707f552c611fbd135cc723b2682b3e7f80f2`
- required capability: DTLS 1.3 client/server with native X.509 peer verification

M2 qualified this path locally.

## Why DTLS remains required

TLS 1.3 assumes a reliable ordered byte stream. Reconstructing one below the weak-network VPN data would recreate head-of-line blocking.

DTLS 1.3 operates on datagrams and therefore preserves the core V2 property: loss of one protected datagram does not force later datagrams to wait for an earlier byte range.

## Layering

```text
WBD IP/session datagram
        ↓
UDPspeeder-compatible FEC
        ↓
source/repair datagram
        ↓
DTLS 1.3 application datagram
        ↓
udp2raw-compatible FakeTCP
        ↓
public network
```

Receive is the reverse.

Every transmitted FEC source/repair shard is independently authenticated. The FEC decoder only sees plaintext that passed DTLS verification.

## Security policy

- DTLS 1.3 only on the mainline.
- 0-RTT disabled initially.
- Native trust-chain + hostname verification is mandatory.
- Optional SPKI pinning may be additive.
- Certificate verification failures are fatal.
- Optional WBD static-token auth occurs only after DTLS Finished.
- Do not invent a second AEAD/key schedule.
- Do not transition the steady-state VPN payload onto an ordinary TLS/TCP byte stream.

## Relationship to TLS Persona

ADR-0004 authorizes an **optional, separate standard TLS 1.3 preflight/bootstrap** with browser-like ClientHello profiles.

This does not supersede DTLS. TLS Persona handles optional connection-establishment appearance/bootstrap metadata; DTLS remains the actual VPN data security authority.

The two roles must remain independently disableable/testable.

## Historical boundary

PR #2 and `docs/benchmarks/m10-004-fec-no-go.md` remain immutable evidence for rejecting ordinary-TCP V1.
