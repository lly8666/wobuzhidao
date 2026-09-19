# 2026-09-01 single-flow startup qualification

This log is a durable recovery record for the qualification work that was repeatedly interrupted by chat/session restoration. It supplements `docs/development/SINGLE_FLOW_DEVLOG.md`; it does not replace live GitHub Actions state or the canonical handoff.

## Frozen requirement

The public transport is one TCP-shaped flow only:

1. one client/server 4-tuple and one SYN lineage;
2. first phase is Reality-like TLS 1.3 bootstrap over a bounded reliable bootstrap stream carried by FakeTCP;
3. authentication produces the ticket/logical-tunnel assignment;
4. the same FakeTCP association then carries wolfSSL DTLS 1.3 and LINK/application datagrams;
5. no second public SYN/connection is allowed between bootstrap and DTLS;
6. sustained payload must retain the existing TCP-like/FakeTCP no-HOL semantics. The established TCP-like recovery/data-plane implementation is frozen unless a deterministic qualification failure proves a defect in it.

## Windows physical failure that motivated the current qualification

The earlier physical Windows run reached Npcap startup but failed with:

- `wbd-faketcp handshake: faketcp: not ipv4/tcp`
- the single-flow Reality ticket was never written.

Latest feature-branch Windows Npcap receive code no longer treats unrelated capture traffic as a fatal handshake error. ARP/IPv6, non-TCP IPv4 and irrelevant TCP tuples are skipped, and a noisy-capture unit test covers IPv6 + UDP noise before a valid WBD TCP packet.

This is considered fixed in source, but physical Npcap injection remains a separate qualification boundary from hosted Windows CI.

## HEAD `fa5e8824687788d8f9e3b9df3533151fe3078ba4`

Most single-flow gates were green, including `ci`, `single-flow-e2e`, `single-flow-no-hol`, `single-flow-tcp-persona`, `faketcp-native`, `faketcp-first-arrival`, `faketcp-pcap-20loss` and `fullstack-first-arrival`.

`single-flow-startup-stress` run `33520437316` failed before exercising the network loop. The harness attempted to open `/tmp/sfstress/client-tunnel.json` as the hosted runner user even though the client process intentionally writes authenticated state as root with restrictive permissions. The first deterministic error was `PermissionError: [Errno 13] Permission denied`.

This was a test ownership bug, not a product failure.

## Commit `865d6f4467cba3af105d1ef05e186be1015843b5`

`ci: read protected single-flow tunnel state as root`

The stress harness now validates protected product state as root instead of weakening product file permissions. A separate runner-owned write check ensures the harness diagnostics directory itself remains writable.

Run `33522266464` then entered the real single-flow stack. Artifact `9806226148` proved the following in round 1:

- client: `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1 logical_tunnel=1`;
- server: same-flow bootstrap accepted on the same FakeTCP association;
- wolfSSL client/server DTLS 1.3 handshake completed;
- LINK client emitted `WBD_LINK_READY`;
- LINK server emitted `WBD_LINK_MUX_SESSION_READY` and logical-tunnel binding.

The run still failed because the stress probe sent arbitrary UDP bytes directly to LINK. LINK correctly rejected those bytes with `application datagram is neither Game envelope, M6A raw-IP nor platformproxy frame`.

Again, this was a harness contract error, not a product data-plane failure.

## Commits `97141246da6db494a172124e81dec05297408266` and `4f39c524a58fbd8c78d05cdf5ab861651ea176f6`

The stress environment was upgraded to exercise the supported PlatformProxy application data plane:

- build and start `wbd-platform-proxy-server` on server loopback;
- place `wbd-link-server-mux` in front of that service;
- run a real UDP echo target behind PlatformProxy;
- construct the formal 44-byte `WBDP` `KindUDPDatagram` frame with nonzero FlowID and a concrete IPv4 peer;
- validate returned FlowID, peer, payload length and payload bytes.

Run `33523079002`, artifact `9806571982`, proved round 1 product behavior was completely successful:

- single-flow bootstrap ready;
- DTLS server `ACCEPT_PASS` and client `CONNECT_PASS`;
- LINK ready;
- server selected `backend=platformproxy`;
- LINK counters: `tx=3 rx=3 drop=0`;
- probe: `STRESS_PLATFORMPROXY_ECHO_PASS round=1`;
- FakeTCP counters showed bootstrap + DTLS + application traffic delivered without retransmission in that round.

The job still exited immediately after round 1. Evidence showed this occurred after the successful probe, in the harness dirty-exit phase. The script performed unconditional sequential `SIGKILL` operations under `set -e`; killing an upstream child can make a downstream child exit before its own explicit kill, causing the later `kill` command to return nonzero and incorrectly fail the run.

## Commit `21d17bbc047277bd362784c710240fc9e2bcaa2a`

`ci: make dirty single-flow cleanup idempotent`

Dirty-exit qualification was changed to match the actual resource-reuse contract:

- `SIGKILL` is idempotent: an already-exited child is success;
- the harness waits for the client namespace product endpoints `127.0.0.1:45101`, `:46101`, and `:47101` to be released;
- stale wrapper/zombie timing is not treated as an occupied product resource;
- failure is reported only if a product UDP endpoint remains bound and would prevent the next reconnect.

The same 20-round NAT + hard-client-exit + full-stack stress is running on this HEAD. Do not classify it green until the live Action is completed successfully.

## Windows/Linux qualification boundary

The repository already contains `windows-linux-single-flow.yml`; it is not merely a cross-compile check.

Its Windows 2022 job executes native Windows Go code and emits a Windows single-flow wire vector. Linux consumes that exact vector through the server state machine, checking Reality-like marker classification, server association, one SYN and one sequence-space lineage. A Linux raw/netns full-stack job additionally exercises Reality-like bootstrap -> DTLS 1.3 -> LINK/TUN/raw-IP. The aggregate marker explicitly records `physical_npcap=0` because hosted runners do not provide the licensed/installed physical Npcap boundary.

A separate `windows-npcap-physical.yml` exists for an elevated self-hosted Windows runner tagged `wbd-npcap`, with Npcap already installed normally. Hosted CI must never be represented as equivalent to that physical capture/injection gate.

## Delivery rule

Do not release a Windows/Linux package pair from this work until one final source HEAD has, at minimum:

1. completed-success 20-round `single-flow-startup-stress` with valid PlatformProxy application traffic and dirty exits;
2. completed-success `windows-linux-single-flow` on the same source HEAD;
3. no red core single-flow gates (`ci`, single-flow E2E/no-HOL/TCP persona, FakeTCP recovery/arrival gates as applicable);
4. completed-success Windows portable bundle and Linux server release from that same source HEAD;
5. artifacts downloaded and hashes/architecture checked before delivery.

Physical Npcap qualification remains separately identified. Never fabricate it when no matching self-hosted runner/profile is available.
