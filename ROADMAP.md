# Roadmap

> **Status: V2.3 SINGLE-FLOW CORRECTION ACTIVE.** The steady-state transport remains UDP/datagram-like over one WBD-owned TCP-shaped FakeTCP carrier, but ADR-0011 replaces the old two-public-connection Reality/FakeTCP setup with one continuous public flow.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE** |
| V2-M2 | native DTLS 1.3 security shim | **DONE** |
| V2-M3A-E | minimal session/control + immutable config foundation | **DONE AS FOUNDATION** |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** |
| V2-M5 | optional two raw lanes | **DEFERRED** |
| V2-M6A/B | packet-preserving Linux/TUN regression harness | **IMPLEMENTED** |
| V2-M6C | OpenWrt transparent capture | **FINAL SHAPE: TPROXY + POLICY ROUTING** |
| V2-M7A/B | Windows TUN + full/split routing | **IMPLEMENTED / PHYSICAL QUALIFICATION IN PROGRESS** |
| V2-M8A-old | separate ordinary-TCP Reality-like front | **SUPERSEDED BY ADR-0011** |
| V2-M8A-SF1 | temporary reliable FakeTCP bootstrap stream | **IMPLEMENTED + NO-HOL BOUNDARY UNIT-QUALIFIED** |
| V2-M8A-SF2 | real TLS 1.3 / Reality-like auth over same FakeTCP association | **IMPLEMENTED + SINGLE-FLOW E2E QUALIFIED** |
| V2-M8A-SF3 | raw-listener fallback/decoy proxy + fingerprint qualification | **NEXT ARCHITECTURE/RESEMBLANCE GATE** |
| V2-M8B-T1 | native FakeTCP + FEC first-arrival / pcap qualification | **RETAINED / GREEN ON CURRENT SINGLE-FLOW HEAD** |
| V2-M8B-T2 | fixed FEC + immutable setup | **RETAINED** |
| V2-M8C | shared-account concurrent association/DTLS/LiveID fan-out | **RETAINED, MUST BE REQUALIFIED WITH SINGLE-FLOW BOOTSTRAP** |
| V2-M9 | optional two-lane research | **POST-RELEASE ONLY IF JUSTIFIED** |
| V2-M10 | release qualification | single-flow protocol gate -> OpenWrt one-shot -> Windows one-shot |

## Current single-flow qualification evidence

At PR #9 head `fd2fbc10e5ce8225d8cbf47b7e1bb3990095dbaf`, the dedicated `single-flow-e2e` gate proves the core public-flow invariant with packet capture and the pinned wolfSSL DTLS implementation:

- exactly one client SYN sequence lineage for `10.88.0.2:41001 -> 10.88.0.1:443`;
- no second client 4-tuple to public port 443;
- real TLS 1.3 / Reality-like bootstrap reports `same_flow=1` on client and server;
- the same FakeTCP processes remain alive through the mode switch;
- DTLS server reaches PEEK / PEER_SET / HRR / ACCEPT_PASS and DTLS 1.3 READY;
- DTLS client reaches CONNECT_PASS and DTLS 1.3 READY;
- 20/20 bidirectional echo probes succeed after the switch.

The separate `faketcp-pcap-20loss` gate is also green after fixing capture readiness; no FakeTCP recovery algorithm was changed to obtain that result.

This is a protocol/integration qualification checkpoint, not final physical Windows/OpenWrt release qualification. The project still must complete the resemblance/fallback gate, concurrent-session requalification, and clean platform one-shot gates.

## Product order of operations

Development now proceeds in this order:

1. keep the measured steady-state no-HOL/FEC/DTLS behavior unchanged;
2. make FakeTCP own the public 4-tuple from the first SYN;
3. add a bounded reliable ordered stream adapter only for TLS bootstrap;
4. run real TLS 1.3 + Reality-like marker + shared username/password admission on that adapter;
5. switch to DTLS/datagram mode on the same 4-tuple/sequence space with no FIN/RST/new SYN;
6. prove a later post-switch datagram bypasses an earlier sequence hole;
7. add/qualify unrecognized ClientHello fallback and the selected TCP/TLS fingerprint;
8. re-run concurrent sessions, <=100 Mbit/s load and pcap gates;
9. re-run OpenWrt TPROXY and Windows TUN one-shot release qualification.

Steps 2-6 are now implemented; the dedicated single-flow E2E gate has qualified steps 2-5, and unit tests cover the post-bootstrap hole-bypass invariant in step 6. Platform convenience must not reintroduce a second public setup connection.

## V2-M8A-SF1 — bounded bootstrap stream

The bootstrap adapter exists because real TLS requires ordered reliable bytes. It is intentionally not a general-purpose steady-state stream.

Required properties:

- one existing FakeTCP association provides the sequence space;
- out-of-order bootstrap bytes are buffered only until contiguous;
- writes are split into bounded chunks and ACK-gated;
- setup has an absolute deadline and bounded memory;
- closing the adapter does not emit an outer FIN or create a new flow;
- the adapter is discarded after authentication/mode barrier.

Unit tests cover bootstrap stream behavior and the transition back to normal datagram semantics, including delivery of a later post-bootstrap datagram across an earlier missing FakeTCP sequence range.

## V2-M8A-SF2 — TLS/Reality-like admission on the same association

Qualified establishment shape:

```text
FakeTCP SYN / SYN-ACK / ACK
  -> TLS 1.3 ClientHello / ServerHello / Finished
  -> Reality-like marker recognition
  -> username/password inside TLS
  -> one-time ticket inside TLS
  -> ACK drain / mode barrier
  -> SAME FakeTCP flow
  -> DTLS 1.3
  -> ticket bind / LINK_INIT / LINK_ACCEPT
```

There is no separate `wbd-reality-front` public product connection before FakeTCP. The old front binary may remain only as a diagnostic/reference tool.

Windows product startup is:

```text
single-flow FakeTCP TLS/auth ready
  -> FakeTCP steady-mode READY
  -> DTLS READY
  -> LINK READY
  -> TUN READY
  -> IPv6 fail-closed
  -> routes
```

Linux product startup uses one raw public listener, not a kernel TCP front plus raw listener sharing the numeric port.

## V2-M8A-SF3 — probe/fingerprint resemblance

Single-flow is necessary but not sufficient for Reality-like behavior.

The release-facing resemblance gate must measure:

- SYN/SYN-ACK option/window/TTL profile against the selected realistic reference;
- real TLS 1.3 ClientHello with configured SNI;
- ClientHello/extension ordering against the selected target profile;
- no WBD-specific plaintext before TLS;
- active-probe behavior for an unrecognized ClientHello.

The target server behavior for an unrecognized ClientHello is a raw-stream proxy to the configured decoy target. This is allowed to use an ordinary outbound TCP connection because it is decoy traffic, not WBD VPN payload.

Until those pcap comparisons pass, use precise wording such as **real TLS single-flow bootstrap**; do not claim browser-perfect or "99% Reality" qualification.

## Retained no-HOL transport evidence

The native public carrier remains WBD-owned TCP-shaped raw packets. Existing focused gates demonstrate SYN/SYN-ACK/ACK, MSS/SACK/window scale, cumulative ACK/SACK, shadow retransmission and complete out-of-order inner datagram delivery.

ADR-0011 changes the setup boundary only. After the mode barrier, steady-state tests must continue to prove that later independent DTLS/FEC datagrams can complete before repair of an earlier FakeTCP sequence hole.

## 100 Mbit/s weak-link release decision

Current critical qualification remains <=100 Mbit/s. The release operating point stays 40 Mbit/s aggregate inner payload because higher loaded points consumed excessive latency margin.

Therefore:

- `legacy` FakeTCP shadow recovery remains the product default;
- `sack-rack` remains experimental;
- FEC wire remains `off` or fixed systematic `20:20` for the current release;
- one raw lane remains the release baseline.

The single-flow correction must not silently reopen these choices.

## Shared account / concurrent sessions

The server remains a personal shared-account service:

- one username/password may be used by several devices;
- each successful TLS bootstrap gets a distinct one-time ticket;
- live identity is ticket/LiveID, not username;
- each public raw 4-tuple owns independent bootstrap state, DTLS worker and immutable link/FEC state.

Updated fan-out target:

```text
one public raw listener
  -> flow A: TLS bootstrap -> DTLS worker A -> LiveID A
  -> flow B: TLS bootstrap -> DTLS worker B -> LiveID B
  -> probe C: unrecognized TLS -> fallback stream proxy
```

The two-client gate must be rerun after the single-flow integration; old two-connection admission evidence remains historical only.

## OpenWrt TPROXY release path

OpenWrt final mode still uses TPROXY + policy routing. It must exempt the single WBD public underlay flow before broad capture and restore all WBD-owned state on exit/failure.

Release proof must include DNS, TCP and UDP application traffic over the one public association.

## Windows TUN release path

Windows final mode remains TUN/Wintun-class L3 capture with Full/Foreign/China route policy, underlay escape, compact prefix classification and cleanup.

The readiness sequence must not call the connection successful before the single-flow TLS bootstrap, FakeTCP steady-mode readiness, DTLS, LINK and TUN are actually ready.

## V2-M10 final release gate

1. unit/protocol/pcap regressions green;
2. exactly one public WBD SYN lineage from setup through data mode;
3. real TLS bootstrap and authenticated admission on that flow;
4. post-switch no-HOL hole-bypass test green;
5. shared-account two-client single-flow fan-out green;
6. <=100 Mbit/s 40 Mbit release/load surface green;
7. clean OpenWrt TPROXY one-shot passes;
8. clean Windows TUN one-shot passes;
9. cleanup restores platform state.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- `Reality TCP -> close -> second FakeTCP connection`;
- parallel kernel Reality listener + raw FakeTCP listener as the intended product same-port design;
- kernel TCP state takeover as a release dependency;
- runtime FEC epochs;
- SACK/RACK as unconditional product default;
- high-frequency continuously learning Auto FEC on the current critical path;
- VLESS/Xray/Vision stream semantics as the data plane;
- WireGuard inner glue;
- Android/no-root;
- mandatory multi-lane duplication.
