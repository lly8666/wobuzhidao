# wobuzhidao Project Constitution — V2.6

## Authority

**ADR-0014 is authoritative for public transport shape.** ADR-0012 is retained only for compatible Logical Tunnel identity/address-lease ideas; its 1..4-lane, Game multipath and make-before-break public-overlap clauses are superseded. ADR-0013 is historical evidence.

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- WBD-owned raw TCP-shaped FakeTCP as the one public weak-network transport;
- exactly **one active public WBD 4-tuple / SYN lineage / FakeTCP sequence space per connected Logical Tunnel**;
- a short real-TLS Reality-like bootstrap carried inside that same FakeTCP association;
- no preliminary ordinary kernel-TCP Reality product connection;
- no FIN/RST/new WBD payload SYN between bootstrap and sustained payload;
- UDP/datagram-like sustained payload semantics with no ordinary-kernel-TCP retransmission/HOL dependency;
- pinned wolfSSL DTLS 1.3 for steady-state encryption/integrity/anti-replay;
- optional FEC, release wire `off` or fixed systematic `20:20`;
- a long-lived Logical Tunnel identity with a stable server-assigned address lease;
- OpenWrt transparent capture through TPROXY and Windows capture through Wintun/TUN raw L3.

The <=100 Mbit/s weak-link qualification ceiling and conservative 40 Mbit/s aggregate-inner release operating point remain unchanged unless a later approved ADR explicitly changes them.

## Global single-flow and no-HOL invariants

A connected Logical Tunnel owns exactly one complete public lineage:

```text
raw FakeTCP SYN / SYN-ACK / ACK
  -> bounded reliable ordered bootstrap stream on that same sequence space
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit bootstrap barrier; no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC
  -> packet/datagram VPN payload
```

Canonical release wording:

- One connected Logical Tunnel has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage.
- Reality-like TLS is the first protected payload phase of that same FakeTCP association.
- A temporary reliable ordered adapter is permitted only during bounded TLS/bootstrap.
- After the bootstrap barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing.
- The retired topology `Reality TCP -> close -> new FakeTCP payload SYN` is forbidden.
- A simultaneous second WBD public transport for the same Logical Tunnel is forbidden.
- Ordinary kernel TCP never owns sustained WBD payload.

Non-negotiable properties:

1. FakeTCP owns the public flow from its first SYN onward.
2. Real TLS 1.3 setup occurs inside the FakeTCP association, not in a separate kernel-TCP connection.
3. The temporary reliable/ordered bootstrap adapter is destroyed at the barrier.
4. Sustained DTLS/FEC/LINK payload remains datagram-oriented and does not inherit ordinary TCP HOL.
5. Shadow ACK/SACK/retransmission may preserve plausible TCP-shaped behavior but must not turn the post-barrier path into an ordered byte stream.
6. Systematic FEC sources are not delayed merely to fill a block.
7. The mature TCP-like/FakeTCP recovery/FEC core is frozen unless deterministic qualification proves a lower-layer defect.

## Reality-like bootstrap fidelity

The first bounded seconds should resemble ordinary Firefox/Reality-like TLS behavior as closely as reproducibly practical without giving the public flow to kernel TCP:

- plausible TCP-shaped SYN/options/ACK progression;
- real TLS 1.3 ClientHello/ServerHello/Finished;
- Firefox 120 uTLS ClientHello persona in the current product implementation;
- configured SNI and WBD recognition-compatible SessionID marker;
- credentials only inside TLS;
- bounded memory/time and ACK-gated bootstrap bytes;
- same 4-tuple and FakeTCP sequence lineage across the barrier;
- no second WBD SYN.

Do not claim a numeric browser/Reality similarity percentage unless backed by a reproducible pcap metric. Fidelity may not be achieved by placing sustained payload on ordinary kernel TCP.

## Logical Tunnel identity and lease

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> exactly one active public WBD transport while connected
          -> zero active public transports while disconnected/dormant
```

- username/password authenticates the account, not transport identity;
- the tunnel lease belongs to Logical Tunnel/device identity;
- same-account devices receive distinct tunnel addresses;
- authenticated setup supplies address/prefix/route configuration;
- server ingress rejects raw IPv4 whose source does not equal the tunnel lease.

A reconnect may preserve Logical Tunnel identity/lease, but it must not overlap two public WBD connections. A replacement public lineage starts only after the previous one has been retired unless a future product-owner-approved ADR defines a one-visible-flow migration mechanism.

## Multipath / Game Lane status

Game Lane, multipath and make-before-break implementations may remain as research/test infrastructure. They are **not product public-transport behavior** under ADR-0014.

Product configuration and server admission must reject any active public transport count other than one for a connected Logical Tunnel. `A -> A+B -> B` public overlap is forbidden by the release contract.

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

## Linux server shape

```text
Internet
  <-> one public WBD raw FakeTCP mux port
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel lease/demux
```

The mux may serve many different users/tunnels, but each individual connected Logical Tunnel may have only one active public association. There is no parallel ordinary kernel-TCP Reality product listener on the WBD public port.

Per-LiveID netns/veth/double NAT remains historical/reference only; VRF/conntrack-zone remains rejected.

## Security and release freeze

- pinned wolfSSL remains `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- DTLS 1.3 remains steady-state cryptographic authority;
- 0-RTT remains disabled until replay/resume semantics are explicitly designed;
- FEC release wire remains `off` or fixed `20:20`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- secrets/tickets/passwords must not be logged;
- non-secret boundary markers/counters remain without per-packet INFO spam.

## Platform invariants

### Windows

- Wintun/TUN raw L3 remains the capture path;
- underlay escape is mandatory;
- one connected Logical Tunnel starts exactly one Npcap/FakeTCP public transport child;
- Npcap ingress ignores unrelated ARP/IPv6/UDP/unrelated TCP before FakeTCP parsing;
- IPv6 remains fail-closed for the connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state;
- Npcap licensing/install constraints remain unchanged.

### Linux/OpenWrt

- Linux server exposes one public WBD port and private internal LINK/raw-IP services;
- WBD firewall helpers add/remove only WBD-owned state and never flush the user's ruleset;
- OpenWrt final capture remains TPROXY + policy routing.

## Required qualification before artifact delivery

One exact substantive source HEAD must prove at least:

1. one and only one WBD public SYN lineage for a connected Logical Tunnel;
2. Firefox120/Reality-like real TLS 1.3 bootstrap occurs on that same FakeTCP association;
3. no FIN/RST/new WBD SYN across bootstrap -> DTLS transition;
4. post-bootstrap no-HOL hole-bypass passes;
5. a second simultaneous public transport for the same Logical Tunnel is rejected;
6. FEC `off` and `20:20` pass;
7. Windows native-wire production and Linux consumption of that wire pass on the same source HEAD;
8. Windows hosted runtime/sandbox and Linux raw/netns full-stack pass;
9. distinct Logical Tunnels receive distinct leases and source spoofing is rejected;
10. shared TUN + one host NAT passes DNS-style UDP, generic UDP and TCP;
11. Windows portable bundle and Linux amd64/arm64 server release pass from that exact source HEAD;
12. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

## Development discipline

- ADR-0014 controls public transport cardinality and same-flow semantics.
- Do not reintroduce 1..4 active public lanes, Game multipath as product behavior or make-before-break public overlap without explicit product-owner approval.
- New deterministic failures are fixed at the first broken layer; do not reopen mature FakeTCP recovery without evidence.
- Detailed decisions, failed experiments, exact heads and qualification results belong under `docs/development/` and are summarized in `.wbd/handoff/current.json`.
