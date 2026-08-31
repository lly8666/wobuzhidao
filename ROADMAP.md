# Roadmap

> **Status: V3 SINGLE-PUBLIC-FLOW ACTIVE.** The release architecture is governed by PROJECT_CONSTITUTION.md and ADR-0011. One WBD session owns exactly one public raw FakeTCP 4-tuple from its first SYN through Reality-like TLS setup, encrypted phase switch and steady-state DTLS/FEC/LINK datagrams. The qualified FakeTCP steady-state transport remains frozen unless deterministic evidence requires a change.

## Product goal

Deliver a personal weak-network VPN for OpenWrt/Linux ↔ Linux or Windows with:

- one public TCP-shaped raw/FakeTCP connection per session;
- real TLS 1.3 Reality-like/browser-like setup during the bounded first phase of that same association;
- no second SYN/socket/4-tuple between admission and the data plane;
- no ordinary kernel-TCP sustained payload and therefore no ordinary-TCP HOL dependency;
- pinned wolfSSL DTLS 1.3 steady-state encryption;
- optional fixed systematic FEC `off` or `20:20` for the release wire;
- packet-preserving LINK/session isolation;
- OpenWrt TPROXY and Windows TUN/Wintun-class platform capture.

The current weak-network qualification ceiling remains **100 Mbit/s physical link capacity**. The release operating point remains 40 Mbit/s aggregate inner traffic on <=100 Mbit/s weak links.

## V3 authoritative session sequence

```text
one raw FakeTCP SYN / SYN-ACK / ACK
        ↓
same public 4-tuple and FakeTCP sequence space
        ↓
bounded ordered bootstrap presentation
        ↓
real TLS 1.3 Reality-like ClientHello / handshake
        ↓
encrypted shared-account admission + per-session identity/ticket
        ↓
encrypted SWITCH_REQ / SWITCH_ACK
        ↓
destroy ordered bootstrap state
        ↓
same FakeTCP association, no FIN/RST/close_notify/new SYN
        ↓
DTLS 1.3
        ↓
FEC off or fixed 20:20
        ↓
LINK packet/datagram traffic
```

The setup byte-stream exists only while TLS requires ordering. After the switch barrier, later independent datagrams must be able to complete while an earlier unit is lost or delayed.

## Current milestones

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V3-M0 | replace V2 dual-public-flow bootstrap with one-public-flow law | **IMPLEMENTED IN EXPERIMENTAL V3; ADR-0011 ACCEPTED** |
| V3-M1 | raw FakeTCP-owned Reality-like TLS 1.3 bootstrap + encrypted switch | **IMPLEMENTED; STRONG LINUX/NAT E2E GREEN** |
| V3-M2 | Windows single-flow client orchestration/readiness | **IMPLEMENTED; HOSTED WINDOWS TEST/BUILD GREEN, PHYSICAL QUALIFICATION IN PROGRESS** |
| V3-M3 | Linux one-public-owner server/manager/release composition | **IMPLEMENTED; V3 RELEASE WORKFLOW GREEN ON PRIOR QUALIFIED HEAD** |
| V3-M4 | exact Windows Npcap WBD-flow demux and adapter-noise qualification | **CURRENT** |
| V3-M5 | V3 firewall/packaging contract cleanup; remove legacy dual-flow release gates | **CURRENT** |
| V3-M6 | full 20/100 ms weak-link and reconnect regression on frozen single-flow candidate | **IN QUALIFICATION** |
| V3-M7 | physical Windows 11/Npcap/NIC/NAT/ISP ↔ Linux ARM64 final platform run | **FINAL PLATFORM GATE** |
| V3-M8 | promote clean semantic commits to formal branch and issue official bundles | **AFTER M7** |

## Frozen transport foundation retained from V2

The transport work below remains valid and should not be rewritten merely because the connection-establishment architecture changed:

- WBD-owned TCP-shaped raw packets rather than an ordinary kernel TCP payload stream;
- one raw lane as the release baseline;
- latency-first legacy shadow recovery as product default;
- SACK/RACK retained as explicit experimental/research mode unless a loaded candidate beats legacy;
- systematic FEC sends available source shards immediately and repair later;
- release FEC modes `off` and fixed systematic `20:20`;
- immutable LINK_INIT/LINK_ACCEPT per association;
- wolfSSL DTLS 1.3 security lock at `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- per-session/ticket/LiveID isolation and independent FEC state;
- 40 Mbit/s release operating point before interpreting 60/80 Mbit/s headroom.

Historical transport benchmark documents remain evidence for those decisions even when their setup diagrams predate V3.

## V3 Reality-like fidelity work

The first seconds should look as close as practical to a normal browser/REALITY-like TLS connection without violating the one-flow/no-HOL laws.

Current work includes:

- browser-like TCP SYN option signature on FakeTCP handshake;
- real TLS 1.3 record and handshake grammar;
- browser/uTLS ClientHello profiles, currently centered on the qualified Firefox-like path;
- valid SNI and certificate handshake;
- encrypted username/password admission;
- encrypted mode-switch control rather than public WBD magic;
- bounded TLS record/handshake sizing and fragmentation tests.

Fingerprint fidelity is allowed to improve incrementally. It is never a reason to open a second public connection or keep an ordered stream after the switch.

## Windows V3 path

Windows must execute:

```text
Npcap underlay discovery
  -> one raw FakeTCP association
  -> in-flow Reality-like TLS/auth/switch ready
  -> DTLS ready
  -> LINK ready
  -> TUN ready
  -> IPv6 fail-closed + routes
  -> connected
```

The raw backend must filter adapter noise before the strict FakeTCP parser. ARP, IPv6, UDP, wrong 4-tuples, malformed frames and self-captured outbound frames are not handshake failures and must be discarded. Child exit must fail readiness immediately; cleanup is idempotent.

`connect_pass` is valid only after the actual LINK/TUN data path is ready.

## Linux V3 path

Official server composition is:

```text
public WBD_PORT
   ↓ sole WBD owner
wbd-faketcp-mux
   ├─ FakeTCP association table
   ├─ bounded Reality-like TLS/auth/switch phase
   └─ DTLS worker per admitted session
        ↓
127.0.0.1 LINK mux
        ↓
127.0.0.1 platform proxy
```

The V3 product bundle must not launch `wbd-reality-front` as a competing kernel TCP listener. Legacy standalone Reality front/mirror code is historical/diagnostic only.

Firewall qualification must model one public WBD port and WBD-owned raw/RST-suppression state. Cleanup must remove only WBD-owned rules.

## Automated V3 qualification

A candidate is not deliverable until the same substantive HEAD has green relevant gates proving:

1. Windows native tests/build for FakeTCP, Reality-like setup, runtime/readiness and diagnostics;
2. Linux host tests/build for FakeTCP mux, single-flow bootstrap and DTLS worker;
3. captured Linux/NAT single-flow E2E with exactly one SYN/4-tuple;
4. real TLS 1.3 before DTLS on that association;
5. encrypted switch and absence of second SYN/FIN/RST/close_notify at the boundary;
6. later-datagram bypass of deliberate earlier post-switch loss;
7. dirty reconnect stress;
8. legacy FakeTCP first-arrival/native/pcap loss regressions;
9. V3 one-public-owner firewall/manager composition;
10. Windows portable and Linux amd64/arm64 bundle qualification.

Physical Windows 11 + Npcap + real NIC/NAT/ISP ↔ Linux ARM64 remains mandatory final platform evidence. Hosted CI is not a substitute for that physical run.

## Development order from current state

1. keep the frozen tcp-like steady-state path unchanged;
2. close Windows Npcap startup/noise gaps with Windows-native tests;
3. remove stale dual-flow workflow/document contracts from V3 qualification;
4. re-run cross-platform + strong single-flow E2E + reconnect + weak-network gates on one substantive HEAD;
5. build V3 Windows and Linux ARM64 artifacts from that exact HEAD;
6. run physical Windows ↔ Linux ARM64 self-test/data probes;
7. fix only the first deterministic physical failure if one remains;
8. once physical qualification passes, promote clean semantic commits to `dev/wbd-raw-fec-v2` and cut official release artifacts.

## Historical V2 decisions superseded for V3

The following are retained only as history and must not be used as current implementation instructions:

- a separate ordinary TCP/TLS Persona or `wbd-reality-front` connection followed by a new FakeTCP SYN;
- ticket correlation as justification for two public flows;
- a product composition with a kernel Reality listener and raw FakeTCP listener competing on the same WBD port;
- final release sequence `Reality front -> close -> FakeTCP -> DTLS`.

ADR-0011 is authoritative when an older ADR/benchmark/roadmap passage conflicts with the V3 one-public-flow law.

## Deferred work

- additional ClientHello/browser fingerprint profiles after the current path is stable;
- extra fixed FEC presets or adaptive FEC;
- multiple raw lanes;
- >100 Mbit/s optimization;
- Android/unprivileged endpoints.

None of these may weaken one-public-flow or no-HOL behavior.
