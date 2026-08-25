# Roadmap

> **Status: V2.2 ACTIVE.** One-lane WBD-owned FEC + DTLS 1.3 + native TCP-shaped FakeTCP has focused first-arrival and 20%-loss pcap evidence. Current work is fixed-FEC scheduler qualification plus **immutable one-time link setup**, followed by Persona/accounts/platform routing. Auto FEC is deferred advanced research.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; external baseline retained |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; pinned wolfSSL DTLS 1.3 + X.509/hostname validation qualified |
| V2-M3A-E | minimal native session/control + bearer auth + legacy fixed config foundation | **DONE AS FOUNDATION**; legacy CONFIG retained only for compatibility tests |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** |
| V2-M5 | optional two raw lanes | **DEFERRED / POST-FIXED-FEC EXPERIMENT** |
| V2-M6A | Linux/OpenWrt packet-preserving L3/TUN core | **IMPLEMENTED** |
| V2-M6B | privileged real-TUN integration | harness implemented; external real-device qualification still required |
| V2-M6C | Linux/OpenWrt capture policy: global / only-cn / only-non-cn | **PLANNED AFTER IMMUTABLE LINK SETUP** |
| V2-M7A | Windows Wintun L3 client | **PLANNED** |
| V2-M7B | Windows global/split capture with underlay escape and minimal persistent rules | **PLANNED** |
| V2-M8A | optional TLS Persona bootstrap | **PLANNED AFTER IMMUTABLE LINK SETUP** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | fixed FEC scheduler comparison + immutable `LINK_INIT/LINK_ACCEPT` | **CURRENT** |
| V2-M8C | account + per-device credential + concurrent multi-session server state | **PLANNED** |
| V2-M9 | optional two-lane striped/hedged/survival research | only if one-lane measured cliff justifies it |
| V2-M10 | release qualification | final Linux/OpenWrt/Windows + security + transport regression |
| V2-X1 | Auto FEC estimator/controller | **FUTURE ADVANCED RESEARCH; NOT ON V2.2 CRITICAL PATH** |

## V2-M8B-T1 evidence retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

## V2-M8B-T2 current gate — fixed FEC + immutable setup

The iid mathematical reference for an ideal systematic `(K+R,K)` MDS block is:

```text
P_fail = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l)
```

For `K=20`, a representative `P_fail <= 1e-5` target requires approximately `R=4/8/12/16/20` at `p=1/5/10/15/20%`. `20:20` is therefore a strong-loss reference rather than a universal default.

The offline fixed-scheduler comparison remains:

1. current `K`-block systematic RS + tail repairs;
2. smaller/micro-block systematic RS;
3. causal/sliding-window systematic linear repair with earlier repairs.

`internal/fec/simulator.go` and `cmd/wbd-fec-sweep` model deterministic iid/burst loss, source-priority serialization, first-complete latency, delivery, wire ratio, repair backlog and finite-run drain. Simulator evidence does not automatically enable an unimplemented live profile.

### Immutable setup rule

The current product does **not** support runtime FEC switching or configuration epochs.

One association is established as:

```text
FakeTCP -> DTLS 1.3 -> LINK_INIT -> LINK_ACCEPT -> AUTH -> Established
```

`LINK_INIT` proposes this association's FEC off/fixed profile, coding geometry/scheduler, flush timing, MTU and lane count. The server validates its capability/resource policy and either accepts the exact proposal or rejects it. It does not silently clamp/rewrite link parameters.

After Established, these parameters never change. A client that wants another FEC profile, MTU, scheduler or lane count closes and establishes a new association.

The current live WBD implementation admits:

- FEC off;
- fixed systematic `20:20` tail-RS;
- one raw lane;
- validated MTU/flush bounds.

`20:10`, micro and causal schedules remain reference/research values until their live codec paths are implemented and qualified.

T2 exit gate:

- fixed-scheduler simulator tests remain green;
- `LINK_INIT/LINK_ACCEPT` wire codec and server state machine are unit-tested;
- unsupported profiles fail establishment explicitly;
- exact accepted config is immutable through AUTH/Established;
- post-establishment LINK_INIT or legacy CONFIG is rejected with reconnect-required semantics;
- live transport integrates the accepted immutable config without reintroducing systematic-source batching.

No Auto controller and no runtime config epoch are part of this gate.

## V2-M8A TLS Persona

Persona remains a real standard TLS 1.3 preflight, separate from the DTLS/FEC data lane.

- client selects `off/native/chrome/firefox/safari/edge` from server-supported profiles;
- server/operator owns endpoint hostname(s), certificate/private key and policy;
- client performs normal trust-chain + hostname validation;
- public sites such as speed-test services may be used as network-treatment baselines, but WBD does not borrow their certificate/private key or present their identity as its own endpoint.

Browser profile implementations are pinned and pcap-qualified rather than assumed current.

## V2-M8C account / concurrent sessions

Extend DTLS-protected bearer authorization into a minimal account/device model:

- one username/account may own several simultaneous device sessions;
- live state is keyed by account + unique session ID, never username alone;
- prefer a distinct high-entropy device token/key per client installation;
- support independent device revocation and optional server-side concurrent-session caps;
- link/FEC/Persona choices are session-local and fixed during establishment.

## V2-M6C / M7 capture and split-routing policy

Common client modes: `off`, `global`, `only-cn`, `only-non-cn`.

All modes first establish explicit escape routes/policy for actual WBD underlay server and Persona/bootstrap endpoints via the original physical gateway. Tunnel recursion is a test failure.

Linux/OpenWrt uses TUN + policy routing and compact kernel prefix/interval sets. Windows uses Wintun-class L3 I/O. Full-tunnel Windows prefers broad routes plus explicit endpoint escape routes. Split mode must avoid thousands of persistent Windows Firewall rules and use compact routing/WFP/equivalent longest-prefix classification.

## V2-M5 / M9 dual-lane admission rule

Do not build `two lanes x full duplicate x 20:20` as a normal 4x mode. First measure cross-lane loss/latency correlation.

Preferred later experiments are `striped`, `hedged`, and explicit emergency `survival`. Two lanes may share one coding family but should use distinct lane IDs, sequence spaces, coding equations/seeds/window phases and schedules.

## V2-X1 Auto FEC — deliberately deferred

Auto FEC is not being developed during V2.2. It depends on loss/recovery estimation, capacity inference, queue-pressure detection, hysteresis, minimum dwell time, directional asymmetry and extensive transition testing. If reopened later, it gets its own milestone and evidence.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- runtime FEC config epochs / mid-session link parameter switching;
- Auto FEC on the current V2.2 critical path;
- VLESS/Xray routing/Vision stream semantics as the product data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication.
