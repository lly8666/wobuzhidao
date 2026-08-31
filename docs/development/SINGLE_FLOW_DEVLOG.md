# WBD Single-Flow Development Log

> Durable engineering history for interrupted-session recovery.
>
> This document records decisions, experiments, failures, fixes and qualification evidence. It is not a substitute for live GitHub Actions status. Always live-refresh the active branch and Actions before acting on a historical result.

## 0. Current product authority

ADR-0014 is the controlling product transport decision.

One connected Logical Tunnel owns exactly one public WBD TCP-shaped lineage:

```text
one raw FakeTCP SYN / one public 4-tuple / one FakeTCP sequence space
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 Reality-like setup, Firefox120 uTLS persona where practical
  -> protected admission / session identity
  -> explicit in-band bootstrap barrier
     (no FIN, no RST, no reconnect, no second WBD payload SYN)
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

Hard rules:

- FakeTCP owns the public flow from the first SYN.
- There is no preliminary ordinary kernel-TCP Reality product connection.
- A connected Logical Tunnel cannot own a simultaneous second public WBD transport.
- Reliable ordered semantics exist only during the bounded TLS/bootstrap adapter and are destroyed at the barrier.
- Post-barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing.
- The mature FakeTCP recovery/SACK/RTO/FEC core is frozen unless deterministic lower-layer evidence isolates a defect.
- Physical Windows testing is final acceptance only; it must not be used as the primary debugging loop while hosted Windows/Linux qualification is red or incomplete.

## 1. Historical architecture that was rejected

The earlier V2 implementation used two unrelated public connections:

```text
ordinary kernel TCP Reality-like TLS bootstrap
  -> account auth
  -> one-time ticket
  -> close TCP

then

new raw FakeTCP SYN
  -> DTLS
  -> LINK ticket bind
  -> payload
```

This was internally ticket-correlated but externally two separate TCP flows. NAT, conntrack, firewall and DPI therefore saw two unrelated lineages. It also created a server shared-port conflict: an ordinary kernel TCP listener and a raw FakeTCP listener both reacted to traffic on the public WBD port.

The user clarified that this violated the original product requirement: public observers must see one continuous TCP-looking connection, with the opening seconds Reality-like and the steady payload still free of ordinary TCP HOL.

ADR-0014 therefore superseded the dual-flow product architecture. Do not revive `Reality TCP -> close -> new FakeTCP SYN` as a shortcut.

## 2. Useful pre-single-flow transport work retained

The dual-flow era still produced transport fixes that remain valuable because they are below the architecture boundary.

### 2.1 FakeTCP final-ACK and DTLS-worker startup

Historical fixes included:

- accepting a data-bearing final ACK instead of requiring an empty final ACK;
- ensuring a successful final ACK falls through to normal segment delivery so first payload is not discarded;
- decoupling SYNACK latency from wolfSSL worker process creation;
- starting the DTLS worker after the FakeTCP association is established;
- adding half-open association expiry and stale-session pointer matching so old timers cannot delete a new connection reusing a 4-tuple;
- retrying Linux raw receive on `EINTR`;
- forcing inherited DTLS worker descriptors to blocking mode in the native child;
- adding DTLS stage markers (`PEEK`, `HRR`, `ACCEPT`) and FakeTCP raw TX/RX boundary markers.

Representative historical experiment winner was the D line around `10df2b0436411797e73352008db20d392bc5a8d6`, which passed repeated RTT100 mux load before the architecture pivot. These results validate the mature TCP-like core but do not validate the retired two-public-flow product topology.

### 2.2 Npcap send path

A Windows-specific bug was found in the Npcap mode setup: code had used `pcap_setmode(handle, 0)` while intending to clear global `SendToRxAdapters`. Npcap 1.88 defines the explicit clear flag as `MODE_SENDTORX_CLEAR = 0x0200`. The Windows path was changed to request the clear mode and fail fast instead of silently continuing with a possibly looped-back injection path.

A marker was added for successful setup:

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
```

### 2.3 Weak-network qualification philosophy

Do not weaken the release criteria to hide a tail retransmission. A historical `faketcp-native` 999/1000 run occurred while RTO had backed off beyond the probe's wait window; the natural rerun passed 1000/1000 with SACK/FastRetransmit/RTO exercised. No recovery-algorithm change was made from that single stochastic edge result.

The conservative release operating point remains 40 Mbit/s aggregate inner payload on <=100 Mbit/s weak links; higher 60/80 Mbit points are headroom, not the release contract.

## 3. Hosted DTLS / certificate / NAT experiments before the architecture pivot

To stop relying on physical Windows for every DTLS question, hosted tests were expanded.

### 3.1 Real CA + hostname DTLS mux environment

A Linux namespace test was upgraded from `none none` certificate arguments to a temporary CA and a SAN certificate for `wbd.test`, using the repository's pinned wolfSSL and real `wbd_dtls_shim` through the same FakeTCP mux/inherited-worker path.

Two independent clients completed:

- FakeTCP mux association;
- inherited DTLS worker startup;
- wolfSSL DTLS 1.3;
- CA verification + hostname verification;
- bidirectional UDP echo.

Observed server stages included:

```text
PEEK -> HRR -> ACCEPT -> DTLSv1.3 READY
```

This ruled out the Linux inherited-DTLS/CA path as the generic cause of the earlier physical DTLS timeout.

### 3.2 Kernel RST + NAT A/B

A namespace NAT experiment confirmed that a host kernel can generate RST for raw FakeTCP-shaped traffic. Two cases were compared:

- allow generated RST through NAT;
- suppress generated RST before the router.

Both FakeTCP and DTLS completed in that hosted topology. Conclusion: kernel RST existence is real, but hosted evidence did not justify adding a broad Windows firewall workaround as the primary product fix.

### 3.3 Shared kernel-TCP:443 + raw FakeTCP:443 experiment

When a real kernel TCP listener and the raw FakeTCP mux were deliberately placed on the same server port behind NAT, a hosted environment reproduced the physical failure shape:

```text
FakeTCP client READY
DTLS client CONNECT_START
server mux has no DTLS BOUND/PEEK
DTLS timeout
```

This was one of the strongest reasons to stop patching the dual-listener topology and move Reality-like setup into the single FakeTCP-owned association.

## 4. Architecture pivot to global single-flow

The product owner restated the requirement:

- public transport must always be one TCP-shaped connection;
- the first seconds must look as close to normal Reality-like/TLS traffic as technically practical;
- TCP-like logic may host that Reality-like setup;
- steady payload must never inherit ordinary TCP HOL.

The chosen architecture is not `kernel TCP takeover`. Instead FakeTCP owns the sequence space from SYN and provides a temporary reliable ordered byte-stream adapter only for TLS/bootstrap. This avoids the cross-platform problem of stealing an established kernel TCP sequence space from Windows/Linux while preserving the one-visible-flow requirement.

Conceptually:

```text
raw FakeTCP SYN / SYNACK / ACK
        |
        | same public 4-tuple + same FakeTCP sequence space
        v
bounded reliable ordered bootstrap stream
        |
        +-- real TLS 1.3
        +-- Firefox120 uTLS-style ClientHello persona
        +-- configured SNI
        +-- WBD recognition-compatible SessionID marker
        +-- protected account/admission exchange
        |
        v
explicit bootstrap barrier
        |
        | no FIN / RST / reconnect / second WBD SYN
        v
DTLS 1.3 datagram phase -> LINK/FEC/VPN
```

The opening reliable adapter may temporarily exhibit stream HOL because TLS requires ordered bytes. That bounded setup HOL ends at the barrier. The steady VPN payload path must retain datagram earliest-complete behavior.

## 5. Physical single-flow evidence already observed

These results are historical snapshots, not current release qualification.

### 5.1 A physical server run reached the full steady path

An Ubuntu ARM64 single-flow server log showed:

```text
WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1
BOUND role=server ... inherited=yes
WBD_DTLS_SERVER_PEEK ...
WBD_DTLS_SERVER_HRR_ARMED
WBD_DTLS_SERVER_ACCEPT_START
WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3 ...
READY role=server version=DTLSv1.3 ...
WBD_LINK_MUX_SESSION_READY ...
```

This proved that a same-flow bootstrap -> DTLS -> LINK path had worked at least once on a physical Ubuntu server.

### 5.2 A Windows build failed before ticket readiness

A later physical Windows self-test produced:

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
wbd-faketcp handshake: faketcp: not ipv4/tcp
wait for single-flow Reality ticket: ticket readiness timeout
```

The packet source was a real Npcap adapter, so unrelated ARP/IPv6/UDP/unrelated TCP frames can arrive while the FakeTCP handshake is waiting. Production code subsequently added Npcap ingress filtering so unrelated traffic is discarded before FakeTCP parsing. Therefore this historical failure must not be assumed to describe the current candidate.

### 5.3 Physical test policy after this point

Do not send another Windows/Linux artifact pair merely because a narrow single-flow test passed. The exact candidate HEAD must first pass the complete hosted Windows/Linux qualification matrix. Only then should the physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 run be requested.

## 6. Repository authority conflict recovered on 2026-08-31

Interrupted sessions exposed contradictory repository state:

- PR #9, `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, ADR-0012 partial-supersession text and ADR-0014 already described the global single-flow product;
- `ROADMAP.md` still described V2.5 1..4 public lanes / Game / make-before-break;
- feature `.wbd/handoff/current.json` was still sequence 61 and restored ADR-0012 multipath product policy;
- `tests/test_handoff.py` still required old V2.4/V2.5 lane wording.

At recovered feature head `2108490e5853d9ef854c80cd1a8da89c8ec11e0f`, handoff-verify run `33390347489`, job `99482302935`, printed `HANDOFF_VERIFY_PASS` from the machine verifier and then failed only because the stale Python architecture test expected:

```text
Each Transport Lane has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage
```

This was an authority/test failure, not a product data-plane failure.

### 6.1 Authority repair commits

- `02c94ded6649800105da25e649f4478a33eda0af` — `docs: align roadmap with ADR-0014 single-flow freeze`
- `433c98e7459b078c18a17257689a249a409e9d3e` — `test: enforce ADR-0014 single-flow handoff contract`
- `0b8612ad345832617b213bfcdd424f8fae6e55d1` — `docs: record single-flow authority recovery`
- `a97b5ddc256bb0d2ec375cc6eb6cd96f544bb1a0` — `handoff: sequence 75 restore ADR-0014 authority`

The TCP-like production core was not changed by these commits.

## 7. Exact-head release qualification infrastructure

Many expensive Windows/Linux workflows use path filters, which means a docs-only authority repair can appear to have no red gates simply because release workflows did not run.

`release-qualification-kick.yml` and `docs/development/QUALIFICATION_KICK.md` exist specifically to prevent that ambiguity.

For one immutable candidate HEAD the aggregator must dispatch/wait for heavy Windows/Linux workflows and require the expected push workflows on the same SHA. It rejects:

- older successes;
- PR merge SHAs;
- merely-started jobs;
- mixed candidate SHAs;
- branch movement during qualification.

The required set includes, among others:

- `windows-linux-single-flow.yml`;
- Windows portable/TUN/admin/raw-IP gates;
- Linux release/firewall gates;
- single-flow startup/link/fullstack;
- `mux-load-100m`;
- FakeTCP recovery / first-arrival / pcap loss;
- single-flow E2E / no-HOL / persona;
- main CI;
- OpenWrt regressions.

Artifacts may be handed to the user only after the exact candidate passes this hosted matrix and matching Windows/Linux release bundles identify the same source SHA.

## 8. Qualification attempt after sequence 75

A pure qualification trigger commit was made:

- candidate: `8a6041bd0b82f11ead14395504273086d5fe3352`
- message: `ci: kick exact-head ADR-0014 release qualification`
- aggregator run: `33392037082`

The aggregator correctly began dispatching exact-head Windows/Linux child workflows. However handoff-verify run `33392037115`, job `99487704225`, failed immediately for a repository recovery error:

```text
HANDOFF_VERIFY_FAIL: resume_read_set paths missing: ['docs/development/SINGLE_FLOW_DEVLOG.md']
```

This file is being created to fix that exact failure. Because creating it moves the feature branch, candidate `8a6041bd...` must be treated as invalidated for release qualification even if some child workflows later succeed. A fresh qualification kick is required after the new handoff sequence is committed.

## 9. Development discipline from here

1. Live-refresh branch and Actions before every continuation.
2. Read `.wbd/handoff/current.json`, this devlog and the dated recovery log before changing product behavior after a session interruption.
3. Treat ADR-0014 as the product public-transport authority.
4. Keep TCP-like/FakeTCP recovery/FEC frozen unless a deterministic lower-layer gate isolates a real core defect.
5. Fix the first deterministic failing layer only.
6. Update this log with important experiments, especially failed hypotheses, so future sessions do not repeat them.
7. End development turns with an updated machine-readable handoff.
8. Never call queued/in-progress Actions green.
9. Never deliver physical-test artifacts while the same-head hosted Windows/Linux matrix is incomplete or red.

## 10. Immediate next action

After this file is committed:

1. increment feature handoff from sequence 75 and record that the first seq75 qualification kick was invalidated by the missing-devlog handoff error;
2. make a fresh exact-head qualification kick;
3. freeze the branch;
4. inspect the first deterministic red from the complete same-head Windows/Linux matrix;
5. if the matrix is fully green, record all run IDs and release artifact source SHAs, then refresh canonical handoff;
6. only after hosted qualification is green, produce/hand off matching Windows x64 portable and Linux ARM64 release artifacts for final physical acceptance.
