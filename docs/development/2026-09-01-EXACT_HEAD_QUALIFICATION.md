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

## Change made by this candidate commit

The exact-head release aggregator is expanded to 18 explicit dispatch gates and 9 exact-SHA push gates (27 total). No product data-plane code, FakeTCP recovery, DTLS wire, LINK wire, or FEC wire is changed by this qualification-hardening commit.

The branch must remain frozen on this candidate while the aggregator runs. Any product/doc commit after the candidate invalidates the qualification and requires a fresh kick.

## Acceptance interpretation

Hosted qualification now self-builds the strongest practical upstream/downstream matrix available in GitHub Actions:

- native Windows runner executes Windows protocol/runtime code and emits single-flow wire evidence;
- Linux runner consumes that Windows wire through the Linux server state machine and continues into raw/netns full stack;
- independent Windows jobs build/qualify portable, TUN, admin, raw-IP gateway, packet persona, IPv6 and pinned DTLS pieces;
- Linux jobs build/qualify release bundles, firewall ownership, raw/netns single-flow, startup stress, LINK fullstack, shared-TUN/one-NAT, Game 1..4 lanes and weak-network load/recovery;
- single-flow E2E/persona/no-HOL gates prove one-SYN same-association setup and post-barrier non-HOL behavior.

Hosted GitHub Windows does not provide the user's physical Npcap/NIC/NAT/ISP path. Therefore no claim of final physical acceptance is allowed from Actions alone. After all 27 hosted gates are green and matching artifacts come from this exact source SHA, one physical Windows 11 -> Ubuntu ARM64 acceptance run is still required before declaring the release complete.
