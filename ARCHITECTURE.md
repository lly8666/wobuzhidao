# Architecture v2.x

> **Status: ACTIVE MAINLINE DESIGN. ADR-0015 is the human-authorized global public-flow authority.**

## Product intent

WBD is a personal weak-network VPN whose public weak-network carrier is WBD-owned raw TCP-shaped FakeTCP. One connected Logical Tunnel exposes exactly one public TCP-shaped association.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - stable server-assigned IPv4 lease
          - exactly one connected public FakeTCP transport
```

Shipping cardinality:

- Disconnected/dormant: 0 public flows.
- Connected: exactly 1 public flow.
- Maximum simultaneous public flow per TunnelID: 1.

There is no product 2..4-lane mode and no make-before-break public-flow overlap.

## One same-association lineage

The connected tunnel owns exactly one complete public lineage:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered bootstrap on that SAME association
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit in-band barrier; no FIN/RST/reconnect/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> immutable LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality product connection and no sustained WBD payload over an ordinary kernel TCP byte stream.

## Reality-like setup phase

FakeTCP owns the public association from the first SYN. Its bounded bootstrap adapter temporarily provides the reliable ordered byte-stream behavior TLS needs. Real TLS 1.3, configured SNI, Reality-like recognition and protected admission run on that same FakeTCP sequence space.

The first seconds should look as close to normal Reality-like/TLS behavior as practical. Fidelity is evidence-driven; use reproducible packet captures/handshake traces rather than unsupported percentage claims.

The bootstrap adapter is bounded in memory/time and destroyed at the explicit barrier. The transition does not send FIN/RST, reconnect or create another WBD payload SYN.

## No-HOL steady data plane

After the bootstrap barrier on the same association:

- DTLS application datagrams are independently authenticated;
- later independently complete payload may progress while an earlier FakeTCP sequence range is missing according to the qualified WBD recovery design;
- WBD shadow ACK/SACK/retransmission preserves TCP-shaped outer behavior without imposing ordinary kernel-TCP ordered delivery;
- systematic FEC sources are not delayed merely to fill a block.

The mature TCP-like/FakeTCP recovery and FEC wire remains frozen unless deterministic qualification proves a lower-layer defect.

## Logical Tunnel identity and lease

The server-assigned tunnel address belongs to Logical Tunnel/device identity rather than disposable FakeTCP/DTLS/LINK process state.

- same-account devices receive distinct leases;
- authenticated same-flow setup supplies address/prefix/route configuration;
- reconnect may preserve the Logical Tunnel lease according to lifecycle policy;
- raw IPv4 ingress requires `source == leased IPv4`; mismatch is a hard spoof/security drop.

Do not reintroduce a global fixed `10.66.0.2/30` identity.

## Lifecycle

Planned replacement is break-before-make at the public transport boundary:

```text
A ACTIVE
  -> stop new inner sends
  -> detach A locally
  -> stop A LINK / DTLS / FakeTCP
  -> confirm A public transport is gone
  -> create B
  -> B same-flow Reality-like bootstrap -> DTLS -> LINK
  -> attach B
  -> B ACTIVE
```

There is never an `A+B` simultaneous public interval for one Logical Tunnel.

DORMANT may preserve Logical Tunnel identity/lease and local TUN/routes while owning zero public transports. Wake establishes exactly one new public transport.

Abrupt underlay loss may degrade to reconnect after cleanup. Server reboot/conntrack loss is not promised to preserve arbitrary existing application TCP sessions.

## Canonical Windows stack

```text
one Wintun / raw L3
      ↓
Logical Tunnel
      ↓
one LINK + optional FEC state
      ↓
one DTLS 1.3 state
      ↓
one same-association Reality-like bootstrap + FakeTCP state
      ↓
public network
```

Shipping configuration accepts exactly one public transport. Historical Game/multilane helpers may remain as unreachable research code, but no product configuration or lifecycle may activate a second FakeTCP child.

## Canonical Linux server stack

The server exposes one public WBD port and can multiplex many users/Logical Tunnels by distinct public tuples:

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux
        <-> one active peer per authenticated TunnelID
        <-> per-tunnel DTLS + LINK
        <-> shared raw-IP/platform path
        <-> Linux root routing / WBD-owned NAT where configured
```

A single server process can handle many TunnelIDs; the global-single-flow rule is **per connected Logical Tunnel**, not a global server connection limit.

Per-LiveID netns/veth/double NAT remains historical/reference only. VRF/conntrack-zone remains rejected.

## Retained research code

Older 1..4-lane Game/race and candidate-lane packages may remain in the tree for historical/research experiments. They are not shipping authority. Release contracts must prove that shipping profile/runtime/server paths cannot create a simultaneous second public flow for one Logical Tunnel.

## Frozen weak-network/security boundaries

- no ordinary kernel-TCP sustained WBD payload and no TCP-over-TCP HOL;
- wolfSSL DTLS 1.3 remains pinned to `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- FEC release wire remains `off` or fixed systematic `20:20`;
- <=100 Mbit/s weak-link qualification ceiling remains;
- 40 Mbit/s aggregate-inner remains the conservative release operating point;
- Windows capture remains Wintun/TUN raw L3 with mandatory underlay escape;
- Windows IPv6 remains fail-closed while connected until real IPv6 qualification;
- OpenWrt capture remains TPROXY + policy routing;
- Linux firewall manipulation remains WBD-owned/scoped;
- Disconnect/Exit restores WBD-owned routes/DNS/NRPT/IPv6/firewall state;
- secrets do not belong in logs and per-packet INFO spam is forbidden;
- Npcap packaging/licensing constraints remain;
- startup RTT optimization and Windows child slimming remain deferred.

## Required qualification before artifact delivery

One exact substantive source HEAD must prove:

1. one SYN / one 4-tuple / one sequence lineage from Reality-like TLS bootstrap through DTLS, LINK and payload;
2. no preliminary ordinary-kernel-TCP Reality WBD flow;
3. no FIN/RST/reconnect/new WBD payload SYN across the bootstrap barrier;
4. post-bootstrap no-HOL hole-bypass;
5. shipping lane count is exactly one and `lanes != 1` is rejected;
6. replacement cannot overlap two public transports;
7. server rejects a second concurrent transport for the same TunnelID;
8. distinct Logical Tunnels receive distinct leases and spoof mismatch is rejected;
9. DNS-style UDP, generic UDP and TCP pass the final shared platform path;
10. FEC `off` and `20:20` and mature FakeTCP recovery remain green;
11. exact-head Windows hosted full-stack/build and Linux full-stack/release artifacts pass;
12. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

## Retired / invalid product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- 1..4 simultaneous public Transport Lanes per Logical Tunnel;
- Game public-flow racing/dedup in shipping product;
- make-before-break `A -> A+B -> B` public-flow overlap;
- per-LiveID netns/veth/double NAT as final raw-IP design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.
