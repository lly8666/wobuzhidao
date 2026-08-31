# Roadmap

> **Status: V2.6 GLOBAL SINGLE-FLOW / ADR-0014 ACTIVE.** ADR-0014 is authoritative for public transport cardinality, same-flow Reality-like bootstrap and no-HOL steady transport. ADR-0012 remains authoritative only for compatible Logical Tunnel identity and server-assigned lease concepts; its 1..4 public-lane, Game multipath and make-before-break overlap clauses are superseded.

## Milestone map

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | raw FakeTCP + weak-network external baseline | **DONE** |
| V2-M2 | pinned wolfSSL DTLS 1.3 | **DONE** |
| V2-M3 | immutable LINK/control foundation | **DONE AS FOUNDATION** |
| V2-M4 | no-HOL FakeTCP/FEC qualification | **DONE / MUST REMAIN GREEN** |
| V2-M5 | Game/multipath research | **RESEARCH ONLY / NOT PRODUCT PUBLIC TRANSPORT** |
| V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association | **IMPLEMENTED / GLOBAL SINGLE-FLOW PRODUCT PATH** |
| V2-M7 | Windows Wintun raw-L3 + Npcap underlay | **IMPLEMENTED / REQUALIFY ON EXACT HEAD** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED / REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9C | one-active-public-transport admission / lifecycle | **ACTIVE** |
| V2-M9D | payload-idle dormant/wake with zero-or-one public transport | **NEXT** |
| V2-M9E | break-before-replace while preserving Logical Tunnel identity | **NEXT AFTER M9C** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **ACTIVE / BLOCKED UNTIL ALL SAME-HEAD GATES GREEN** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## Frozen product transport model

One connected Logical Tunnel owns exactly **one** public WBD TCP-shaped lineage:

```text
one raw FakeTCP SYN lineage / public 4-tuple / sequence space
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 Reality-like Firefox120 setup / protected admission
  -> explicit barrier: no FIN / RST / new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality product connection. A connected tunnel may not own a simultaneous second public WBD transport for Game mode, redundancy or make-before-break replacement.

The mature TCP-like/FakeTCP recovery/FEC core is frozen unless a deterministic lower-layer qualification isolates a core defect.

## Frozen weak-network/release limits

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- pinned wolfSSL DTLS 1.3;
- FEC `off` or fixed systematic `20:20`;
- no ordinary kernel-TCP sustained WBD payload and no TCP-over-TCP HOL.

## Logical Tunnel identity + lease

```text
Account -> Device/Installation -> Logical Tunnel -> exactly one active public WBD transport while connected
```

Logical Tunnel owns stable TunnelID and server-assigned IPv4 lease. FakeTCP/DTLS/LINK transport state is disposable. Reconnect/dormancy may preserve logical identity and lease, but product transport cardinality remains one while connected and zero while disconnected/dormant.

## Shared Linux TUN + one NAT

```text
Internet
  <-> one WBD-owned raw FakeTCP mux port
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> shared WBD TUN
  <-> lease/tunnel demux
```

Exit gates remain: distinct Logical Tunnels receive distinct leases, identical inner tuples remain isolated, DNS/UDP/TCP pass, source spoofing is rejected, and firewall changes remain WBD-scoped.

## Public transport lifecycle

- Connected: exactly one active public WBD association.
- Disconnected/dormant: zero active public WBD associations.
- Replacement may preserve Logical Tunnel identity/lease but must retire the old public lineage before starting the new one.
- `A -> A+B -> B` public overlap is forbidden under ADR-0014.
- Game Lane / multipath / make-before-break code may remain in-tree only as research/test infrastructure and must not influence product public-transport behavior.

## Reality-like fidelity

The first bounded seconds must use real TLS 1.3 on the same FakeTCP sequence space, with Firefox120 uTLS ClientHello persona, configured SNI and WBD recognition-compatible SessionID marker. Credentials are protected by TLS. Do not claim a numeric `99%` similarity without a reproducible pcap metric.

The bootstrap adapter may be reliable and ordered only until the explicit barrier. After the barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing.

## Platform requirements

### Linux server

- one public `WBD_PORT` and one raw FakeTCP mux listener;
- no parallel ordinary kernel-TCP Reality product listener;
- each connected Logical Tunnel binds exactly one active public association;
- WBD-owned firewall/RST suppression only;
- internal LINK/DTLS/raw-IP services remain private.

### Windows

- Wintun/raw-L3 capture remains;
- underlay escape remains mandatory;
- exactly one Npcap/FakeTCP public child per connected Logical Tunnel;
- Npcap ingress ignores unrelated ARP/IPv6/UDP/unrelated TCP before FakeTCP parsing;
- IPv6 remains fail-closed while connected;
- Disconnect/Exit restores routes, DNS/NRPT, IPv6 and WBD-owned firewall state.

## Final V2-M10 release gate

On one exact substantive `SOURCE_SHA`:

1. exactly one public WBD SYN lineage exists for a connected Logical Tunnel;
2. Firefox120/Reality-like real TLS 1.3 bootstrap runs on that same FakeTCP association;
3. bootstrap -> DTLS transition has no FIN/RST/new WBD SYN and preserves the same 4-tuple / sequence lineage;
4. no separate ordinary kernel-TCP Reality product connection exists;
5. post-bootstrap no-HOL hole-bypass is green;
6. a second simultaneous public transport for the same Logical Tunnel is rejected;
7. distinct leases + shared TUN + one NAT are green;
8. identical inner tuples from two Logical Tunnels remain isolated;
9. source spoofing is rejected;
10. FEC `off` and `20:20` remain green;
11. Windows native-wire production and Linux consumption pass on the same source HEAD;
12. Windows hosted runtime/sandbox and Linux raw/netns full-stack gates pass on that HEAD;
13. Windows portable and Linux amd64/arm64 release artifacts build from that exact HEAD and report the same `SOURCE_SHA`;
14. clean physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS + UDP + TCP and deterministic cleanup.

Until all automated same-head gates are current green, do not hand a new artifact to the physical tester.

## Deferred

- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps;
- any future seamless migration mechanism, which must preserve one visible public lineage and requires a new product-owner-approved ADR.
