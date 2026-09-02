# 2026-09-01 exact-head Windows/Linux qualification recovery

## Why this log exists

Chat recovery has repeatedly truncated the development discussion. This file is durable project history for the current qualification cycle. Future sessions should read it together with `.wbd/handoff/current.json` and `docs/development/SINGLE_FLOW_DEVLOG.md` before changing product behavior.

## Recovered state before this candidate

- Canonical `dev/wbd-raw-fec-v2` was at handoff sequence 79, commit `7d7a6acc945a69f8b811a1accae943ca8acfffd5`.
- Active feature branch `feat/single-flow-reality-faketcp` was at `e7b21b0980ca0dec721dfdd3a34ec21cb68242d7`.
- PR #9 remained the active single-flow/product branch.
- The visible exact-head check set for `e7b21b09` had no observed failure conclusion, but several release jobs were skipped by path filters. That is not release authorization.

## Current architecture authority

The current Constitution/Architecture, not the stale top section of older devlog text, controls:

- `single-flow` is PER TRANSPORT LANE.
- Each lane owns exactly one FakeTCP SYN lineage/public 4-tuple/sequence space.
- The opening bounded reliable adapter carries real TLS 1.3 Reality-like recognition/admission inside that same FakeTCP association.
- The in-band bootstrap -> DTLS switch emits no FIN/RST/reconnect/new WBD payload SYN inside a lane.
- Steady payload is DTLS/LINK/FEC packet/datagram oriented and must not inherit ordinary kernel-TCP HOL.
- A Logical Tunnel may own 1..4 independent complete lanes; Game/race and make-before-break are product behavior.
- Linux final product topology is per-lane transport -> Game/race -> Logical Tunnel -> one shared WBD TUN -> one WBD-owned host NAT.
- Mature FakeTCP/TCP-like ACK/SACK/RTO and release FEC behavior is frozen unless deterministic evidence isolates a core defect.

ADR-0014's old global-one-flow interpretation is withdrawn/invalidated. ADR-0011 controls same-association per-lane bootstrap/no-HOL; ADR-0012 controls Logical Tunnel multipath/lifecycle.

## Historical physical evidence that remains relevant

A physical Ubuntu ARM64 run previously reached:

`WBD_SINGLE_FLOW_BOOTSTRAP_READY same_flow=1 -> DTLS PEEK/HRR/ACCEPT_PASS -> READY role=server -> WBD_LINK_MUX_SESSION_READY`.

A later physical Windows run failed before ticket readiness with `wbd-faketcp handshake: faketcp: not ipv4/tcp`. Npcap ingress filtering for unrelated frames was subsequently added, so that historical failure is not automatically attributable to the current candidate. Physical Windows is final acceptance, not the primary debugging loop while hosted qualification is incomplete.

## Qualification gap found on 2026-09-01

The existing release aggregator already forced important Windows/Linux/single-flow/load gates, but it had not yet made the recently restored product layer mandatory. In particular, exact-head release authority did not force:

- `game-lane-fullstack.yml` for 1..4 lanes and FEC off/20:20;
- `shared-tun-two-client.yml` for one shared Linux TUN + one host NAT with two authenticated tunnels;
- explicit Windows FakeTCP persona, IPv6 kill-switch, and pinned DTLS build gates.

That gap could allow a candidate to look release-qualified while the newest Game/shared-TUN product wiring had only path-filtered or stale evidence.

## Candidate 2370c0b7: full matrix hardening and stale Linux settings contract

Commit `2370c0b730091a86038e96bb025aea027daa6d37` expanded release authority to 18 workflow-dispatch gates plus 9 exact-SHA push gates (27 total). It changed no product data-plane behavior.

The aggregator run `33455412514` successfully verified that the candidate was still feature-branch HEAD and dispatched the 18 opt-in workflows. The first deterministic red was exact-head `linux-server-release` run `33455435058`, settings job `99694293613`:

```text
scripts/linux_server_manager.sh: .../opt/bin/wbd-ip-gateway-shared: not found
product run unexpectedly started Game Lane
```

Inspection showed the production manager and release builder were already aligned with the current Constitution: the manager launches shared-TUN gateway -> Game server -> LINK server -> one public raw mux, and the release builder packages `wbd-ip-gateway-shared`, `wbd-game-lane-server`, shared-TUN firewall helpers and reports `max_tunnel_lanes=4 game_product=1 shared_tun=1 host_nat=1`.

The failure was instead a stale `scripts/linux_server_settings_test.sh` fixture left from the withdrawn ADR-0014 global-one-flow interpretation. That test deliberately omitted the shared gateway binary, made any Game invocation exit 99, expected `max_tunnel_lanes=1`, and required LINK to connect directly to the legacy platform service. The test was therefore rejecting the correct current product topology.

Fix policy: update only the Linux settings qualification fixture/contract. Do not change FakeTCP/TCP-like recovery, DTLS, LINK wire or FEC wire. Candidate `76b6cb8ae6fd0fd43d7eb029d464b345a51404eb` carried that correction and a new exact-head kick.

## Candidate 76b6cb8a: stale Windows single-lane test contracts

The next deterministic red was the normal exact-head `ci` push run `33455678780`, job `99695021574`. Product packages compiled and most Go packages passed, but old Windows tests still encoded the withdrawn global-one-lane interpretation. Representative failures were:

```text
internal/windowsgui: second product lane error=<nil>
internal/windowsruntime: lanes=2 unexpectedly accepted
controller events: actual start:faketcp-1 -> dtls-1 -> link-1 -> game -> tun; old test expected faketcp -> dtls -> link -> tun
```

The implementation was already current product behavior:

- `Profile.Validate()` delegates lane count to `logicaltunnel.ValidateProductTransportLaneCount`, whose product range is 1..4;
- `Controller.Connect()` creates one `LaneBootstrap` per lane, assigns a distinct dynamic FakeTCP source port, performs same-association Reality-like bootstrap per lane, verifies all lanes return the same authenticated Logical Tunnel lease, then transfers ownership to `Executor.StartMultiLane`;
- `Executor.StartMultiLane` waits each lane FakeTCP/DTLS/LINK before starting one Game aggregator and one TUN, and rolls all lane processes back before exposing routes on failure.

The failing tests were stale rather than the implementation. In particular, controller failure injection still targeted command name `faketcp` even though product lane command names are now `faketcp-1`..`faketcp-4`; some tests therefore no longer injected the failure they claimed to test.

Three test-only commits migrated the Windows qualification contract without changing product implementation:

1. `4902266902e0f6829fa4e47225abe3e7a39b9a86` — controller tests use lane-aware process names/order, assert Game/TUN lifecycle, add a full two-lane connect/disconnect test, and verify second-lane bootstrap failure rolls both prestarted public flows back before DTLS/routes.
2. `e0e3436f59343e3ffc899d8a51c9a06059463499` — runtime profile tests accept the frozen product lane range 1..4 and reject out-of-range values.
3. `35dd45cccddad8288f39245dd66022bea434d6be` — GUI profile tests accept 1..4 lanes and reject 5.

A fast pre-qualification CI on `35dd45cc` completed successfully: run `33456102687`, job `99696263062`, with both `Go tests` and `Handoff tests` green. This is only a syntax/unit-contract preflight; it is not release authority because the final exact-head full product matrix has not yet been kicked on a frozen candidate.

## Qualification discipline after these fixes

- Do not touch mature FakeTCP/TCP-like ACK/SACK/RTO/FEC logic unless a deterministic lower-layer/full-stack gate isolates it.
- A change to a stale test contract must strengthen current architecture assertions rather than merely delete old checks.
- Each candidate that moves the feature branch invalidates previous qualification evidence.
- The final candidate must remain branch HEAD while the aggregator re-dispatches all 18 opt-in gates and resolves all 9 exact-SHA push gates.
- Packaging success alone is not delivery authority.

## Acceptance interpretation

Hosted qualification self-builds the strongest practical upstream/downstream matrix available in GitHub Actions:

- native Windows runner executes Windows protocol/runtime code and emits single-flow wire evidence;
- Linux runner consumes that Windows wire through the Linux server state machine and continues into raw/netns full stack;
- independent Windows jobs build/qualify portable, TUN, admin, raw-IP gateway, packet persona, IPv6 and pinned DTLS pieces;
- Linux jobs build/qualify release bundles, firewall ownership, raw/netns single-flow, startup stress, LINK fullstack, shared-TUN/one-NAT, Game 1..4 lanes and weak-network load/recovery;
- single-flow E2E/persona/no-HOL gates prove one-SYN same-association setup and post-barrier non-HOL behavior.

Hosted GitHub Windows does not provide the user's physical Npcap/NIC/NAT/ISP path. Therefore no claim of final physical acceptance is allowed from Actions alone. After all 27 hosted gates are green and matching artifacts come from the exact source SHA, one physical Windows 11 -> Ubuntu ARM64 acceptance run is still required before declaring the release complete.
