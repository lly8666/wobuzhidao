# Roadmap

> **Status: V2.4 GLOBAL SINGLE-PUBLIC-FLOW HARDENING ACTIVE.** ADR-0013 controls public transport count/replacement. ADR-0012 retains Logical Tunnel/address-lease/shared-TUN work only where it does not require multipath.

## Milestone map

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | raw FakeTCP + weak-network external baseline | **DONE** |
| V2-M2 | pinned wolfSSL DTLS 1.3 | **DONE** |
| V2-M3 | immutable LINK/session/control foundation | **DONE AS FOUNDATION** |
| V2-M4 | no-HOL FakeTCP/FEC first-arrival qualification | **DONE / MUST REMAIN GREEN** |
| V2-M5 | historical Game Lane first-arrival/race research | **IMPLEMENTED RESEARCH / NOT CURRENT PRODUCT PATH** |
| V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association | **IMPLEMENTED / QUALIFIED FOUNDATION** |
| V2-M7 | Windows Wintun raw-L3 capture/routing + Npcap single-flow underlay | **IMPLEMENTED / EXACT-HEAD REQUALIFICATION REQUIRED** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED / REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9C | global one-public-transport product freeze + duplicate-lane rejection | **ACTIVE** |
| V2-M9D | payload-idle DORMANT/wake with 0-or-1 public transport | **NEXT AFTER M9C GREEN** |
| V2-M9E | break-before-make age/path replacement | **NEXT AFTER M9C GREEN** |
| V2-M10 | exact-source automated Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED ON M9C/M9E QUALIFICATION** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED UNTIL FUNCTIONAL ARCHITECTURE IS GREEN** |

## Frozen global transport invariant

For one connected Logical Tunnel:

```text
public WBD transport count = 1
```

Disconnected/dormant may be zero.

The one public transport is:

```text
one raw FakeTCP SYN lineage
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 / Reality-like marker + admission
  -> SAME 4-tuple / SAME sequence space / NO second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> optional fixed FEC
  -> packet/datagram VPN payload without ordinary-TCP HOL
```

Current release product must not expose 2..4 public lanes, Game/race duplication, or old+candidate overlap. `internal/logicaltunnel.MaxProductPublicTransportLanes` remains `1`.

## Frozen weak-network/release limits

Unless a later explicit benchmark ADR reopens them:

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- pinned wolfSSL DTLS 1.3;
- FEC `off` or fixed systematic `20:20` on the qualified release wire;
- systematic source datagrams are not delayed merely to fill FEC blocks;
- no ordinary kernel-TCP sustained WBD payload path and no TCP-over-TCP HOL dependency.

## V2-M9A — Logical Tunnel + server-assigned unique address

Retained identity hierarchy:

```text
Account -> Device/Installation -> Logical Tunnel -> one current public Transport Epoch
```

A Logical Tunnel owns:

- stable TunnelID while the logical VPN is enabled;
- server-assigned unique IPv4 lease from a configurable pool;
- authenticated route/address configuration;
- zero public transports while dormant/disconnected or one while connected.

LiveID/FakeTCP/DTLS/LINK belong to the disposable transport epoch, not to the IP lease.

Exit gates:

1. two same-account logical tunnels receive different addresses;
2. Windows does not assume every client is `10.66.0.2/30`;
3. authenticated tunnel configuration carries address/prefix/route information;
4. ingress source address must equal the lease;
5. lease cleanup/reconnect/reuse is deterministic;
6. pool is configurable.

## V2-M9B — Shared server TUN + one NAT

Selected final Windows raw-IP server shape:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> shared WBD TUN
  <-> lease/tunnel demux
  <-> one current transport per tunnel
```

Exit gates:

- two different leased tunnel IPs simultaneously work as separate Logical Tunnels;
- both clients may use the same inner TCP source port `40000` to the same target/port;
- DNS-style UDP, generic UDP and TCP pass;
- target observes host NAT identity, not WBD private addresses;
- spoofed source lease is rejected;
- WBD firewall helper modifies only WBD-owned state.

## V2-M9C — Global one-public-transport enforcement

This milestone corrects the superseded multipath pivot without changing mature FakeTCP recovery.

Required work:

- release invariant `MaxProductPublicTransportLanes == 1`;
- Windows product controller owns exactly one `wbd-faketcp` public child per Connect;
- Linux product starts one raw single-flow public listener and no parallel kernel-TCP Reality admission listener;
- LINK server rejects a second concurrently usable transport claim for the same TunnelID;
- product UI/config has no desired public lane count greater than one;
- Game Lane/race libraries remain experimental/history only;
- release-contract tests make reintroducing multipath a CI failure;
- ADR/constitution/architecture/roadmap/handoff consistently point to ADR-0013.

Exit gate: exact-head CI + same-flow fullstack + Windows/Linux capability/build gates all pass after this freeze.

## V2-M9D — Payload-idle sleep and wake

A Logical Tunnel may retain lease/Wintun/routes while public transport count becomes zero.

Maintain separate clocks for real payload activity and transport liveness. PING/PONG/control never resets real payload idle.

On idle expiry:

- close the one FakeTCP/DTLS/LINK transport;
- keep Logical Tunnel lease, Wintun, capture routes/DNS and connected logical state;
- enter `DORMANT`.

On first new Wintun packet:

- wake with a bounded queue;
- establish one new same-flow Reality-like transport;
- resume when DTLS/LINK is ready.

Exit gate: sleep/wake does not require user reconnect, create a second public flow, or leak unbounded buffered packets.

## V2-M9E — Break-before-make replacement and mobility

Age/path replacement must obey the global one-flow invariant:

```text
A ACTIVE
  -> stop new payload admission to A
  -> bounded close/flush when possible
  -> retire A locally
  -> establish B
  -> Reality-like bootstrap -> DTLS -> LINK
  -> B ACTIVE
```

There is no A+B overlap.

This means planned rotation/network migration may have a short packet pause. A small bounded local reconnect buffer is allowed; a second public flow is not.

The same replacement state machine should handle:

- scheduled rotation;
- Windows NIC/default-route/public-IP change;
- NAT/path change;
- missed-PONG/no-valid-RX;
- FakeTCP/DTLS/LINK child failure;
- server-requested replace;
- manual reconnect.

Abrupt loss may leave stale server bookkeeping after the old physical path is already unusable. A fresh authenticated transport may supersede that stale record, but two usable data transports for one TunnelID are forbidden.

## Reality-like fidelity work

The first seconds should be as close to ordinary Reality-like/TLS behavior as reproducibly possible while retaining one raw FakeTCP association and no steady-state HOL.

Qualification should use packet captures and analyzers for:

- TCP persona (options, sequence/ACK progression, packet sizing/timing);
- TLS 1.3 ClientHello/persona/SNI/record progression;
- no second SYN/FIN barrier violation;
- same 4-tuple through DTLS mode transition.

Do not publish a numeric `99%`/browser-perfect similarity claim without a defined reproducible metric. If a fidelity technique needs sustained kernel TCP or a second public connection, the single-flow/no-HOL invariant wins pending a newer explicit decision.

## Platform requirements that must not regress

### Linux server

- one public `WBD_PORT` product surface;
- one raw single-flow WBD listener;
- no parallel ordinary kernel TCP Reality product listener;
- WBD-owned firewall/RST suppression only;
- never flush/replace the user's host ruleset;
- internal LINK/DTLS/raw-IP listeners remain private.

### Windows

- Wintun/raw-L3 remains the product capture path;
- underlay escape remains mandatory;
- one Connect -> one public `wbd-faketcp` child;
- Npcap ingress must ignore unrelated ARP/IPv6/UDP/TCP noise;
- device-wide IPv6 remains fail-closed while connected until IPv6 tunneling is qualified;
- Disconnect/Exit restores routes, DNS/NRPT, IPv6 and WBD-owned firewall state;
- Npcap installation/licensing constraints remain unchanged.

## Observability requirements

Retain non-secret correlation IDs, first-boundary markers and counters without per-packet INFO spam. Credentials, passwords, tickets, resume secrets and identity secrets never belong in logs.

Detailed development decisions, failed experiments, exact heads and gate results belong under `docs/development/` and must be summarized in `.wbd/handoff/current.json`.

## Final V2-M10 release gate

On one exact substantive `SOURCE_SHA`, all of the following must be true:

1. one-SYN Reality-like -> DTLS -> LINK -> raw-IP continuity is green;
2. no parallel ordinary kernel-TCP WBD bootstrap exists;
3. post-bootstrap no-HOL hole-bypass is green;
4. Windows Npcap noise filtering is green;
5. same TunnelID rejects a second concurrently usable transport;
6. distinct tunnel leases + shared TUN + one NAT are green;
7. same source port `40000` / same Internet target works for two **different Logical Tunnels**;
8. source spoofing is rejected;
9. break-before-make reconnect preserves the Logical Tunnel lease;
10. FEC `off` and `20:20` remain green;
11. same-flow startup stress and TCP/TLS persona capture gates are green;
12. Windows build/admin-smoke/capability and Linux release/firewall/full-stack gates are green;
13. Windows and Linux artifacts report the same `SOURCE_SHA`;
14. clean physical Windows 11 -> Ubuntu ARM64 passes DNS + UDP + TCP;
15. Disconnect/Exit restores route, DNS/NRPT, IPv6 and WBD-owned firewall state without manual repair.

Until all changed automated gates pass together, do not hand a new artifact to the physical tester.

## Deferred until after V2-M10 functional architecture

- DTLS HRR-cookie startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT work;
- Windows child-process/module slimming;
- native replacement of PowerShell underlay/network configuration;
- additional FEC profiles or higher release throughput caps;
- reopening multipath/Game Lane as a product feature (requires an explicit newer product decision because it conflicts with ADR-0013).
