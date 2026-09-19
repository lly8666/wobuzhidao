# 2026-08-31 current-head single-flow requalification

## Purpose

The exact-tested single-flow candidate recorded by handoff sequence 68 was `ed7b717d74ffa6fc823685e37a58432d46630ca6`. The feature branch subsequently advanced by many commits, mainly Logical Tunnel, raw-IP backend, Wintun/TUN and gateway work. Old green runs must not be used to qualify the newer source. This log records the requalification of the current branch and distinguishes production defects from stale test/qualification fixtures.

Frozen architectural constraints for this work:

- one public TCP-shaped 4-tuple and one SYN lineage **per Transport Lane**;
- initial reliable ordered FakeTCP phase carries TLS 1.3 / Reality-like admission;
- no second SYN/FIN/RST transition between admission and sustained transport within one lane;
- the same FakeTCP association switches in-band to pinned wolfSSL DTLS 1.3 and LINK/FEC datagrams;
- sustained transport must not inherit kernel TCP stream HOL;
- a Logical Tunnel may own 1..N independent Transport Lanes, but each lane independently obeys the one-flow invariant;
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

## Finding 4: current single-flow bootstrap, DTLS and LINK admission reach READY; old LINK echo fixture violates the current application-payload contract

Requalification then moved to feature head `0fdd656d8ad5f69ca695d4653c728457ebe0a477`.

`single-flow-link-fullstack` run `33326620684` failed only after all protected transport stages had passed. Both FEC variants reached:

```text
SINGLE_FLOW_LINK_STAGE_PASS stage=bootstrap_v2
SINGLE_FLOW_LINK_STAGE_PASS stage=dtls13
SINGLE_FLOW_LINK_STAGE_PASS stage=link
```

The server then rejected the first old echo probe with:

```text
application datagram is neither M6A raw-IP nor platformproxy frame
```

Source review confirmed the production contract is correct:

- `wbd-link-proxy` carries the local application datagram as LINK application payload;
- `wbd-link-server-mux` accepts a formal raw IPv4 M6A packet or a platformproxy frame;
- an arbitrary ASCII string such as `SINGLE_FLOW_LINK_ECHO_00` is not a valid logical-tunnel payload and must be rejected.

Therefore the red fullstack run is evidence that the old qualification fixture is stale, not evidence that FakeTCP, Reality-like bootstrap, DTLS 1.3 or LINK admission is broken. The correct replacement is a TUN/raw-IPv4 qualification through the raw-IP backend, while retaining the public pcap one-SYN/same-flow assertions.

## Finding 5: two-client qualification proves identity separation before hitting the same stale arbitrary-payload fixture

`single-flow-two-client` run `33326620631` also reached the current transport contract before failing. For both FEC modes the workflow proved:

- two same-account clients completed V2 single-flow bootstrap;
- the clients received distinct TunnelIDs;
- the clients received distinct server-assigned `/32` tunnel addresses;
- tickets were distinct;
- both wolfSSL DTLS 1.3 handshakes completed;
- two independent LINK sessions became READY and consumed the tickets.

It then sent arbitrary `A-...` / `B-...` UDP strings directly into LINK and hit the same formal payload rejection. The replacement qualification must use two real raw-IP/TUN clients, preserve distinct server-assigned leases, and prove both can use inner TCP source port `40000` simultaneously to the same Internet target/port.

## Finding 6: the existing raw-IP E2E already contains the correct data-plane skeleton, but still has one pre-V2.4 hard-coded address

`.github/workflows/single-flow-rawip-e2e.yml` already exercises:

```text
single-flow FakeTCP / Reality-like V2
  -> wolfSSL DTLS 1.3
  -> LINK
  -> wbd-tun
  -> raw-IP gateway
  -> Linux namespace Internet target
```

and verifies DNS-style UDP, generic UDP and TCP. This is the correct skeleton for the LINK fullstack replacement.

However the workflow currently assigns `10.66.0.2/30` manually to its client TUN. V2.4 requires the client to consume the server-issued `address4` lease from `tunnel-config.json`. New qualification must parse and apply that issued `/32`; copying the old hard-coded address would preserve a stale architectural shortcut.

The current raw-IP server fixture still uses the historical per-session netns backend. It remains useful as correctness evidence, but V2.4 explicitly says that per-session netns/double-NAT is not the selected final Windows server topology. Do not expand that backend as new product architecture; shared-TUN/one-host-NAT remains the selected direction.

## Finding 7: main CI red was a V2.3 invariant baked into the test, not a production regression

On head `0fdd656d...`, main CI run `33326620656` completed all Go tests successfully. The only failing unit test was `test_v23_single_flow_architecture_invariants_are_persisted`, which still required the old whole-VPN invariant:

```text
exactly one public TCP-shaped raw/FakeTCP association
```

V2.4 intentionally changed the invariant to **per Transport Lane**. A Logical Tunnel may use multiple independent lanes, while every lane still owns exactly one public FakeTCP SYN lineage / 4-tuple / sequence space and never creates a second WBD payload SYN between Reality-like bootstrap and steady DTLS/FEC payload.

Commit `6397ae0ff9aa7c6c65dd8200f3ca883a8a88e8ef` (`test: align handoff contract with v2.4 per-lane single-flow`) updates this test contract only. It does not change transport behavior or relax no-HOL.

## Finding 8: Windows and Linux have strong isolated hosted gates, but release still lacks one combined Windows-runtime -> Linux-server gate

Existing hosted evidence includes:

- Windows `windows-faketcp-persona`: Windows builds/runs FakeTCP backend tests, Reality-like single-flow tests, virtual-wire no-HOL, runtime and diagnostics tests;
- Windows `windows-tun-admin-smoke`: real hosted Windows Wintun adapter, real route/address/NRPT state, stale-state recovery and real bidirectional Wintun dataplane;
- Linux `windows-rawip-gateway`: two Windows-style TUN sessions through real Linux raw-IP gateway/firewall paths using both iptables and nft;
- Linux single-flow/raw-netns workflows: real raw FakeTCP + pinned wolfSSL DTLS + LINK and raw-IP Internet namespace paths.

These are necessary but not sufficient for the user's delivery condition. Before another package is offered, add a combined automated gate that executes the actual Windows runtime-side binaries and a Linux server-side stack in one qualification topology (prefer hosted Windows + WSL2/Linux when privileged raw networking is available; otherwise use an explicit cross-platform synthetic packet-I/O bridge while preserving the real single-flow/Reality/DTLS/LINK state machines, and label that limitation honestly).

Npcap Free Edition licensing/install constraints remain in force: do not silently redistribute or silently install Npcap merely to make CI green.

## Current qualification policy

No Windows or Linux package is to be delivered until the exact current source candidate has current green evidence on both hosted Windows and hosted Linux/raw-netns gates **and** the combined Windows-runtime/Linux-server qualification gate is green. Older green evidence is historical only.

Immediate next work:

1. replace the stale arbitrary-payload `single-flow-link-fullstack` probe with dynamic-lease raw-IP/TUN traffic while retaining the one-public-flow pcap proof;
2. replace `single-flow-two-client` arbitrary echo with two dynamic-lease raw-IP/TUN sessions and same inner source-port isolation proof;
3. add the combined Windows-runtime/Linux-server gate;
4. rerun current-head CI and release builds;
5. only after all mandatory gates are green, create packages;
6. write and verify canonical handoff sequence 71 before ending the workstream.
