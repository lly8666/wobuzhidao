# 2026-08-29 — Single-flow exact-head qualification closure

This note is a durable recovery record for the WBD V2.3 single-public-flow work. Chat history has been interrupted repeatedly, so this file records the exact branch, commits, test authority, failures already retired, release artifacts, and next action.

## Product constraint frozen for this work

The public WBD session is exactly one TCP-shaped 4-tuple and one SYN lineage from connect to disconnect.

- FakeTCP owns the public sequence space from the sole SYN/SYNACK/ACK.
- The first bounded phase of that same FakeTCP association carries real TLS 1.3 / Reality-like admission traffic.
- The ClientHello persona is pinned to uTLS Firefox 120; WBD modifies only the normal 32-byte TLS 1.3 compatibility SessionID so it can carry the route-classifier marker.
- There is no ordinary kernel-TCP Reality bootstrap followed by a second FakeTCP SYN.
- After admission the same FakeTCP association switches in-band to pinned wolfSSL DTLS 1.3, LINK, optional fixed systematic FEC 20:20, then VPN datagrams.
- The reliable ordered bootstrap behavior is setup-only. Post-switch datagrams must not inherit ordinary TCP cumulative-stream head-of-line blocking.
- Mature FakeTCP recovery, sequence, ACK/SACK, retransmission, FEC, and steady-state data-plane semantics remain frozen unless deterministic evidence specifically requires a change.

The retired dual-public-flow design remains historical diagnostic context only.

## Recovery state at start of this turn

Canonical branch: `dev/wbd-raw-fec-v2`.

Active implementation branch / PR:

- branch: `feat/single-flow-reality-faketcp`
- draft PR: #9, `Single-flow Reality-like bootstrap over FakeTCP`
- feature HEAD when this turn resumed: `7bdd0ebbdc4ca91a3d70368acdfe4661b8c1f820`

The feature branch was already substantially beyond the older sequence-61 handoff. Live inspection confirmed the following were already implemented and green on `7bdd0ebb...`:

- single-flow E2E;
- one public SYN / one public 4-tuple assertion;
- same-flow wrong-marker fallback to the TLS decoy;
- single-flow two-client isolation;
- post-switch no-HOL qualification;
- Firefox 120 public TLS persona gate;
- Windows 11-family raw TCP/IP presentation gate;
- Windows portable bundle;
- Linux release;
- FakeTCP native / first-arrival / pcap loss qualification;
- 100 Mbit two-session single-flow capacity characterization.

## Reality-like persona status

The current single-flow client uses `uTLS v1.6.5` with `HelloFirefox_120` on the already-established FakeTCP bootstrap stream. The WBD route marker is derived from the preset ClientHello random and replaces only the 32-byte TLS 1.3 compatibility SessionID contents; uTLS remains responsible for the Firefox cipher, extension, GREASE, group and ALPN persona.

The `single-flow-tcp-persona` workflow performs capture-based comparison against the pinned Firefox 120 expectations. On the exact qualified source head described below, run `33245026974`, job `99080911580`, completed `success`.

For the physical Windows client, presentation-only packet persona code gives the raw association a coherent Windows-family outer TCP/IP appearance while preserving mature transport semantics. `windows-faketcp-persona` run `33245027146`, job `99080911957`, completed `success` on the same source head.

No FakeTCP recovery/FEC algorithm was modified in this turn.

## Post-switch no-HOL proof

`single-flow-no-hol` run `33245026920`, job `99080911483`, completed `success` on the exact qualified source head.

The gate intentionally creates a hole in an earlier post-ready FakeTCP ACK|PSH payload and requires a later independent DTLS datagram to cross the same association before the earlier sequence hole is repaired. This is the critical proof that a single TCP-shaped sequence lineage does not mean ordinary kernel-TCP stream HOL after the setup-to-DTLS switch.

## One-flow E2E proof

`single-flow-e2e` run `33245027009`, job `99080911711`, completed `success`.

Its product path uses one FakeTCP-owned public association. The gate verifies the one-SYN lineage carries Reality-like TLS admission and then DTLS payload without a FIN/new SYN boundary. The wrong-route-marker case remains on the same raw ingress and is forwarded to the configured TLS decoy rather than requiring a standalone public Reality listener.

`single-flow-two-client` run `33245026936` also completed successfully for both FEC-off and fixed-20:20 variants, proving independent ticket/session identity for two simultaneous single-flow clients.

## Qualification harness cleanup performed in this turn

Substantive feature commit:

`48e9fd45790a4c85d012aadb7a2ea50d3ad95479` — `ci: remove dual-flow remnants from mux load gate`

Only `.github/workflows/mux-load-100m.yml` changed. No product executable source changed.

The cleanup removed three misleading historical remnants from the authoritative V2.3 capacity gate:

1. the pull-request path filter no longer treats the retired dual-flow `scripts/bench_mux_two_session_100m.py` as an input;
2. `py_compile` now checks the actual single-flow core `scripts/bench_mux_two_session_single_flow_100m.py` plus its runner;
3. the gate no longer builds an unused standalone `wbd-reality-front` executable.

The runner already imports `bench_mux_two_session_single_flow_100m`; therefore this commit makes the workflow definition accurately reflect the architecture it was already executing. It does not change FakeTCP/TLS/DTLS/LINK wire behavior.

## Exact-head Actions qualification

Authoritative source head for this turn: `48e9fd45790a4c85d012aadb7a2ea50d3ad95479`.

All non-conditionally-skipped workflows observed for this head completed successfully. Key runs:

- main `ci`: `33245027098` — success;
- `single-flow-e2e`: `33245027009` — success;
- `single-flow-tcp-persona`: `33245026974` — success;
- `windows-faketcp-persona`: `33245027146` — success;
- `single-flow-no-hol`: `33245026920` — success;
- `single-flow-two-client`: `33245026936` — success;
- `faketcp-native`: `33245026951` — success;
- `faketcp-pcap-20loss`: `33245026966` — success;
- `faketcp-first-arrival`: `33245026977` — success;
- `fullstack-first-arrival`: `33245027029` — success;
- `linux-shared-port`: `33245026923` — success;
- `windows-dtls-build`: `33245027001` — success;
- `windows-tun-build`: `33245027020` — success;
- `windows-ipv6-killswitch`: `33245027012` — success;
- `windows-portable-bundle`: `33245026925` — success;
- `linux-server-release`: `33245027019` — success;
- `game-lane-fullstack`: `33245026931` — success;
- `openwrt-fullstack-one-shot`: `33245027067` — success;
- `openwrt-tcp-tproxy`: `33245027167` — success;
- `mux-load-100m`: `33245027049` — success (`bench 20`, `bench 100`, and `aggregate` all success);
- `handoff-verify`: `33245027059` — success for the then-current handoff contract.

Workflows reported as skipped on this head were path/condition-gated legacy or AB jobs, not failures.

## Exact-head 100 Mbit / RTT100 characterization

Run: `mux-load-100m` `33245027049`.

RTT100 job: `99080911916`, success. The full 40/60/80 Mbit aggregate-inner sweep completed; there was no FakeTCP bootstrap/READY timeout.

Compact RTT100 results:

| Offered inner | FEC | delivery | delivered active goodput | p99 one-way | plaintext expansion |
|---:|---|---:|---:|---:|---:|
| 40 Mbit/s | off | 0.802928 | 32.112 Mbit/s | 50.961 ms | 1.000x |
| 40 Mbit/s | 20:20 | **1.000000** | **39.994 Mbit/s** | 63.551 ms | 2.137x |
| 60 Mbit/s | off | 0.797920 | 47.875 Mbit/s | 145.248 ms | 1.000x |
| 60 Mbit/s | 20:20 | 0.742880 | 44.573 Mbit/s | 346.912 ms | 2.095x |
| 80 Mbit/s | off | 0.585443 | 46.834 Mbit/s | 224.817 ms | 1.000x |
| 80 Mbit/s | 20:20 | 0.732629 | 58.608 Mbit/s | 1013.063 ms | 2.094x |

Interpretation:

- The release operating point remains qualified: aggregate-inner 40 Mbit/s with fixed 20:20 FEC delivered 100% at ~39.994 Mbit/s under RTT100/20% measurement loss.
- 60/80 Mbit are headroom characterization, not release requirements.
- Fixed 20:20 expands plaintext to roughly 2.1x. Thus 60 Mbit inner implies roughly 126 Mbit/s before remaining framing/transport overhead, and 80 Mbit implies roughly 168 Mbit/s, both above the modeled 100 Mbit link. Their queueing/delivery collapse is therefore capacity oversubscription evidence, not justification to retune the mature FakeTCP recovery core.
- The historical RTT100 failure mode where one FakeTCP client never became READY is no longer present on this exact head.

RTT100 evidence artifact:

- artifact `mux-load-rtt100-sweep`
- artifact ID `9712599094`
- Actions ZIP SHA-256 `f0c1b5c4e34fa1a9b5c2479be5fdcc80ac20ca813661d52cf695fdf94869c90f`.

## Exact-head release artifacts

The Actions PR workflows checkout a generated merge commit for PR qualification, so artifact names contain merge SHA `6d6654c767645eb6a65024b335dd40da71b7994e`. Their `workflow_run.head_sha` remains the authoritative feature source SHA `48e9fd45790a4c85d012aadb7a2ea50d3ad95479`.

Windows portable:

- run `33245026925` — success;
- artifact ID `9712592173`;
- artifact name `wbd-windows-portable-6d6654c767645eb6a65024b335dd40da71b7994e`;
- Actions ZIP digest `sha256:c42f1fbcef7cc4c4b972023f1d398cfe767cf192bf51c29838481a9fc65a06d4`.

Linux ARM64:

- run `33245027019` — success;
- artifact ID `9712579571`;
- artifact name `wbd-linux-server-arm64-6d6654c767645eb6a65024b335dd40da71b7994e`;
- Actions ZIP digest `sha256:36a43a810d221e20e2ff0b550b7c266d3623b13ec7bd2e5a73165df837fc729d`.

Linux AMD64:

- artifact ID `9712589155`;
- artifact name `wbd-linux-server-amd64-6d6654c767645eb6a65024b335dd40da71b7994e`;
- Actions ZIP digest `sha256:b3deca6c6531179ae655cfc7a7f29a0e046c2abf67a33a023a48a4291b8073cd`.

## Retired hypotheses / do-not-repeat work

Do not return to these directions without new deterministic evidence:

- separate ordinary-TCP Reality bootstrap followed by a second FakeTCP public connection;
- sustained VPN payload over ordinary TLS/TCP;
- changing mature FakeTCP retransmission/FEC merely because 60/80 Mbit 20:20 oversubscribes a 100 Mbit link;
- broad Windows RST firewall workarounds: prior NAT/RST sandbox A/B showed FakeTCP+DTLS can complete with RSTs allowed or suppressed;
- treating historical dual-flow load scripts as V2.3 qualification evidence.

## Remaining work after this closure

The automated single-flow architecture is now qualified on the exact source head above. The next highest-value task is physical qualification using the exact Windows x64 and Linux ARM64 artifacts from that head on the user's real Windows 11/Npcap + physical network/NAT/ISP + Ubuntu ARM64 path.

Expected physical evidence should show one FakeTCP process performing `WBD_SINGLE_FLOW_BOOTSTRAP_READY`, then DTLS READY, LINK READY, TUN/routes and probes without a second public Reality bootstrap process/connection.

If physical E2E exposes a deterministic failure, diagnose the first missing marker/boundary before changing transport semantics.

Known secondary hardening issues remain separate from the single-flow data plane:

- PowerShell 5.1 UTF-8 BOM can corrupt route-state JSON for Go `json.Unmarshal`;
- stopping an already-exited LINK child should be idempotent rather than surfacing `TerminateProcess: Access is denied`;
- account/Reality credentials should eventually be moved off process argv on Linux to avoid exposure through process/status listings.

Do not mix these secondary fixes into the first physical single-flow qualification unless they become the first deterministic blocker.
