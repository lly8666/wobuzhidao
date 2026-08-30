# 2026-08-31 current-head single-flow requalification

## Purpose

The exact-tested single-flow candidate recorded by handoff sequence 68 was `ed7b717d74ffa6fc823685e37a58432d46630ca6`. The feature branch subsequently advanced by many commits, mainly Logical Tunnel, raw-IP backend, Wintun/TUN and gateway work. Old green runs must not be used to qualify the newer source. This log records the requalification of the current branch and distinguishes production defects from stale test/qualification fixtures.

Frozen architectural constraints for this work:

- one public TCP-shaped 4-tuple and one SYN lineage;
- initial reliable ordered FakeTCP phase carries TLS 1.3 / Reality-like admission;
- no second SYN/FIN/RST transition between admission and sustained transport;
- the same FakeTCP association switches in-band to pinned wolfSSL DTLS 1.3 and LINK/FEC datagrams;
- sustained transport must not inherit kernel TCP stream HOL;
- mature FakeTCP recovery/FEC/TCP-like data plane is not to be changed without deterministic evidence.

## Resume state

At resume, `feat/single-flow-reality-faketcp` was at `9a209c4fed32e6ce8cad378c15ae47fb7031bb27`, substantially ahead of the sequence-68 exact-tested commit. Exact-head Actions showed many red runs, so no package from the older green commit was considered releasable.

## Finding 1: raw-IP metadata unit fixture was stale

Main CI initially failed `TestRawIPBackendReceivesSessionMetadataBeforePayload` with `raw-IP lane lacks Logical Tunnel binding`.

Production inspection showed the server mux was already enforcing the intended current contract: a raw-IP backend cannot start without a valid Logical Tunnel binding, and v2 tunnel metadata must be sent before payload. The failing test still supplied an old short SID-only fixture.

Commit `747fc887c04a094f91976875a5e9cc6510400fc7` (`test: bind raw IP metadata to logical tunnel`) updated only the test:

- create a full 16-byte TunnelID;
- create an IPv4 /32 lease;
- install a `realityfront.TicketBinding` in `peerTunnelBindings`;
- assert that the first backend datagram is v2 TunnelMeta with the exact TunnelID and lease;
- assert that the second datagram is the actual M6A payload.

No production data-plane code changed. On that exact head, Go tests passed; the remaining main-CI failure was the intentionally stale handoff metadata.

## Finding 2: Windows-style raw-IP qualification was blocked by its own TIME_WAIT

The hosted qualification reached working production traffic before failing:

- both clients completed DNS-style UDP and generic UDP;
- both clients completed simultaneous identical-tuple TCP in round 1;
- gateway sessions were created and cleaned with zero production drops;
- failure happened only when the probe script reused local `10.66.0.2:40000` in round 2 and received `EADDRINUSE`.

This was a probe-socket lifecycle problem, not a gateway defect. Commit `09706ef34ec92817420f4f41f47fa312cd53a1fb` (`test: allow exact TCP tuple reconnect in raw IP qualification`) added `SO_REUSEADDR` to the probe before binding while preserving the qualification invariant that round 2 reuses the exact same inner address and TCP source port.

The exact-head `windows-rawip-gateway` run subsequently passed.

## Finding 3: old single-flow E2E never reached the product handshake

The next exact-head product-looking red gate was `single-flow-e2e`. Artifact inspection showed:

- `wbd-faketcp` exited before sending a packet because the workflow omitted the current mandatory `--reality-installation-id` and `--reality-tunnel-config-out` arguments;
- mux only printed its server-ready line;
- DTLS never started meaningfully;
- the pcap contained only its 24-byte header and zero packets.

Therefore this was not evidence that current single-flow TLS/DTLS failed. The workflow was still exercising the pre-Logical-Tunnel V1 CLI contract.

A second stale fixture was discovered in `scripts/single_flow_decoy.go`: the fallback helper still used V1 `BootstrapServerSimple`, while the product client now uses encrypted SingleFlow V2 admission with installation identity and Logical Tunnel configuration.

### V2 fixture upgrades

Commit `a3e70d0990519724673c893652cb89bc13b4e5a8` (`test: upgrade single-flow decoy to logical tunnel v2`) changed only the test decoy:

- create an independent Logical Tunnel manager;
- call `BootstrapServerSimpleV2`;
- return a valid ticket + tunnel ID + /32 lease;
- log explicit `logical_tunnel_v2=1` evidence.

Commit `61e57c244c34bbb6f1b0e92de5cdbc28e2cc3d32` (`test: centralize current single-flow e2e qualification`) added `scripts/qualify_single_flow_e2e.sh`. The qualification is now repository code instead of hundreds of lines embedded in YAML. Its intended invariants are:

1. primary path uses one public tuple `client:41001 -> server:443`;
2. client sends valid V2 installation identity and persists ticket + tunnel config;
3. tunnel config must contain a 32-hex TunnelID, IPv4 /32 and non-empty IPv4 routes;
4. the same association proceeds to pinned wolfSSL DTLS 1.3;
5. 20 bidirectional echo probes must pass;
6. pcap must show one unique SYN sequence lineage and no second public source port;
7. wrong-marker traffic stays on the original raw flow and is proxied to the TLS decoy;
8. wrong-marker fallback must not start a WBD DTLS worker.

Commit `5bc25ee8bc4564d1e1c69dd40151344c57b81351` (`test: rebase single-flow e2e on current v2 contract`) rewrote `.github/workflows/single-flow-e2e.yml` to build the current binaries and pinned wolfSSL, generate test certificates, call the centralized qualification script, expose failure logs, and preserve pcap/tunnel evidence.

These three commits are test/qualification changes. They do not alter the public-flow protocol or mature TCP-like recovery logic.

## Current qualification policy

No Windows or Linux package is to be delivered until the exact current source candidate has current green evidence on both hosted Windows and hosted Linux/raw-netns gates. Older `ed7b...` green evidence is historical only.

At the time this log entry was created, the new `5bc25ee8...` Actions matrix had started but the rewritten single-flow E2E and main Go test job were still running. Results and any subsequent fixes must be appended here before release/handoff.
