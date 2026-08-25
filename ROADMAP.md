# Roadmap

> **Status: V2.2 ACTIVE.** One-lane WBD-owned FEC + DTLS 1.3 + native TCP-shaped FakeTCP has focused first-arrival and 20%-loss pcap evidence. Current work is **fixed FEC scheduler qualification and runtime `off|fixed` control**, followed by Persona/accounts/platform routing. Auto FEC is deferred to later advanced research.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; external baseline retained |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; pinned wolfSSL DTLS 1.3 + X.509/hostname validation qualified |
| V2-M3A-E | minimal native session/control + bearer auth + fixed config foundation | **DONE AS FOUNDATION**; extend rather than replace |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED**; historical evidence only |
| V2-M5 | optional two raw lanes | **DEFERRED / POST-FIXED-FEC EXPERIMENT** |
| V2-M6A | Linux/OpenWrt packet-preserving L3/TUN core | **IMPLEMENTED** |
| V2-M6B | privileged real-TUN integration | harness implemented; external real-device qualification still required |
| V2-M6C | Linux/OpenWrt capture policy: global / only-cn / only-non-cn | **PLANNED AFTER FIXED FEC CONTROL** |
| V2-M7A | Windows Wintun L3 client | **PLANNED** |
| V2-M7B | Windows global/split capture with underlay escape and minimal persistent rules | **PLANNED** |
| V2-M8A | optional TLS Persona bootstrap | **PLANNED AFTER FIXED FEC CONTROL** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | fixed FEC math/scheduler comparison + `off|fixed` runtime config epochs | **CURRENT** |
| V2-M8C | account + per-device credential + concurrent multi-session server state | **PLANNED** |
| V2-M9 | optional two-lane striped/hedged/survival research | only if one-lane measured cliff justifies it |
| V2-M10 | release qualification | final Linux/OpenWrt/Windows + security + transport regression |
| V2-X1 | Auto FEC estimator/controller | **FUTURE ADVANCED RESEARCH; NOT ON V2.2 CRITICAL PATH** |

## V2-M8B-T1 evidence retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

This validates the invariant that an available systematic source should not wait for the FEC block.

## V2-M8B-T2 current gate — fixed minimum-delay optional FEC

The iid mathematical reference for an ideal systematic `(K+R,K)` MDS block is:

```text
P_fail = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l)
```

For `K=20`, a representative `P_fail <= 1e-5` target requires approximately `R=4/8/12/16/20` at `p=1/5/10/15/20%`. This is planning evidence that `20:20` is a strong-loss reference, not a universal default.

The current fixed-scheduler comparison is:

1. current `K`-block systematic RS + tail repairs;
2. smaller/micro-block systematic RS;
3. causal/sliding-window systematic linear repair with repairs spread earlier.

`internal/fec/simulator.go` and `cmd/wbd-fec-sweep` now provide the deterministic offline timing/capacity model. The simulator keeps source opportunities immediate, gives ready sources priority over repairs, applies iid or burst loss with fixed seeds, and records direct/repaired delivery, p50/p95/p99, wire ratio, repair backlog and finite-run drain.

T2 exit gate:

- fixed-scheduler simulator tests are green in repository CI;
- representative iid and burst sweeps identify where tail/micro/causal differ and where capacity pressure causes repair backlog;
- runtime `fec.mode=off|fixed` works without reconnect;
- directional configuration epochs switch only at coding-window boundaries;
- server advertises capability/resource ceilings and can accept/clamp/reject fixed client proposals;
- live transport regression confirms no systematic-source batching returns.

No Auto controller is part of this exit gate.

## V2-M8A TLS Persona

Persona remains a real standard TLS 1.3 preflight, separate from the DTLS/FEC data lane.

Configuration ownership:

- client selects `off/native/chrome/firefox/safari/edge` from server-supported profiles;
- server/operator owns allowed hostname(s), certificate/private key, protocol/profile support and policy caps;
- client performs normal trust-chain + hostname validation;
- public sites such as speed-test services may be used as external measurement baselines, but WBD does not borrow a third-party certificate/private key or present an unrelated third-party identity as its own endpoint.

Qualification records ClientHello bytes/segmentation, TLS version/cipher/ALPN, certificate validation, p50/p95/p99, failure rate and MTU/fragmentation behavior. Browser profile implementations are pinned and pcap-qualified rather than assumed current.

## V2-M8C account / concurrent sessions

Extend the existing DTLS-protected bearer authorization into a minimal account/device model:

- username/account identity may own several simultaneous device sessions;
- live transport state is keyed by account + unique session ID, never username alone;
- prefer a distinct high-entropy device token/key per client installation;
- support independent device revocation and optional server-side concurrent-session caps;
- FEC, routing, Persona and lane settings remain client-session choices negotiated after auth.

## V2-M6C / M7 capture and split-routing policy

Common client modes: `off`, `global`, `only-cn`, `only-non-cn`.

All modes first establish explicit escape routes/policy for actual WBD underlay server and Persona/bootstrap endpoints via the original physical gateway. Tunnel recursion is a test failure.

Linux/OpenWrt uses TUN + policy routing and compact kernel prefix/interval sets. Windows uses Wintun-class L3 I/O. Full-tunnel Windows prefers broad routes plus explicit endpoint escape routes instead of rewriting thousands of firewall rules. For Windows split mode, compare compact aggregated routes with WFP/equivalent interception plus a user-space longest-prefix classifier and choose the design with the least persistent system mutation.

China/non-China classification is CIDR longest-prefix membership. Do not expand prefixes into an enormous exact-IP hash or rule set. Use a radix/Patricia-style portable classifier or superior platform-native interval/prefix set. IPv4 and IPv6 databases are versioned and atomically swapped.

## V2-M5 / M9 dual-lane admission rule

Do not build `two lanes x full duplicate x 20:20` as a normal 4x mode. First measure whether lane loss/latency is sufficiently de-correlated to justify the second lane.

Preferred fixed experiments are:

- `striped`: sources and independent repairs spread across lanes, near the same `1+alpha` intentional overhead;
- `hedged`: selected second-lane duplicate/repair;
- `survival`: explicit emergency source duplication + independent repair, potentially >2x and therefore requiring a separate admission/constitution update.

The two lanes can share one coding family but must use distinct lane IDs, sequence spaces, coding seeds/equations/window phases and schedules so repair traffic is complementary rather than byte-identical duplication.

## V2-X1 Auto FEC — deliberately deferred

Auto FEC is not being developed during the current V2.2 path. It depends on loss/recovery estimation, capacity inference, queue-pressure detection, hysteresis, minimum dwell time, directional asymmetry and extensive transition testing. Those concerns are intentionally separated from the fixed FEC product so they cannot destabilize current transport/platform work.

If reopened later, Auto must be a new explicit milestone with its own evidence and must fail safely to an admitted `off` or `fixed` profile.

## Platform state retained

M6A packet-preserving WBDP/TUN code and the M6B privileged real-TUN harness remain in-tree. Actual OpenWrt/Linux and Windows qualification requires real hosts/drivers/raw privileges and repository-backed receipts.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- VLESS/Xray routing/Vision stream semantics as the product data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication;
- Auto FEC on the current V2.2 critical path.
