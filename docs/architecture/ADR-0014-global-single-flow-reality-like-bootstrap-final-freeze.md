# ADR-0014: Global single public flow with Reality-like bootstrap and no-HOL steady state

Status: **ACCEPTED / PRODUCT-OWNER FINAL FREEZE — 2026-08-31**

Supersedes the transport-count, Game/multipath and make-before-break clauses of ADR-0012 and the later withdrawal text in ADR-0013. ADR-0012 remains useful historical/design evidence for Logical Tunnel identity and server-assigned address leases. ADR-0013 remains historical evidence for the global-one-flow experiment. Where either conflicts with this ADR, **ADR-0014 controls**.

## Product-owner requirement

For one connected WBD Logical Tunnel, the public network must observe **exactly one public WBD 4-tuple** and exactly one WBD TCP-shaped connection lineage at a time:

- one client/server 4-tuple;
- one FakeTCP SYN / SYN-ACK / ACK lineage;
- one FakeTCP sequence space;
- no preliminary ordinary kernel-TCP Reality connection;
- no second WBD SYN after Reality-like admission;
- no concurrent second WBD Transport Lane for redundancy, Game mode or planned replacement.

This is a **global tunnel invariant**, not merely a per-lane invariant.

## Same-flow phase model

```text
one raw FakeTCP SYN lineage / public 4-tuple / sequence space
  -> bounded reliable ordered bootstrap adapter on that same FakeTCP association
  -> real TLS 1.3 Reality-like ClientHello / ServerHello / Finished
  -> protected account admission + ticket / Logical Tunnel lease binding
  -> explicit bootstrap barrier, with no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3 on the same FakeTCP association
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload
```

FakeTCP owns the public flow from the first SYN onward. The temporary ordered adapter exists only to satisfy TLS setup semantics and is destroyed at the barrier.

## Reality-like fidelity

The first bounded seconds should resemble ordinary Firefox/Reality-like TLS behavior as closely as reproducibly practical without transferring ownership to ordinary kernel TCP. The current product persona uses a real TLS 1.3 uTLS Firefox 120 ClientHello and preserves the browser fingerprint except for the WBD recognition-compatible session marker required by admission.

A numeric browser/Reality similarity claim is not a release criterion unless backed by a reproducible pcap metric. The hard release criteria are real TLS 1.3, plausible TCP-shaped handshake/options, configured SNI, protected credentials, one public 4-tuple and no second WBD SYN.

## No-HOL requirement

The setup stream may be reliable and ordered because TLS requires it. That property **must end at the bootstrap barrier**. Sustained VPN payload must never depend on ordinary kernel-TCP retransmission or ordered byte-stream delivery.

After the barrier:

- DTLS application datagrams are independently authenticated;
- a later independently complete datagram may progress while an earlier FakeTCP sequence range is missing;
- WBD shadow ACK/SACK/retransmission may preserve plausible TCP-shaped outer behavior without imposing ordinary-TCP HOL;
- systematic FEC sources are not held merely to fill a block.

The mature FakeTCP recovery/FEC core is frozen unless a deterministic qualification proves a defect below this setup/lifecycle layer.

## Logical Tunnel identity

Logical Tunnel identity and its server-assigned IPv4 lease remain useful and may survive reconnect/dormancy. They do **not** authorize simultaneous public transports. Product transport cardinality is exactly one while connected and zero while disconnected/dormant.

Planned path replacement must not use `A -> A+B -> B`. A new public lineage may be created only after the previous WBD public lineage has been retired. Future seamless migration work, if any, must preserve one visible public lineage and requires a new product-owner-approved ADR.

## Game/multipath status

`internal/gamelane`, multipath and make-before-break code may remain as research/test infrastructure, but they are **not product public-transport behavior** under this freeze. Product configuration must reject any active public transport count other than one.

## Platform rules

### Windows

- Wintun raw L3 remains the capture path.
- One connected Logical Tunnel starts exactly one FakeTCP/Npcap public transport child.
- Npcap ingress must ignore unrelated ARP/IPv6/UDP/unrelated TCP noise before FakeTCP parsing.
- IPv6 remains fail-closed for the connected interval until a real IPv6 tunnel path is qualified.
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state.

### Linux

- One public `WBD_PORT` and one raw FakeTCP mux listener remain the server surface.
- The mux may serve many different users/tunnels, but each individual connected Logical Tunnel may bind only one active public transport association.
- There is no parallel ordinary kernel-TCP Reality product listener on the WBD public port.
- WBD firewall changes remain scoped to WBD-owned state.

## Release qualification

Before physical artifact delivery, one exact substantive source HEAD must prove at least:

1. one and only one WBD public SYN lineage for a connected Logical Tunnel;
2. real TLS 1.3 Reality-like bootstrap occurs on that same FakeTCP association;
3. bootstrap -> DTLS transition has no FIN/RST/new WBD SYN and preserves the same 4-tuple / sequence lineage;
4. post-bootstrap no-HOL hole-bypass passes;
5. a second simultaneous public lane for the same Logical Tunnel is rejected;
6. Firefox 120 packet-persona qualification passes;
7. FEC `off` and fixed `20:20` pass;
8. Windows native-wire production and Linux consumption of that wire pass on the same source HEAD;
9. Windows hosted runtime/sandbox qualification and Linux raw/netns full-stack pass;
10. Windows portable bundle and Linux amd64/arm64 server release build from that exact substantive source HEAD;
11. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

## Development discipline

Detailed decisions and failures go under `docs/development/`. Handoff must cite this ADR as authority. Tests or docs that reintroduce 1..4 active public lanes, Game multipath as product behavior, or make-before-break public overlap are architecture regressions.