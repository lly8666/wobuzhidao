# WBD Single-Flow Development Log

> Durable engineering history for interrupted-session recovery.
>
> This document records decisions, experiments, failures, fixes and qualification evidence. It is not a substitute for live GitHub Actions status. Always live-refresh the active branch and Actions before acting on a historical result.

## 0. Current product authority

The current product authority is **ADR-0011 + ADR-0012**.

The word **single-flow** applies to each independent **Transport Lane**, not to the entire Logical Tunnel.

Each Transport Lane owns exactly one public TCP-shaped lineage:

```text
one raw FakeTCP SYN / one public 4-tuple / one FakeTCP sequence space
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 Reality-like setup, Firefox120 uTLS persona where practical
  -> protected admission / lane identity
  -> explicit in-band bootstrap barrier
     (no FIN, no RST, no reconnect, no second SYN for that lane)
  -> pinned wolfSSL DTLS 1.3 on the same FakeTCP association
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

A connected **Logical Tunnel may own 1..4 independent complete Transport Lanes**. Each lane has its own public 4-tuple, FakeTCP sequence space, TLS bootstrap, DTLS state, LINK association and loss/recovery state. No FakeTCP sequence, DTLS nonce/record state or FEC state is shared across lanes.

For Game/latency racing, one logical PacketID may be transmitted over multiple independent lanes; the first valid complete arrival wins and later copies are suppressed. This is a product-layer race, not a shared TCP/FakeTCP sequence space.

Hard rules:

- FakeTCP owns each public lane from its first SYN.
- There is no preliminary ordinary kernel-TCP Reality product connection before a lane.
- Reality-like TLS setup occurs inside the same FakeTCP association that later carries DTLS/LINK payload.
- Reliable ordered semantics exist only during the bounded TLS/bootstrap adapter and are destroyed at the barrier.
- Post-barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing; steady payload must not inherit ordinary kernel-TCP HOL.
- The mature FakeTCP ACK/SACK/RTO/recovery/FEC core is frozen unless deterministic lower-layer evidence isolates a defect.
- Logical Tunnel lifecycle may add/remove/replace independent lanes (including make-before-break) while preserving tunnel identity/address lease.
- Physical Windows testing is final acceptance only; it must not be used as the primary debugging loop while hosted Windows/Linux qualification is red or incomplete.

### Withdrawn interpretation

An earlier repository interpretation treated ADR-0014 as meaning **one public WBD flow for the entire Logical Tunnel** and prohibited simultaneous independent lanes. That interpretation is withdrawn and is retained only as historical context. It must not be used to remove ADR-0012 multipath/lifecycle behavior.

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

The product requirement is instead: **within every Transport Lane**, public observers see one continuous TCP-looking connection, the opening seconds are Reality-like/TLS-like, and steady payload remains free of ordinary TCP HOL.

Do not revive `Reality TCP -> close -> new FakeTCP SYN` as a shortcut.

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

A Windows-specific bug was found in Npcap mode setup: code had used `pcap_setmode(handle, 0)` while intending to clear global `SendToRxAdapters`. Npcap 1.88 defines the explicit clear flag as `MODE_SENDTORX_CLEAR = 0x0200`. The Windows path was changed to request the clear mode and fail fast instead of silently continuing with a possibly looped-back injection path.

Marker:

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
```

### 2.3 Weak-network qualification philosophy

Do not weaken release criteria to hide a tail retransmission. A historical `faketcp-native` 999/1000 run occurred while RTO had backed off beyond the probe wait window; the natural rerun passed 1000/1000 with SACK/FastRetransmit/RTO exercised. No recovery-algorithm change was made from that single stochastic edge result.

The conservative release operating point remains 40 Mbit/s aggregate inner payload on <=100 Mbit/s weak links; higher 60/80 Mbit points are headroom, not the release contract.

## 3. Hosted DTLS / certificate / NAT experiments before the architecture pivot

### 3.1 Real CA + hostname DTLS mux environment

A Linux namespace test was upgraded from `none none` certificate arguments to a temporary CA and SAN certificate for `wbd.test`, using pinned wolfSSL and the real `wbd_dtls_shim` through the same FakeTCP mux/inherited-worker path.

Two independent clients completed FakeTCP association, inherited DTLS worker startup, wolfSSL DTLS 1.3, CA/hostname verification and bidirectional UDP echo. Server stages included:

```text
PEEK -> HRR -> ACCEPT -> DTLSv1.3 READY
```

This ruled out the Linux inherited-DTLS/CA path as the generic cause of earlier physical DTLS timeout.

### 3.2 Kernel RST + NAT A/B

A namespace NAT experiment confirmed that a host kernel can generate RST for raw FakeTCP-shaped traffic. Allowing generated RST through NAT and suppressing it before the router both completed FakeTCP + DTLS in that hosted topology. The evidence did not justify a broad Windows RST firewall workaround.

### 3.3 Shared kernel-TCP:443 + raw FakeTCP:443 experiment

When a real kernel TCP listener and raw FakeTCP mux were deliberately placed on the same server port behind NAT, hosted CI reproduced the physical failure shape:

```text
FakeTCP client READY
DTLS client CONNECT_START
server mux has no DTLS BOUND/PEEK
DTLS timeout
```

This was strong evidence against the retired dual-listener architecture and supported moving Reality-like setup into the FakeTCP-owned association itself.

## 4. Architecture pivot: single public flow per Transport Lane

The chosen architecture is not kernel-TCP takeover. FakeTCP owns each lane's sequence space from SYN and provides a temporary reliable ordered byte-stream adapter only for TLS/bootstrap. This avoids the cross-platform problem of stealing an established kernel TCP sequence space while preserving one-visible-flow-per-lane semantics.

Conceptually:

```text
raw FakeTCP SYN / SYNACK / ACK
        |
        | same public 4-tuple + same FakeTCP sequence space
        v
bounded reliable ordered bootstrap stream
        |
        +-- real TLS 1.3
        +-- Firefox120 uTLS-style ClientHello persona where practical
        +-- configured SNI
        +-- WBD recognition-compatible marker
        +-- protected account/admission exchange
        |
        v
explicit bootstrap barrier
        |
        | no FIN / RST / reconnect / second SYN for the lane
        v
DTLS 1.3 datagram phase -> LINK/FEC/VPN
```

The opening reliable adapter may temporarily exhibit stream HOL because TLS requires ordered bytes. That bounded setup HOL ends at the barrier. Steady VPN payload retains datagram earliest-complete behavior.

A Logical Tunnel may instantiate 1..4 such complete lanes. Lane independence is mandatory; multipath does not merge their TCP-like sequence spaces.

## 5. Physical single-flow evidence already observed

These are historical snapshots, not current release qualification.

### 5.1 Physical Ubuntu ARM64 reached steady state

A physical single-flow server run reached:

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

This proves same-flow bootstrap -> DTLS -> LINK has worked on physical Ubuntu.

### 5.2 Historical Windows Npcap ingress failure

A later physical Windows self-test produced:

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
wbd-faketcp handshake: faketcp: not ipv4/tcp
wait for single-flow Reality ticket: ticket readiness timeout
```

A real Npcap adapter can return unrelated ARP/IPv6/UDP/unrelated TCP frames while the FakeTCP handshake waits. Production code subsequently added ingress filtering so unrelated traffic is discarded before FakeTCP parsing. Do not assume this historical failure describes the current candidate.

### 5.3 Physical test policy

Do not hand out another Windows/Linux artifact pair because one narrow single-flow test passes. The exact candidate HEAD must first pass the complete hosted Windows/Linux qualification matrix. Only then request physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.

## 6. Authority-conflict recovery history

Interrupted sessions exposed contradictory repository state: single-flow-per-lane work, ADR-0012 multipath lifecycle, a temporary global-one-lane ADR-0014 interpretation, roadmap text and handoff tests were not aligned.

Several repair commits restored machine-readable recovery and revealed that some failures were authority/test failures rather than data-plane failures. Later reconciliation reaffirmed ADR-0012 multipath and withdrew the global-one-lane interpretation. Historical ADR-0014 material must therefore be read as superseded context, not current product authority.

Relevant durable reconciliation files include:

- `docs/development/2026-08-31-adr0014-globalization-rollback.md`;
- `docs/development/2026-08-31-adr0012-multipath-reaffirmation.md`;
- `docs/development/2026-08-31-contract-reconciliation.md`.

## 7. Exact-head release qualification infrastructure

Many expensive Windows/Linux workflows use path filters. A docs-only or upper-layer fix can therefore appear to have no red gates simply because release workflows did not run.

`release-qualification-kick.yml` and `docs/development/QUALIFICATION_KICK.md` exist to prevent that ambiguity.

For one immutable candidate HEAD, the aggregator dispatches 18 product workflows and requires 9 exact-head push gates. It rejects older successes, PR merge SHAs, merely-started jobs, mixed candidate SHAs and branch movement during qualification.

The dispatched product set covers, among others:

- combined `windows-linux-single-flow.yml`;
- Windows portable/TUN/admin/raw-IP/persona/IPv6/DTLS gates;
- Linux release/firewall gates;
- single-flow raw-IP/startup/LINK fullstack;
- FakeTCP recovery;
- OpenWrt regression;
- Game 1..4-lane product lifecycle;
- shared-TUN two-client lifecycle.

The exact-head push set includes main CI, FakeTCP native/pcap/first-arrival, fullstack first-arrival, OpenWrt TCP TPROXY, single-flow E2E/no-HOL/TCP persona.

Artifacts may be handed to the user only after the exact candidate passes this hosted matrix and matching Windows/Linux bundles identify the same source SHA.

## 8. 2026-09-01 recovery and release-repair cycle

### 8.1 Live candidate compile blocker

Recovered feature head:

```text
3124abfb1d8370619dc724f9f796a7fd4192865b
```

Both Linux ARM64 and AMD64 release builds failed at compile time:

```text
internal/gamelane/control.go:56:10: undefined: ErrLanes
internal/gamelane/control.go:61:11: undefined: ErrLanes
```

`internal/gamelane/control_test.go` already expected `errors.Is(err, ErrLanes)`. `internal/gamelane/gamelane.go` defined lane limits but the sentinel error had been removed during upper-layer refactoring.

Fix:

```text
604996ae3344c6f43464ecacaa3b9934c790cca5
fix: restore Game lane membership error
```

Only `ErrLanes = errors.New("gamelane: invalid lane membership")` was restored. No FakeTCP/TCP-like/DTLS/FEC wire behavior changed.

### 8.2 Game matrix artifact executable-bit blocker

After the compile repair, the Game runtime matrix failed before network behavior because downloaded Actions artifacts lost Unix executable bits:

```text
exec of ".../wbd-game-lane-server" failed: Permission denied
```

First CI fix:

```text
54209c78d1268a3133d29d70002e23d80720e93e
ci: restore executable bits for Game lane artifacts
```

This restored `wbd-*` executable bits. The next run progressed farther: single-flow bootstrap completed and the server/mux started, but DTLS activation then failed because the native shim uses an underscore name and was not matched by `wbd-*`:

```text
WBD_SINGLE_FLOW_DTLS_ACTIVATE_FAIL ... wbd_dtls_shim: permission denied
```

Second CI fix:

```text
f20dbb7963a9657b973ad836afeb72081c2caa48
ci: restore DTLS shim executable bit
```

The workflow now restores executable bits for both `wbd-*` and `wbd_dtls_shim` before the 1/2/3/4-lane + fixed-20:20 smoke matrix runs.

Again, this is qualification infrastructure only; the TCP-like core remains unchanged.

### 8.3 Release authority for this cycle

Do not deliver artifacts from `3124abfb`, `604996ae`, `54209c78` or `f20dbb79` merely because a subset of checks is green. After deterministic pre-kick failures are repaired and the development log is current, create a fresh final commit by updating `docs/development/QUALIFICATION_KICK.md`, freeze that exact feature HEAD, and require `release-qualification-kick` to certify all exact-head child workflows.

## 9. Development discipline

1. Live-refresh branch and Actions before every continuation.
2. Read canonical `.wbd/handoff/current.json`, this devlog and the dated reconciliation logs after interruption.
3. Treat ADR-0011 + ADR-0012 as current product authority: one public single-flow association per Transport Lane; 1..4 independent lanes per Logical Tunnel.
4. Treat the global-one-lane ADR-0014 interpretation as withdrawn historical context.
5. Keep TCP-like/FakeTCP ACK/SACK/RTO/recovery/FEC frozen unless a deterministic lower-layer gate isolates a core defect.
6. Fix the first deterministic failing layer only.
7. Update this log with important experiments and failed hypotheses so later sessions do not repeat them.
8. End a development cycle with an updated machine-readable handoff on the canonical branch.
9. Never call queued/in-progress Actions green.
10. Never deliver physical-test artifacts while the same-head hosted Windows/Linux release matrix is incomplete or red.

## 10. Immediate next action

1. Let `f20dbb7963a9657b973ad836afeb72081c2caa48` run far enough to verify that the Game 1..4-lane/FEC-smoke matrix has progressed past artifact permissions.
2. Inspect the first deterministic red anywhere on the current feature HEAD; repair only that layer.
3. Once ordinary pre-kick checks show no known deterministic blocker, update `docs/development/QUALIFICATION_KICK.md` as the **last feature-branch commit**.
4. Freeze the resulting candidate HEAD while `release-qualification-kick` dispatches and validates its 27 exact-head child gates.
5. On any child failure, record the evidence here, fix it, and produce a new kick/candidate; never mix evidence from different SHAs.
6. If exact-head qualification is fully green, fetch and verify the Windows x64 portable and Linux ARM64 release artifacts from that exact SHA.
7. Update canonical handoff with candidate SHA, qualification run IDs, results and artifact digests; verify handoff green.
8. Only then provide the matching artifacts for final physical Windows-to-Ubuntu acceptance.
