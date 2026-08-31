# ADR-0004: Product-scope clarification — TCP-shaped datagrams, native TUN, optional TLS Persona

> **V3 SUPERSESSION NOTICE (2026-08-31):** ADR-0011 supersedes this ADR's decision to use a separate ordinary TCP/TLS Persona connection. V3 keeps the raw FakeTCP/datagram and no-kernel-TCP principles from this document, but Reality-like TLS setup now runs as the bounded first phase of the **same single public FakeTCP association**. Do not use the separate-connection text below as current product guidance.

Status: **SUPERSEDED IN PART BY ADR-0011 FOR V3**; retained as V2.2 historical evidence.

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

## Decision 3 — historical V2.2 TLS Persona decision (superseded for V3)

V2.2 allowed a separate connection-establishment Persona option using a real standard TLS 1.3 preflight connection. **ADR-0011 replaces this for V3:** browser/Reality-like TLS setup is now carried inside the first bounded phase of the one raw FakeTCP association, and there is no second public connection.

Historical profile names and fingerprint research remain useful (`native`, browser-like `chrome`/`firefox`/`safari`/`edge`), but they must be applied to the in-flow V3 bootstrap rather than a separate socket.

## Why not copy the full Xray/REALITY/Vision stack

Xray/REALITY is useful reference material for uTLS ClientHello handling and bootstrap engineering. Vision mainly optimizes ordered proxy/TLS stream forwarding and is not a match for WBD's packet/FEC/DTLS data plane.

WBD therefore borrows narrowly:

- maintained browser ClientHello profile machinery;
- profile naming/configuration ideas;
- handshake-size/fragmentation awareness;
- fail-closed configuration patterns.

WBD does not add VLESS, Xray routing or Vision stream semantics.

## Security boundary

The Reality-like TLS bootstrap is not counted as the steady-state data-plane security layer. DTLS 1.3 remains the sustained encryption/integrity authority after the encrypted V3 phase switch.

Certificate/hostname policy is controlled by the current V3 product configuration and qualification; historical V2.2 assumptions in this ADR do not override ADR-0011 or the Project Constitution.

## Qualification retained from this ADR

Reality-like fidelity work should still record selected profile, TLS version/cipher/ALPN, ClientHello length/segmentation and behavior under MTU/fragmentation pressure. V3 additionally requires one public SYN/4-tuple and no ordinary-TCP steady-state HOL, as defined by ADR-0011.

## Consequences

- Kernel TCP anchor remains retired.
- Native L3/TUN remains the platform capture direction.
- Multiple raw lanes remain deferred.
- The separate TLS Persona connection described by V2.2 is retired from the V3 product path.
- Browser-like fingerprint work continues only inside the bounded single-flow bootstrap.
