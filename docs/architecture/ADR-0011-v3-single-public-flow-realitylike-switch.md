# ADR-0011: V3 single-public-flow Reality-like setup and in-flow switch

Status: **ACCEPTED — V3 PRODUCT AUTHORITY** (2026-08-31)

Supersedes the V3 product-path decisions in ADR-0004 and ADR-0008 that used a separate ordinary TCP/TLS persona/front followed by a distinct FakeTCP association. Those documents remain historical evidence only where they conflict with this ADR.

## Context

The original product requirement is stricter than logical ticket correlation between two transports: a public observer, NAT, firewall or conntrack device must see one continuous TCP-shaped connection for one WBD session.

The former V2 path created an ordinary TCP/TLS Reality-like bootstrap, closed it, then created a raw FakeTCP association. That produced two public transport identities and also allowed kernel TCP state and raw FakeTCP state to compete on the same public port. Physical Windows/Linux testing and NAT namespace experiments showed that this composition was both semantically wrong for the product requirement and a source of avoidable setup races.

At the same time, carrying sustained VPN payload through an ordinary kernel TCP byte stream is forbidden because loss of an earlier TCP byte can head-of-line block later independent VPN datagrams.

## Decision

One WBD session owns exactly one public raw FakeTCP association from its first SYN until teardown.

```text
one raw SYN / SYN-ACK / ACK
        ↓
same raw FakeTCP sequence space
        ↓
bounded ordered bootstrap stream
        ↓
real TLS 1.3 Reality-like ClientHello / ServerHello / certificate / Finished
        ↓
encrypted username/password admission + fresh session ticket
        ↓
encrypted SWITCH_REQ / SWITCH_ACK barrier
        ↓
destroy bootstrap ordered-stream state
        ↓
same 4-tuple and same FakeTCP association
        ↓
DTLS 1.3 + optional fixed FEC + LINK packet/datagram traffic
```

There is no second public SYN, no second public socket, no FIN/RST/TLS close_notify at the switch boundary, and no kernel-TCP handoff.

## Public ownership

The raw FakeTCP mux is the sole WBD owner of the configured public port. A V3 Linux server must not launch `wbd-reality-front` as a competing kernel TCP listener on that port.

Reality-like parsing, TLS setup, account admission and the encrypted phase switch run as a bounded phase inside the raw mux association. Legacy `wbd-reality-front` code may remain for historical/diagnostic compatibility, but it is not part of the V3 release composition.

## Reality-like fidelity

The setup phase uses real TLS 1.3 grammar and records over the temporary ordered presentation provided by the FakeTCP association. The target is a browser/REALITY-like first-seconds wire image while preserving the one-flow law.

Current requirements are:

- browser-like TCP SYN options on the FakeTCP handshake;
- browser-like/uTLS ClientHello profile where qualified;
- valid SNI and real TLS 1.3 handshake grammar;
- certificate handshake and encrypted account admission;
- no plaintext WBD switch marker on the public wire;
- TLS setup records remain confined to the bootstrap phase.

Exact byte-for-byte equivalence to a particular browser build is not required to block V3 correctness. Fidelity work must never introduce a second connection or move steady-state payload into ordinary TCP.

## No-HOL switch law

Ordered reassembly exists only because TLS requires a byte stream during setup. The encrypted switch ACK is a destructive phase barrier.

After that barrier:

1. ordered bootstrap assemblers are discarded;
2. subsequent units retain datagram boundaries;
3. an earlier lost/reordered steady-state unit must not prevent a later independent DTLS/FEC/LINK datagram from completing;
4. kernel TCP retransmission, congestion-control and receive-queue state are not delivery authorities.

The existing FakeTCP sender/receiver/recovery/FEC steady-state implementation remains frozen unless a deterministic regression proves it must change.

## Windows client composition

The Windows client discovers its Npcap underlay first, then starts one FakeTCP association. Reality-like TLS/auth/switch completes inside that child before DTLS starts. Required readiness order:

```text
single FakeTCP + Reality-like bootstrap/switch ready
  -> DTLS ready
  -> LINK ready
  -> TUN ready
  -> IPv6 fail-closed policy/routes
  -> connected
```

Npcap captures an adapter rather than a socket. The raw backend must therefore reject ARP, IPv6, UDP, wrong 4-tuples, self-captured outbound frames and malformed frames before they reach the strict FakeTCP parser.

## Linux server composition

```text
public WBD_PORT
   ↓
wbd-faketcp-mux  (sole public WBD owner)
   ├─ raw FakeTCP handshake/association table
   ├─ bounded Reality-like TLS 1.3 bootstrap
   ├─ encrypted switch barrier
   └─ inherited DTLS worker after switch
        ↓
127.0.0.1 LINK mux
        ↓
127.0.0.1 platform proxy
```

Firewall lifecycle protects only this WBD-owned public raw path and narrowly suppresses kernel interference required by FakeTCP. Cleanup removes only WBD-owned state.

## Qualification

A V3 release candidate must prove on automated Windows/Linux gates and captured Linux/NAT E2E that:

1. one session emits exactly one public SYN and uses one public 4-tuple;
2. real TLS 1.3 Reality-like setup occurs on that same association;
3. account admission and phase-switch control are encrypted;
4. no second SYN, FIN, RST or TLS close_notify appears at the switch;
5. DTLS 1.3 starts on the same FakeTCP association;
6. bidirectional payload succeeds;
7. deliberate loss of an earlier post-switch packet does not block a later datagram;
8. dirty reconnects cleanly replace stale association state;
9. Windows raw/Npcap boundary tolerates normal adapter noise;
10. Linux official bundle has one public WBD owner and does not launch a V2 Reality kernel listener.

Physical Windows 11 + Npcap + real NIC/NAT/ISP to Linux ARM64 remains the final platform gate. CI must not be presented as a substitute for that physical qualification.

## Consequences

- ADR-0004's separate TLS Persona connection is superseded for V3.
- ADR-0008's `close bootstrap TCP -> FakeTCP` ticket-join flow is superseded for V3.
- Ticket/session identity remains useful inside the same association, but it no longer joins two public flows.
- DTLS 1.3, fixed FEC, LINK/session isolation and the qualified FakeTCP steady-state transport remain retained.
- Legacy dual-flow packaging/workflows must not be treated as V3 release gates.
