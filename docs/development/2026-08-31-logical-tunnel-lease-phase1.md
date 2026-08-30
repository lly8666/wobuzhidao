# 2026-08-31 Logical Tunnel Lease Phase 1 Development Log

Status: **ACTIVE / ADR-0012 Phase 1**

This log is append-only engineering context for the ADR-0012 pivot. It exists so chat/session interruption cannot erase implementation history or reasoning.

## 2026-08-31 live refresh and stop-old-work checkpoint

- Live-refreshed PR #9 (`feat/single-flow-reality-faketcp`). Head at start of Phase 1 work: `0a19016304179b8f80ac43b51be0bb08fa7f201f`.
- Reconciled `.wbd/handoff/current.json` checkpoint `49366382d91de1ba0403e90caba645feaded5307` against PR head. The only intervening commit was `0a190163...`, updating handoff metadata; there was no concurrent product-code delta to rebase or overwrite.
- Re-read ADR-0012, PROJECT_CONSTITUTION.md, ARCHITECTURE.md, ROADMAP.md, current handoff, the architecture-pivot execution contract, historical raw-IP gateway log, `internal/gamelane/gamelane.go`, and `internal/dataplane/frame.go` in the required order.
- **Stopped product expansion of per-LiveID netns + veth + double NAT.** Existing implementation/tests/logs remain untouched as historical/correctness evidence. VRF/conntrack-zone remains rejected.
- FakeTCP/DTLS/FEC wire semantics are frozen for this phase. Existing Game Lane PacketID/first-arrival/dedup semantics remain the future multipath foundation; no migration-specific PacketSeq is being introduced.

## Current code ownership defect confirmed

Current raw-IP identity is still lane/backend-peer based rather than Logical-Tunnel based:

- `cmd/wbd-link-server-mux` consumes a one-time ticket into account + LiveID and generates a short diagnostic SID.
- For raw-IP payload it sends only the short SID as localhost metadata to `wbd-ip-gateway-server`.
- `cmd/wbd-ip-gateway-server` keys sessions by backend UDP peer and creates one netns/TUN/veth topology per backend session.
- Windows `Profile.normalized()` still globally defaults `TunnelIPv4` to `10.66.0.2/30`.

That ownership is exactly what ADR-0012 supersedes.

## Phase 1 selected integration boundary

The Phase 1 identity chain will be:

```text
Account
  -> Device / Installation ID (stable client-side installation identity)
      -> Logical Tunnel
          -> TunnelID
          -> server-assigned IPv4 lease
          -> authenticated tunnel config
              -> replaceable LiveID/FakeTCP/DTLS/LINK lanes
```

Implementation plan:

1. Add an architecture-neutral `internal/logicaltunnel` manager with a configurable IPv4 pool. The manager owns TunnelID + lease and is keyed by account + installation identity, not LiveID or FakeTCP tuple.
2. Extend the same-flow Reality-like TLS admission with a bounded v2 auth request that includes InstallationID and a bounded authenticated response carrying the one-time lane ticket plus TunnelID/address/prefix/route configuration.
3. Extend ticket records so LINK bind can recover the authenticated Logical-Tunnel binding when it consumes the one-time lane ticket. Existing v1 wrappers remain for historical tests/tools.
4. Windows persists a stable InstallationID locally, receives the authenticated tunnel config from the FakeTCP bootstrap child, and stops using a global `10.66.0.2/30` default. `/32` is the first product experiment.
5. Enforce raw-IP anti-spoof at the architecture-neutral LINK mux ingress boundary: decoded IPv4 source must equal the leased Logical-Tunnel IPv4 before payload is forwarded to any raw-IP backend. This avoids adding new product behavior to the superseded netns gateway and survives the Phase 2 shared-TUN replacement.
6. Local raw-IP backend metadata becomes TunnelID/lease aware rather than SID-only. Old SID metadata may remain as a compatibility/reference decoder until Phase 2 removes the obsolete path.

## Phase 1 gates before Phase 2

Automated tests must prove at minimum:

- two same-account installations acquire different active IPv4 leases;
- reconnect/second lane for the same account+installation resolves to the same active Logical Tunnel/lease;
- release makes lease reuse deterministic;
- a packet sourced from another Logical Tunnel's lease is rejected;
- authenticated single-flow bootstrap round-trips InstallationID -> TunnelID + `/32` address + route config + one-time lane ticket;
- Windows plan consumes server-returned address rather than relying on hard-coded `10.66.0.2/30`;
- existing single-flow/no-HOL/FakeTCP/DTLS/FEC tests remain unchanged in semantics and green.

## Explicit non-goals for this phase

Do not implement shared TUN/NAT yet, do not extend netns product topology, do not add DORMANT/rotation yet, do not change Game Lane wire semantics, do not optimize DTLS HRR/startup RTT, do not merge LINK bind/init, do not implement 0-RTT, do not slim Windows child processes, and do not change FakeTCP recovery/FEC profiles.
