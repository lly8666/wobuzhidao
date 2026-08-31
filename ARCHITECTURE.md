# Architecture v2.6

> **Status: ACTIVE MAINLINE DESIGN. ADR-0014 is authoritative for public transport shape.**

## Product intent

WBD is a personal weak-network VPN whose public payload is carried by **exactly one WBD-owned raw TCP-shaped FakeTCP association per connected Logical Tunnel**. The first bounded phase of that same association carries real TLS 1.3 Reality-like setup; after an explicit barrier the same public 4-tuple and FakeTCP sequence lineage carries pinned wolfSSL DTLS 1.3, LINK/FEC and packet/datagram VPN payload without ordinary kernel-TCP HOL.

Logical Tunnel identity and its server-assigned address lease are long-lived logical state. They do not authorize multiple concurrent public transports.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - stable server-assigned IPv4 lease
          - 1 active public WBD transport while connected
          - 0 active public WBD transports while disconnected/dormant
```

## Global single-flow lineage

One connected Logical Tunnel owns exactly one complete public transport:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit barrier; no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC off or fixed 20:20
  -> packet/datagram payload
```

The single-flow invariant is **global to the connected Logical Tunnel**, not merely per lane. A second simultaneous WBD public 4-tuple for redundancy, Game mode or planned replacement is a product-architecture violation.

There is no separate ordinary kernel-TCP Reality product connection and no sustained WBD payload over an ordinary TCP byte stream.

### Canonical release-contract wording

- One connected Logical Tunnel has one public FakeTCP association from SYN through Reality-like bootstrap and steady payload.
- The bootstrap-to-payload switch stays on the same association with no second WBD payload SYN.
- During product operation no separate ordinary kernel-TCP WBD Reality/payload connection exists.
- Bootstrap carries real TLS 1.3 ClientHello/ServerHello/Finished on the same FakeTCP sequence space.
- Steady-state qualification verifies post-bootstrap earliest-complete datagram behavior while an earlier FakeTCP range is missing.
- Product transport cardinality is exactly one while connected.
- The weak-link conservative release operating point remains 40 Mbit/s aggregate inner payload.

## Reality-like setup phase

FakeTCP owns the public network flow from the first SYN onward. Its bounded bootstrap adapter temporarily provides the reliable ordered byte-stream behavior TLS needs.

The current product client uses a real TLS 1.3 uTLS Firefox 120 ClientHello persona. Apart from the WBD recognition-compatible SessionID marker required for admission, the implementation preserves the generated browser fingerprint rather than constructing a custom minimal ClientHello.

The setup phase includes configured SNI, real TLS 1.3 handshake and protected account admission. It is bounded in memory/time and ACK-gated. Once admission/ticket/Logical Tunnel binding is complete, an explicit barrier destroys ordered-bootstrap semantics without sending FIN/RST or creating a new WBD SYN.

Do not claim a numeric Reality/browser similarity percentage without a reproducible pcap metric. Fidelity may not be obtained by moving sustained VPN payload onto ordinary kernel TCP.

## No-HOL steady data plane

After the bootstrap barrier:

- DTLS application datagrams are independently authenticated;
- later independently complete payload may progress while an earlier FakeTCP sequence range is missing;
- WBD shadow ACK/SACK/retransmission preserves TCP-shaped outer behavior without imposing ordinary TCP ordered delivery;
- systematic FEC sources are not delayed merely to fill a block;
- release FEC remains `off` or fixed systematic `20:20`.

The mature TCP-like/FakeTCP recovery and FEC core remains frozen unless deterministic qualification proves a lower-layer defect.

## Canonical packet stack

```text
Windows Wintun / OpenWrt TPROXY captured packet
        ↓
raw IP packet
        ↓
Logical Tunnel lease
        ↓
LINK + optional FEC
        ↓
DTLS 1.3 application datagram
        ↓
one WBD FakeTCP association
        ↓
public network
```

## Logical Tunnel identity and lease

Authentication binds the same-flow transport to a Logical Tunnel/device identity. The server-assigned tunnel address lease belongs to that logical identity rather than a disposable FakeTCP/DTLS/LINK process lifetime.

Same-account devices receive different tunnel addresses. Authenticated setup supplies address/prefix/route parameters. Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

Reconnect/dormancy may preserve Logical Tunnel identity and lease, but the product may not overlap old and replacement public WBD associations. A future seamless migration mechanism must preserve one visible public lineage and requires an explicit new product-owner-approved ADR.

## Multipath / Game research status

`internal/gamelane`, multipath and make-before-break implementations may remain in-tree for research or isolated tests. They are **not product public transport** under ADR-0014.

Product policy must reject an active public transport count other than one. `A -> A+B -> B` public overlap is not a release path.

## Linux Windows-raw-IP server data plane

```text
Internet
  <-> one public WBD raw FakeTCP mux port
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel lease/demux
```

The Linux product exposes one public `WBD_PORT` and one raw FakeTCP mux listener. That mux can serve many independent users/Logical Tunnels, but any one connected Logical Tunnel may bind exactly one active public FakeTCP association. Internal LINK/raw-IP services remain private. No parallel ordinary kernel-TCP Reality product listener competes for the public port.

Per-session netns/veth/double NAT remains historical/reference only. VRF/conntrack-zone remains rejected.

## Frozen security and weak-network boundaries

- one WBD-owned raw TCP-shaped FakeTCP association is the public payload carrier per connected Logical Tunnel;
- no ordinary kernel-TCP sustained payload and no TCP-over-TCP HOL;
- wolfSSL DTLS 1.3 remains pinned to `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- FEC release wire remains `off` or fixed `20:20`;
- <=100 Mbit/s weak-link qualification ceiling remains;
- 40 Mbit/s aggregate-inner remains the conservative release operating point.

## Platform requirements

### Windows

- Wintun/TUN raw L3 remains final capture;
- underlay escape is mandatory;
- one connected Logical Tunnel owns exactly one Npcap/FakeTCP public transport child;
- Npcap ingress ignores unrelated ARP/IPv6/UDP/unrelated TCP before FakeTCP parsing;
- IPv6 remains fail-closed throughout the connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state.

### Linux/OpenWrt

- one public WBD port and one raw mux listener;
- scoped WBD-owned firewall/RST suppression only;
- OpenWrt final capture remains TPROXY + policy routing.

## Observability and secrecy

Retain non-secret tunnel/transport correlation IDs, boundary markers and counters without per-packet INFO spam. Credentials/passwords/tickets/resume secrets do not belong in logs.

## Required qualification before artifact delivery

One exact substantive source HEAD must prove:

1. exactly one public WBD SYN lineage for a connected Logical Tunnel;
2. real Firefox120/Reality-like TLS 1.3 bootstrap runs on that same FakeTCP association;
3. no FIN/RST/new WBD SYN across the bootstrap barrier;
4. no separate ordinary kernel-TCP Reality product connection exists;
5. post-bootstrap no-HOL hole-bypass passes;
6. a second simultaneous public transport for the same Logical Tunnel is rejected;
7. distinct Logical Tunnels receive distinct leases and identical inner tuples remain isolated;
8. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
9. lease source spoofing is rejected;
10. FEC `off` and `20:20` remain green;
11. Windows native wire production and Linux consumption of that same wire pass on one source HEAD;
12. Windows hosted runtime/sandbox and Linux raw/netns full-stack gates pass on that source HEAD;
13. Windows portable and Linux amd64/arm64 release artifacts build from that exact substantive source HEAD;
14. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and cleanup before release designation.

## Retired / superseded product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- 1..4 concurrent product public Transport Lanes;
- Game multipath as a product public-transport path;
- public make-before-break overlap `A -> A+B -> B`;
- per-LiveID netns/veth/double-NAT as final raw-IP design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.
