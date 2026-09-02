# ADR-0015: Human-authorized global single public flow

Status: **ACCEPTED / HUMAN PRODUCT-OWNER AUTHORITY — 2026-09-02**

## Authority and correction

The live human product owner explicitly requires that a connected WBD tunnel expose **exactly one simultaneous public TCP-shaped connection**. This ADR records that instruction and corrects the repository drift that generalized per-lane single-flow into a 1..4-public-lane product.

For shipping product behavior, this ADR supersedes the public-transport-cardinality, Game multipath and make-before-break overlap clauses of ADR-0012 and the withdrawal rationale in ADR-0014. Historical files remain in the repository as engineering history, but they are not authority where they conflict with this ADR.

Agent-authored documents cannot override this rule without a later explicit live human instruction.

## Global invariant

For one connected Logical Tunnel / installation:

```text
EXACTLY ONE simultaneous public WBD FakeTCP association
EXACTLY ONE public 4-tuple / TCP-shaped sequence lineage
EXACTLY ONE SYN lineage while connected
```

There is no preliminary ordinary-kernel-TCP Reality WBD connection, no second FakeTCP lane, no Game/race copy over another public flow, and no make-before-break overlap.

A reconnect or planned replacement may create a new public association **only after the previous public association has been removed from the shipping data path and torn down**. Planned replacement is therefore break-before-make at the public-flow boundary.

## Same-flow Reality-like setup

FakeTCP owns the one public association from its first SYN.

The beginning of that exact association temporarily provides bounded reliable/ordered stream semantics so that the setup phase can look and behave as close to a normal Reality-like TLS 1.3 exchange as practical:

```text
one raw FakeTCP SYN / SYNACK / ACK lineage
  -> bounded reliable ordered bootstrap adapter on SAME association
  -> real TLS 1.3 ClientHello / ServerHello / Finished
  -> configured SNI and Reality-like recognition
  -> protected username/password admission
  -> authenticated Logical Tunnel configuration / ticket binding
  -> explicit in-band bootstrap barrier
```

The setup phase is intentionally short. Its temporary ordered adapter is destroyed at the barrier.

Reality-like fidelity is evidence-driven. The target is normal-looking TCP/TLS behavior during the first seconds, but no numeric similarity percentage is claimed without a reproducible packet-capture metric.

## No-HOL steady state on the same association

The barrier does **not** send FIN/RST, reconnect, or a new SYN. The same FakeTCP association and public 4-tuple continue as:

```text
same public FakeTCP association
  -> pinned wolfSSL DTLS 1.3
  -> immutable LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload
```

Sustained WBD payload never runs inside an ordinary kernel TCP byte stream. Missing FakeTCP sequence ranges must not impose ordinary TCP ordered-delivery HOL on independently complete DTLS/FEC payload.

The mature FakeTCP ARQ/recovery/FEC wire is frozen unless a deterministic lower-layer qualification isolates a real defect.

## Product cardinality

Shipping product policy is not a range:

```text
Disconnected / dormant: 0 public flows
Connected:               1 public flow
Maximum simultaneous:    1 public flow
```

`lanes=2`, `lanes=3`, `lanes=4`, dynamic second-lane attachment, Game public multipath, public-flow racing/dedup and candidate overlap are rejected in the shipping runtime.

Research packages may retain historical multipath code only if it is unreachable from shipping configuration and release qualification. No research component may weaken the product cardinality guard.

## Lifecycle

Planned public-flow replacement is:

```text
A ACTIVE
  -> stop new inner sends to A
  -> detach A from the shipping local aggregation path
  -> stop LINK / DTLS / FakeTCP for A
  -> confirm old public transport is no longer active
  -> create B with a fresh public association
  -> B performs same-flow Reality-like bootstrap -> barrier -> DTLS -> LINK
  -> attach B
  -> B ACTIVE
```

There is never an `A+B` public overlap for one Logical Tunnel.

If A has already failed abruptly, reconnect starts B after cleanup of WBD-owned local state. The product does not promise preservation of arbitrary application TCP sessions across complete underlay loss or server reboot.

## Server rule

The Linux public raw mux may serve many users and many Logical Tunnels on one WBD public port. However, **each authenticated TunnelID may have at most one simultaneous public transport peer**. A second concurrent transport claim for the same TunnelID is rejected.

## Qualification before artifact delivery

One exact substantive source HEAD must prove at minimum:

1. one SYN / one 4-tuple / one FakeTCP sequence lineage from Reality-like TLS setup through DTLS, LINK and payload;
2. no ordinary preliminary Reality TCP connection;
3. no FIN/RST/reconnect/new WBD payload SYN at the bootstrap barrier;
4. post-bootstrap no-HOL hole-bypass remains green;
5. product profiles accept `lanes=1` and reject every `lanes!=1` connected configuration;
6. shipping lifecycle cannot create a simultaneous candidate/second public flow;
7. server rejects a second concurrent public transport claim for the same TunnelID;
8. FEC `off` and fixed `20:20` and the mature FakeTCP recovery gates remain green;
9. exact-source Windows hosted build/fullstack and Linux fullstack/release gates pass;
10. final same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

## Non-goals

This correction does not redesign FakeTCP ARQ, SACK/RTO behavior, DTLS wire, LINK wire or FEC wire. It removes unauthorized multi-public-flow orchestration and freezes the single public association as a product invariant.
