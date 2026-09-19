# 2026-09-02 — Global single-public-flow authority correction

## Why this log exists

A sequence of prior agent-authored changes drifted from the human product requirement. The repository evolved from the desired single public TCP-shaped connection into a Logical Tunnel architecture that allowed 1..4 independent public FakeTCP Transport Lanes, Game/race duplication and make-before-break overlap.

The human product owner corrected this explicitly in the live development conversation: **WBD must always present one public TCP connection/flow for one connected tunnel.** The first seconds should be as Reality-like/TLS-like as practical, and the same public association must then carry the no-HOL DTLS/FEC data plane without a second SYN.

This file is the durable engineering record of the correction. Do not reconstruct authority from old handoff text or withdrawn ADR wording.

## Valid technical work retained

The architecture correction does **not** discard the mature TCP-like data plane. The following remain valuable and frozen unless deterministic evidence says otherwise:

- raw TCP-shaped FakeTCP owns the public association;
- legacy recovery remains the default;
- established SACK/RTO/first-arrival/FEC behavior remains intact;
- pinned wolfSSL DTLS 1.3 remains the steady-state crypto transport;
- immutable LINK remains above DTLS;
- FEC release wire remains only `off` or fixed systematic `20:20`;
- release operating point remains 40 Mbit/s aggregate inner on <=100 Mbit/s weak links;
- Windows Npcap and Linux raw mux implementation work remains reusable.

## Important historical debugging conclusions

### The rejected two-connection architecture

An older implementation did:

```text
ordinary TCP Reality-like bootstrap -> close -> new raw FakeTCP SYN -> DTLS -> LINK
```

This violated the product requirement because NAT/firewall/DPI saw two unrelated connections. It also created real conntrack/shared-port races. It is permanently rejected.

### Same-flow implementation evidence

Later work moved Reality-like TLS bootstrap onto the FakeTCP association itself. Physical Linux evidence reached:

```text
WBD_SINGLE_FLOW_BOOTSTRAP_READY same_flow=1
-> inherited DTLS BOUND
-> PEEK / HRR / ACCEPT_PASS
-> READY role=server DTLSv1.3
-> WBD_LINK_MUX_SESSION_READY
```

This proved that a single FakeTCP association can carry the bounded Reality-like setup and then transition to DTLS/LINK without a second product SYN.

### Windows ingress/readiness work

Windows development added readiness-gated process startup and corrected Npcap `MODE_SENDTORX_CLEAR` handling. Diagnostic markers were added at FakeTCP raw TX/RX, mux ingress and DTLS handshake stages. Hosted regressions were added after a physical `faketcp: not ipv4/tcp` ingress failure.

### Virtual/CI environments built

The project already has meaningful non-physical qualification and should use it before asking for another physical run:

- Linux network namespaces with raw FakeTCP;
- NAT/router namespaces;
- pinned wolfSSL DTLS 1.3 client/server;
- temporary CA + SAN `wbd.test` server certificate with CA/hostname verification;
- inherited UDP DTLS worker path;
- bidirectional UDP echo;
- loss/recovery and no-HOL qualification;
- Windows hosted compilation/fullstack regressions where Npcap physical capture is not required.

Physical Windows 11 + Npcap + real NIC/NAT/ISP -> Ubuntu ARM64 remains the final non-simulatable gate, not the first debugging environment.

## Architecture drift found on 2026-09-02

PR #9 shipping Windows runtime was found to support multiple simultaneous public flows:

- `Profile.Lanes` allowed 1..4;
- `Controller.Connect()` started one FakeTCP/DTLS/LINK group per configured lane;
- Game/race aggregated those groups;
- `ReplaceLane()` performed make-before-break by starting a candidate public FakeTCP flow before stopping the old one;
- Linux LINK mux allowed up to four concurrent public peers for one TunnelID;
- release-contract tests explicitly required the multipath behavior.

This was not merely dormant research code: it was reachable from shipping profile/runtime code and therefore violated the live product requirement.

## Correction plan

ADR-0015 is the current human-authorized product authority.

Implementation changes are intentionally orchestration/contract focused:

1. shipping product lane count becomes exactly one;
2. `lanes != 1` is rejected by runtime/profile validation;
3. Windows connect/wake can create only one public FakeTCP transport;
4. planned replacement becomes break-before-make at the public-flow boundary;
5. candidate/dynamic/Game APIs must not create a simultaneous second public flow in shipping mode;
6. Linux LINK mux caps simultaneous public transport at one per TunnelID;
7. architecture/release-contract tests are rewritten to guard the global invariant;
8. FakeTCP ARQ, DTLS, LINK and FEC wire semantics are not changed for this correction.

## Delivery rule

Do not produce a new user artifact from a mixed or partially qualified head. One exact substantive source SHA must pass Windows and Linux automated qualification, including single-flow, no-HOL, fullstack and release gates, before artifact delivery. The final physical Windows-to-Ubuntu run is performed only after those automated gates are green.

## Handoff rule

At the end of the cycle, `.wbd/handoff/current.json` must be advanced from the current sequence with:

- the exact substantive candidate SHA;
- live-refresh requirement;
- current green/red workflow evidence marked as a snapshot, not live state;
- the remaining physical-only validation;
- a resume read set containing ADR-0015, this development log, the shipping Windows runtime, Linux single-flow server path and release contract.

`handoff-verify` must be green before the handoff is declared complete.
