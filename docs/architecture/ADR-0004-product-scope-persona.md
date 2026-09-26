# ADR-0004: Product-scope clarification — TCP-shaped datagrams, native TUN, optional TLS Persona

Status: **ACCEPTED FOR V2.2 MAINLINE** (2026-08-25)

## Context

The previous roadmap carried an easy-faketcp-inspired kernel TCP anchor experiment and explicitly prohibited browser-like TLS fingerprint work.

The product requirement is now clarified:

- both endpoints are the operator's own OpenWrt/Linux/Windows devices;
- privileged raw sockets, firewall control, TUN, Npcap/Wintun-class access are acceptable;
- the public carrier only needs to be **TCP-shaped** at the raw packet layer;
- the payload must retain UDP/datagram-like weak-network behavior;
- the local kernel does not need to own or believe the raw payload stream;
- an optional browser-like TLS connection-establishment persona is desirable, but it must not replace the datagram data plane.

## Decision 1 — retire kernel TCP anchor from the product roadmap

The product uses classic udp2raw-compatible FakeTCP semantics.

A real OS TCP socket is not required for:

- payload delivery;
- ACK ownership;
- sequence ownership;
- product correctness.

Previous M4 packet-state research remains useful historical evidence, but no additional kernel-anchor engineering is required.

## Decision 2 — make native L3/TUN the next core milestone

The next product work is the minimum packet-preserving VPN path:

```text
TUN/IP
  → WBD packet framing
  → FEC
  → DTLS 1.3
  → FakeTCP
```

The Linux/OpenWrt path is implemented first, then Windows interoperability.

## Decision 3 — admit optional TLS Persona

WBD may offer a connection-establishment Persona option:

- `off`
- `native`
- browser-like ClientHello profiles such as `chrome`, `firefox`, `safari`, `edge`
- later randomized profiles only after qualification

The initial design is a **real standard TLS 1.3 preflight connection to an operator-controlled TLS endpoint**. It may produce a short-lived bootstrap/session-binding value that is then presented inside the authenticated WBD session.

This is intentionally separate from the FakeTCP/DTLS data lane.

## Why not copy the full Xray/REALITY/Vision stack

Xray/REALITY is useful reference material for uTLS ClientHello handling and bootstrap engineering. Vision mainly optimizes ordered proxy/TLS stream forwarding and is not a match for WBD's packet/FEC/DTLS data plane.

WBD therefore borrows narrowly:

- maintained browser ClientHello profile machinery;
- profile naming/configuration ideas;
- handshake-size/fragmentation awareness;
- fail-closed configuration patterns.

WBD does not add VLESS, Xray routing or Vision stream semantics.

## Security boundary

TLS Persona is not counted as the data-plane security layer.

The product remains secure when Persona is `off` because DTLS 1.3 still authenticates/encrypts the data lane.

Persona must use normal certificate validation against an operator-controlled hostname. It must not require a third-party private key or disable peer verification.

## Qualification

Persona qualification must record:

- selected profile;
- TLS version/cipher/ALPN;
- trust/hostname validation;
- ClientHello byte length;
- number of TCP segments used by the ClientHello;
- handshake p50/p95/p99 and failure rate;
- behavior under MTU/fragmentation pressure.

This requirement exists because browser-like ClientHello implementations can change size across library versions; a profile that fragments unexpectedly may be less robust than a smaller profile.

## Consequences

- V2-M4 kernel-anchor is retired.
- V2-M6 Linux/OpenWrt TUN becomes the current core milestone.
- V2-M5 two-lane remains deferred.
- Optional TLS Persona is developed after the one-lane core path is functional and before broad product hardening.
- Large parameter sweeps happen after core Linux/OpenWrt + Windows interoperability, with Persona included as an optional test dimension.
