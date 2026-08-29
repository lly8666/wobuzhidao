# WBD V2.3 Single-Flow Development Log

This file is the durable engineering log for the V2.3 transport architecture. It exists because interactive chat history can be truncated; decisions, failed hypotheses, physical-machine evidence, CI evidence, and next actions must therefore be recoverable from the repository itself.

## Frozen product requirement

The public session is exactly one TCP-shaped flow from the first SYN until disconnect.

1. FakeTCP owns the public 4-tuple and TCP sequence lineage from the sole SYN/SYNACK/ACK.
2. The first bounded phase of that same association carries a Reality-like TLS 1.3 persona and account admission. It should resemble ordinary Reality/TLS setup as closely as practical without handing the steady-state data path to kernel TCP.
3. There is no FIN/close followed by a second public SYN. No separate ordinary-TCP Reality bootstrap is allowed in the product Connect path.
4. After admission, the same association switches to DTLS 1.3 and then LINK/FEC/VPN datagrams.
5. Steady-state payload must not inherit ordinary TCP head-of-line blocking. A missing earlier FakeTCP sequence must not block delivery of a later independent post-switch datagram.
6. The mature FakeTCP recovery/FEC implementation is frozen unless deterministic new evidence specifically requires a change. Work should concentrate on setup, mode switch, orchestration, qualification, and observability.

## Why V2.2/early V2.3 dual-flow architecture was rejected

The earlier implementation used two public flows:

- ordinary kernel TCP + TLS/Reality-like bootstrap to obtain a one-time ticket;
- after that process exited, a new raw FakeTCP handshake opened a second public flow, then DTLS/LINK used the ticket.

The ticket logically related the two flows inside WBD, but NAT/firewall/DPI devices saw unrelated connection state. This contradicted the original requirement that the whole session be one TCP-looking connection.

Physical testing also exposed practical consequences of the split design. Reality and FakeTCP could reuse or compete on the same public port/tuple, kernel TCP and raw FakeTCP maintained different sequence spaces, and real NAT/conntrack behavior could disagree with WBD even when the FakeTCP client locally printed READY.

The architectural correction is not "run VPN payload over TLS/TCP". That would restore ordinary TCP retransmission/ordered-delivery HOL and defeat the weak-network design. Instead, FakeTCP owns the connection from SYN onward and temporarily exposes a bounded reliable ordered bootstrap stream only for the TLS/admission phase. After the switch, the existing datagram-oriented transport semantics resume.

## Legacy FakeTCP stabilization retained as mature core

Before the single-flow pivot, the raw transport was extensively stabilized. Important retained fixes include:

- accepting a data-bearing final ACK in the server association state machine;
- preserving that payload instead of discarding it after handshake state transition;
- delaying expensive DTLS worker creation until the FakeTCP association is established;
- expiring half-open sessions without allowing stale cleanup goroutines to delete a newer connection reusing the same flow key;
- retrying Linux raw `recvfrom` on `EINTR`;
- DTLS worker inherited-fd/blocking fixes and detailed DTLS handshake markers;
- Windows Npcap `MODE_SENDTORX_CLEAR = 0x0200` rather than incorrectly using capture mode 0;
- readiness-gated Windows startup so FakeTCP must be ready before DTLS, DTLS before LINK, LINK before TUN/routes.

These behaviors form the mature TCP-like/data-plane baseline and are not targets for gratuitous redesign.

## Physical-machine evidence that led to the pivot

On the old split-flow build, a real Windows 11 x64 client and Ubuntu ARM64 server showed several stages over repeated tests:

- Reality-like admission succeeded.
- FakeTCP client could print READY.
- after readiness gating was added, DTLS timeout became the first unambiguous blocker rather than a misleading later LINK/DNS failure.
- a later diagnostic build produced a complete successful server chain on one attempt: raw payload observed, DTLS worker BOUND, PEEK, peer set, HRR armed, ACCEPT_PASS, DTLS READY, then `WBD_LINK_MUX_SESSION_READY`.
- subsequent reconnects could again stop before DTLS worker BOUND even though the Windows FakeTCP process printed READY.

This intermittent shape motivated increasingly realistic namespace/NAT tests and ultimately the recognition that keeping ordinary kernel TCP and raw FakeTCP as independent public flows was itself the wrong product model.

## Sandbox/CI experiments before the single-flow pivot

The project now has substantial network-namespace qualification rather than relying only on physical testing.

### Real certificate DTLS mux test

A virtual environment creates a temporary CA/service certificate and runs the pinned wolfSSL DTLS 1.3 shim through the real FakeTCP mux inherited-worker path. Multiple clients validate CA/hostname and exchange bidirectional UDP payload. This proved that Linux mux -> inherited UDP worker -> pinned wolfSSL DTLS 1.3 -> return payload is functional in isolation.

### Kernel RST/NAT A/B

A NAT router namespace was used while allowing versus suppressing client kernel TCP RST packets. Kernel RSTs were observed, including through the router, but DTLS still completed in both groups in that environment. Therefore a broad Windows firewall/RST workaround was deliberately not added without stronger evidence.

### Same-port kernel TCP + raw FakeTCP experiments

More realistic testing showed that combining a kernel TCP listener and raw FakeTCP on the same public port can create conntrack/sequence-space ambiguity. This reinforced the architectural decision to remove the separate product Reality listener entirely instead of continuing to patch shared-port competition.

## Single-flow implementation lineage

The single-flow branch is `feat/single-flow-reality-faketcp`, PR #9.

Key implementation commits recorded in handoff history include:

- `e3825826...` scaffold bounded reliable bootstrap stream over FakeTCP;
- `16df0ac...` bind Reality-like bootstrap to the sole FakeTCP association;
- `f8b8bbf...` integrate Linux mux same-association TLS/admission;
- `060b1c3...` hand sequence state from bootstrap into post-bootstrap transport;
- `13e2d854...` convert Windows self-test startup to single-flow;
- `1cc45ada...` make Windows runtime own only one public FakeTCP process;
- `6a4a018...` gracefully disable the legacy Linux front in product startup;
- `62c654c...` make the Linux server manager own only one public FakeTCP listener;
- `282ef4df...` freeze the one-public-flow/no-HOL architecture contract;
- `1fc56555...` hard-disable standalone Linux Reality front in product path;
- `ee37379c...` add single-flow E2E qualification;
- `904ad0a8...` fix inherited DTLS handoff;
- later commits add TCP persona, two-client, and no-HOL qualification.

Current Windows product orchestration discovers the physical underlay, starts the unique FakeTCP process with Reality-like bootstrap arguments, waits for the ticket generated by that same process, then starts DTLS/LINK/TUN behind readiness gates. It does not execute `BuildBootstrap` in the product `Connect` path. `BuildBootstrap` remains only for diagnostic/backward-compatible tooling.

Current Linux product orchestration starts platform proxy, LINK mux, and a single `wbd-faketcp-mux` public listener. The mux receives the front certificate/key, server name, route key, account credentials, ticket directory and fallback target directly. The installed standalone `wbd-reality-front` binary is diagnostic/reference only and is not launched by `run_server()`.

## Qualification results as of 2026-08-29

### Single-flow E2E

`single-flow-e2e` has completed successfully after the DTLS inherited-fd fixes. The test checks that the client public capture has one SYN sequence/one 4-tuple, that Reality-like TLS admission completes on that association, and that DTLS payload subsequently traverses the same running FakeTCP process.

### TCP persona / fallback

`single-flow-tcp-persona` has completed successfully on recent heads. The single raw listener handles the intended Reality-like TLS bootstrap while retaining the configured fallback/decoy behavior for non-WBD TLS-like traffic.

### Two-client isolation

`single-flow-two-client` has completed successfully, establishing independent single-flow associations and ticket identities without reintroducing a standalone public TCP bootstrap.

### Post-switch no-HOL proof

The first `single-flow-no-hol` run was reported red solely because its final iptables counter parser used the wrong output columns. The product evidence itself already passed. Commit `12dfbde6f38ead63f175729042e50389c1c49b20` corrected only the test parser.

The corrected run `33243630006`, job `99077133630`, completed success. It deliberately dropped exactly the first post-ready client->server ACK|PSH payload, sent a later independent DTLS datagram 50 ms afterward, and required the later datagram to arrive before the earlier hole recovered.

Observed proof:

- drop count: exactly 1 post-ready payload;
- later datagram arrival: 0.224 ms after its send point;
- earlier dropped datagram recovery: 1002.478 ms after its original send point;
- `later_before_repair=1`;
- same-flow bootstrap marker present;
- wolfSSL client/server reached DTLSv1.3 with `TLS_AES_256_GCM_SHA384`.

This is direct evidence that the steady-state phase is not ordinary TCP HOL: the later independent datagram crossed a one-second earlier sequence hole immediately.

### Main CI flaky observation

On an earlier head the main `ci` workflow failed only in an existing game-lane random assertion (`in_first=3 in_dup=2, want 3/3`). Relevant `faketcp`, `realityfront`, `windowsruntime`, and `dtlsworker` packages passed. The next head reran the same main CI successfully without an unrelated code change. Do not modify the single-flow/FakeTCP design because of this game-lane flake unless it becomes deterministic.

## 100 Mbit qualification correction

A major qualification conflict was found on 2026-08-29: the historical `scripts/bench_mux_two_session_100m.py` still implemented the rejected dual-flow setup. It launched standalone `wbd-reality-front`, obtained tickets over separate ordinary TCP flows, then started independent FakeTCP clients. Therefore its RTT100 setup timeout did not qualify the new V2.3 architecture.

The old core is intentionally retained as a historical comparison reference. New commits:

- `be13d6fd85b48e57107b09cac978931d1eb9784e` add `scripts/bench_mux_two_session_single_flow_100m.py`;
- `3da850c8cb151768d760bbb3e624aa9b4a9c4f49` switch `bench_mux_two_session_100m_runner.py` to that single-flow core.

The new core reuses the mature qdisc setup, resource sampling, meter, probe, load generation, FEC comparison and result schema. Only admission/setup changes:

1. start LINK server and the one public FakeTCP mux;
2. mux itself receives Reality-like front certificate/identity/auth parameters;
3. start two FakeTCP clients, each with Reality-like bootstrap parameters on its sole association;
4. require two client and two server `WBD_SINGLE_FLOW_BOOTSTRAP_READY` markers;
5. read the two in-flow tickets;
6. start DTLS, LINK, then the unchanged 40/60/80 Mbit steady-state sweep;
7. activate 20% measurement loss only after both sessions are fully ready, preserving the historical capacity qualification boundary.

This keeps performance comparability while ensuring the benchmark is actually testing the product architecture.

## Current engineering policy

- Do not reintroduce standalone public Reality/TCP setup into Windows Connect or Linux run path.
- Do not make the steady-state payload use ordinary kernel TCP/TLS.
- Do not alter mature FakeTCP recovery/FEC merely to make a setup benchmark green.
- Qualification failures must first be classified as setup/bootstrap, mode-switch, DTLS/LINK, test-harness, or steady-state data-plane failures using deterministic logs/artifacts.
- A new physical Windows package should not be handed to the user until deterministic single-flow CI gates are green and the package is built from the exact qualified source head.

## Immediate next actions

1. Run the new single-flow `mux-load-100m` sweep at RTT20/RTT100, 20% measurement loss, 40/60/80 Mbit aggregate offered inner rates, FEC off and fixed 20:20.
2. If setup fails, inspect per-client FakeTCP bootstrap markers and mux bootstrap markers; fix only setup/concurrency/readiness.
3. If 40 Mbit steady-state fails after both sessions are ready, inspect artifacts before considering any transport change. The release target remains stable 40 Mbit aggregate on <=100 Mbit weak links.
4. Once qualification is deterministic, clean obsolete dual-flow build-only references from the current qualification workflows while retaining historical reference tests where useful.
5. Refresh Windows portable and Linux ARM64 release artifacts from the exact qualified single-flow head.
6. Update canonical `.wbd/handoff/current.json` and require `handoff-verify` success before ending the development task.
