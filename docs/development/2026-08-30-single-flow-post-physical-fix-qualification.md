# Single-flow post-physical-failure fixes and exact-head requalification

Date: 2026-08-30

## Purpose

This log is the durable recovery record after the first physical Windows 11/Npcap single-flow test exposed a startup failure after the earlier automated qualification. Chat history is not authoritative; this file plus `.wbd/handoff/current.json` are.

The product architecture remains the user-required terminal design:

- exactly one public TCP-shaped 4-tuple and one SYN/sequence lineage per VPN session;
- FakeTCP owns that flow from the SYN;
- a bounded reliable ordered setup phase on the same association carries real TLS 1.3 / Reality-like admission;
- the setup phase switches in-band, without FIN/RST/close/new SYN, to pinned wolfSSL DTLS 1.3 -> LINK/FEC/VPN datagrams;
- sustained payload must not inherit ordinary kernel-TCP cumulative-stream HOL;
- the mature FakeTCP ACK/SACK/recovery/FEC core remains frozen unless deterministic evidence requires a change.

## Physical evidence that reopened qualification

The user's real Ubuntu ARM64 server demonstrated that the single-flow design can complete on hardware: `WBD_SINGLE_FLOW_BOOTSTRAP_READY same_flow=1`, inherited DTLS `BOUND`, DTLS `ACCEPT_PASS`, DTLS READY and `WBD_LINK_MUX_SESSION_READY` were all observed on one session.

A later Windows 11/Npcap self-test failed before bootstrap with:

`wbd-faketcp handshake: faketcp: not ipv4/tcp`

The subsequent Reality-ticket timeout and `TerminateProcess: Access is denied` were downstream/cleanup noise. The first deterministic failure was a physical Npcap adapter packet that was not the inbound WBD TCP flow being passed into the strict handshake parser.

## Fixes after the physical failure

The feature line is `feat/single-flow-reality-faketcp`, PR #9. The exact requalified source head is:

`9bacaab9268f893349800316b93c4d692158d6ec`

Important post-failure fixes on this line include:

1. `cc70119819d77d18cfa7441d37610f529f6e3b32` — filter Windows Npcap ingress to the exact, unfragmented inbound WBD IPv4/TCP 4-tuple before the FakeTCP protocol state machine. UDP, ICMP, unrelated TCP, wrong peers/ports, fragments, malformed packets and outbound self-capture are ignored. Exact kernel RST copies remain observable for diagnostics.
2. `0d4301de1d1f6d3bcf9dfaf62cbe2e376c28dfb4` — Windows-only regression tests for noisy Npcap ingress, including VLAN cases.
3. `d2710aa51bfde939634614db6264fd5f750ae911`, `223b4aa5b04defa0540c50efa41fce6d9dc549e7`, `b0921ee943890a4680dcfa71282c1c0f96fd959c`, `00c19a756946d0d69457676a5fb3f54ef419b4af`, `33d0fa67cfd138b7aa2f977112b835dff5e79a93`, `7dae5adf43cfd0ee88349a715b41ca3208bf94fc` — make product and diagnostics readiness process-aware, fail immediately if the sole FakeTCP/bootstrap child exits, and reap already-exited children so a real startup error is not replaced by ticket polling or cleanup noise.
4. `2ce5825dc45223738ed42da1f87e57ba1cd3db91`, `f603da0f0785327eace9ce2b4a89a925d6b07565`, `136d8ff635f78107400b780d6936ed5566e95a3c` — allocate a fresh Windows dynamic-range raw source port per Connect, freeze it for that one public flow, and avoid immediate reuse across reconnects. This changes only connection metadata; it does not change FakeTCP recovery/FEC semantics.
5. `47f7ef59...`, `2b8a24d7...`, `f6a40e7a...` and follow-up CI work — add a protocol virtual wire, Windows-hosted execution and a full FakeTCP -> Reality-like bootstrap -> DTLS -> LINK qualification path.
6. `b4673c86c43601097ad602f4372a0fc0c353d787` plus stress-harness fixes through the current head — add repeated full-stack startup through a real namespace NAT and dirty client exits.

## Windows exact-head qualification

`windows-faketcp-persona` run `33260280825` completed success on a GitHub Windows Server 2025 hosted runner. Its Windows execution step runs, rather than merely compiles:

- Windows FakeTCP packet-persona tests;
- `cmd/wbd-faketcp` Windows backend tests, including noisy Npcap ingress filtering;
- `internal/realityfront` single-flow tests;
- `internal/singleflowvwire` `TestSingleFlowVirtualWireAdmissionThenNoHOL`;
- `internal/windowsruntime` controller/executor/readiness/dynamic-source-port tests;
- `internal/windowsdiag` readiness/early-exit tests;
- a Windows x64 FakeTCP executable build.

The run reports:

`WBD_WINDOWS_FAKETCP_PERSONA_PASS ... reality_like_tls=pass single_flow_vwire=pass no_hol=pass backend_tests=pass runtime_tests=pass diagnostics_tests=pass`

This is a real Windows-hosted protocol/runtime qualification. It is not claimed to be a physical Npcap driver + physical NIC injection test; that final environmental layer cannot be reproduced equivalently on the hosted runner under the project's Npcap installation constraints.

`windows-portable-bundle` run `33260280869` completed success for source head `9bacaab...`.

## Linux exact-head full-stack qualification

`single-flow-link-fullstack` run `33260280837` completed success for both FEC `off` and fixed `20:20`.

The test builds pinned wolfSSL 5.9.2 and the real WBD binaries, creates client/server Linux network namespaces, generates temporary front/DTLS certificates, captures the public wire, and runs:

FakeTCP one-flow SYN -> same-flow Reality-like TLS/auth bootstrap -> one-time ticket -> pinned wolfSSL DTLS 1.3 -> LINK ticket bind -> 20 application UDP echoes.

For the `off` case the captured-wire checker reported:

`SINGLE_FLOW_WIRE_INVARIANT_PASS {client_syn_packets:1, server_synack_packets:1, seq_spaces:1, fin_rst:0, ...}`

and the stack reported:

`SINGLE_FLOW_LINK_FULLSTACK_PASS fec=off public_flows=1 tls_bootstrap=1 dtls13=1 link=1 echo=20`

The server/client logs include `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`, DTLS `ACCEPT_PASS` / client READY and LINK session READY. The `20:20` matrix job also completed success.

## Reconnect/NAT qualification

`single-flow-startup-stress` run `33260280864` completed success.

It keeps one persistent server mux/LINK stack behind a three-namespace client -> MASQUERADE NAT router -> server topology and performs 20 complete rapid sessions. Each round uses a fresh dynamic source port, requires bootstrap/DTLS/LINK readiness and three application echoes, then SIGKILLs the real client FakeTCP/DTLS/LINK children, verifies their PIDs are gone and UDP endpoints are released, and immediately starts the next session without resetting the server or NAT topology.

All 20 rounds passed and the final marker was:

`SINGLE_FLOW_STARTUP_STRESS_PASS rounds=20 nat=1 dirty_exit=1 full_stack=1`

This gate directly addresses the earlier physical pattern where one connection could work and rapid subsequent attempts became unstable.

## Current exact-head matrix

For feature source head `9bacaab9268f893349800316b93c4d692158d6ec`, the following relevant workflows were observed completed/success:

- main `ci` — `33260280835`
- `windows-faketcp-persona` — `33260280825`
- `windows-tun-build` — `33260280850`
- `windows-ipv6-killswitch` — `33260280882`
- `windows-dtls-build` — `33260280884`
- `windows-portable-bundle` — `33260280869`
- `single-flow-e2e` — `33260280848`
- `single-flow-tcp-persona` — `33260280833`
- `single-flow-no-hol` — `33260280857`
- `single-flow-two-client` — `33260280856`
- `single-flow-startup-stress` — `33260280864`
- `single-flow-link-fullstack` — `33260280837`
- `faketcp-native` — `33260280873`
- `faketcp-pcap-20loss` — `33260280843`
- `faketcp-first-arrival` — `33260280860`
- `fullstack-first-arrival` — `33260280866`
- `mux-load-100m` — `33260280839`
- `linux-server-settings` — `33260280851`
- `linux-shared-port` — `33260280852`
- `linux-server-release` — `33260280813`

No relevant current-head workflow was observed failed; retired/old dual-flow or unrelated conditional matrices may be skipped by design.

## Exact-head artifacts and independent verification

Windows portable artifact:

- run: `33260280869`
- artifact id: `9717076881`
- Actions ZIP SHA-256: `de2009443f4177ef3138f2d9b4c8433b53aa6ce8bb49f4cae126af660fb9ac9c`
- extracted `wbd.exe` SHA-256: `e9cd50247ddb66f78d7e913f0cf8386bc38d0711c96640084968dc6fd46c9c53`
- format: PE32+ x86-64

Linux ARM64 artifact:

- run: `33260280813`
- artifact id: `9717108868`
- Actions ZIP SHA-256: `952180baa6b81134e6f4095724ae4a232dbd80b68d16ed354846fdb361a495cd`
- extracted `wbd-linux-server-arm64.tar.gz` SHA-256: `f6399478cd82931bd87080a3b3cef338678bd01c9a5d45f48e58154130efe3a5`
- the bundled `.sha256` file matches the extracted tarball;
- representative DTLS/FakeTCP/LINK binaries are ELF AArch64;
- the manager help smoke executes and states the V2.3 one-public-raw-FakeTCP/same-association Reality-like model.

## Release interpretation

Automated upstream/downstream qualification is now complete for the current source head, including a real Windows hosted OS for Windows-specific packet/backend/runtime logic and a real Linux network-stack full-data-path environment for one-flow bootstrap/DTLS/LINK/wire invariants/reconnect stress.

The only remaining environment that is not equivalent in CI is a physical Windows 11 Npcap driver + physical NIC + the user's real NAT/ISP path. A physical retest is therefore final environmental qualification, not first-line debugging. If it still fails, the first deterministic marker must be logged and reproduced in sandbox before changing mature TCP-like/FEC internals.
