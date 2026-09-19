# ADR-0004: Product-scope clarification — TCP-shaped datagrams, native TUN, optional TLS Persona

Status: **PARTIALLY SUPERSEDED BY ADR-0011** (original 2026-08-25; amended 2026-08-29)

ADR-0004 remains authoritative for retiring the kernel TCP anchor, keeping the sustained WBD data plane packet/datagram-oriented, and treating Xray/REALITY/Vision only as implementation references. Its former decision to use a **separate real TCP/TLS preflight connection** is no longer a valid product shape.

## Preserved decisions

- both endpoints are operator-controlled OpenWrt/Linux/Windows devices with privileges for raw sockets, firewall control, TPROXY/TUN and Npcap/Wintun-class integration;
- the public carrier is WBD-owned TCP-shaped FakeTCP;
- sustained product payload must retain UDP/datagram-like weak-network behavior;
- the local kernel does not own the WBD payload sequence space or ordered delivery;
- kernel TCP anchor/state takeover is not required for product correctness;
- browser/REALITY-like ClientHello engineering may be used to improve setup resemblance;
- WBD does not import VLESS, Xray routing or Vision stream semantics into the data plane.

## Superseded TLS Persona boundary

The V2.2 text described an optional standard TLS 1.3 **preflight connection separate from the FakeTCP/DTLS data lane**. ADR-0011 rejects that network shape because a public observer sees two unrelated connections.

The V2.3 product requirement is instead:

```text
one WBD-owned raw FakeTCP SYN lineage
  -> temporary bounded reliable ordered bootstrap stream
  -> real TLS 1.3 / Reality-like ClientHello recognition and admission
  -> same 4-tuple + same FakeTCP sequence space + no new SYN
  -> DTLS 1.3 / LINK / FEC datagrams
```

Thus the TLS Persona idea is retained, but it is moved **inside the first phase of the single FakeTCP association**.

## Why this still preserves no-HOL transport

`crypto/tls` requires reliable ordered bytes, so the setup adapter may temporarily reorder/retransmit a small bounded amount of handshake traffic. The adapter is destroyed at the mode barrier. Sustained VPN traffic continues as independent DTLS/FEC datagrams and must pass the later-datagram-bypass gate.

No ordinary kernel TCP byte stream is permitted to become the sustained WBD carrier.

## REALITY/browser resemblance

Xray/REALITY and browser/uTLS implementations remain useful references for:

- ClientHello profile/fingerprint behavior;
- TLS extension ordering and GREASE behavior;
- handshake size and fragmentation;
- fallback/decoy handling;
- fail-closed configuration.

The implementation must be judged by packet capture. Real TLS 1.3 on one flow is the first checkpoint; a "99% Reality/browser-like" claim requires measured SYN/TCP-option and ClientHello fingerprint comparison rather than naming alone.

## Certificate policy

The later personal-product decision allowing explicit certificate/hostname verification disablement supersedes this ADR's older mandatory-verification wording. The configured mode must remain explicit; DTLS 1.3 stays the steady-state security authority.

## Consequences

- kernel TCP anchor remains retired;
- native L3/TUN/TPROXY platform work remains valid;
- the separate Persona/preflight public connection is retired;
- ADR-0011 is authoritative for the single-flow setup/data transition;
- optional multi-lane work remains deferred.
