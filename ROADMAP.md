# Roadmap

> **Status: V2.2 ACTIVE.** One-lane WBD-owned FEC + DTLS 1.3 + native TCP-shaped FakeTCP now has focused first-arrival and 20%-loss pcap evidence. The next work moves from a frozen 20:20 experiment to an optional/adaptive FEC control surface, then Persona/accounts/platform routing.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; external baseline retained |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; pinned wolfSSL DTLS 1.3 + X.509/hostname validation qualified |
| V2-M3A-E | minimal native session/control + bearer auth + fixed config | **DONE AS FOUNDATION**; will be extended rather than replaced |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED**; historical evidence only |
| V2-M5 | optional two raw lanes | **DEFERRED / POST-ADAPTIVE-FEC EXPERIMENT** |
| V2-M6A | Linux/OpenWrt packet-preserving L3/TUN core | **IMPLEMENTED** |
| V2-M6B | privileged real-TUN integration | harness implemented; external real-device qualification still required |
| V2-M6C | Linux/OpenWrt capture policy: global / only-cn / only-non-cn | **PLANNED AFTER CONTROL/FEC EPOCHS** |
| V2-M7A | Windows Wintun L3 client | **PLANNED** |
| V2-M7B | Windows global/split capture with underlay escape and minimal persistent rules | **PLANNED** |
| V2-M8A | optional TLS Persona bootstrap | **ADMITTED; IMPLEMENT NEXT AFTER FEC CONTROL BOUNDARY** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | adaptive FEC math, scheduler comparison, off/fixed runtime config epochs | **CURRENT** |
| V2-M8B-T3 | client-side Auto FEC estimator/controller | planned after T2; must pass stability/hysteresis/resource tests |
| V2-M8C | account + per-device credential + concurrent multi-session server state | **PLANNED** |
| V2-M9 | optional two-lane striped/hedged research | only if one-lane measured cliff justifies it |
| V2-M10 | release qualification | final Linux/OpenWrt/Windows + security + transport regression |

## V2-M8B-T1 evidence now retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path now streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

The p50 values are near the one-way propagation floor and materially improve over the previous block-batched systematic schedule. This validates the invariant that an available systematic source should not wait for the FEC block.

## V2-M8B-T2 current gate — minimum-delay optional FEC

The mathematical reference for iid random loss is an ideal systematic `(K+R,K)` MDS block. Its block-recovery failure probability is:

```text
P_fail = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l)
```

For `K=20`, a representative `P_fail <= 1e-5` target requires approximately `R=4/8/12/16/20` at `p=1/5/10/15/20%`, demonstrating that 20:20 is a strong-loss reference rather than a good universal default.

The current implementation can stream systematic sources but full-block RS repairs are still causal only after enough source state exists. T2 therefore builds a deterministic simulator/sweeper and compares:

1. current `K`-block RS + tail repairs;
2. smaller/micro-block systematic RS;
3. causal/sliding-window systematic linear repair with repairs spread as early and evenly as possible.

The simulator must include offered payload rate and link capacity. A FEC scheme that reduces erasure probability but pushes `payload_rate * (1+repair_ratio)` near path capacity is not a latency win because queue delay then harms every packet.

T2 exit gate:

- runtime `fec.mode=off|fixed` works without reconnect;
- directional configuration epochs switch only at coding-window boundaries;
- server advertises capability/resource ceilings and can accept/clamp/reject client proposals;
- deterministic iid + burst-loss tests compare first-complete p50/p95/p99, delivery, CPU/RSS and total wire cost;
- one candidate repair scheduler wins enough evidence to become the Auto-controller substrate.

## V2-M8B-T3 Auto FEC

Auto is a **client-side controller**. It observes directional loss/recovery, RTT, delivered goodput and queue pressure, then proposes a bounded profile. The server is adaptive but retains hard limits.

Auto must have hysteresis and minimum dwell time so normal random variation does not oscillate FEC modes. It must fail safely to the last admitted fixed/off profile when estimates are stale. Uplink and downlink may choose different protection.

Auto is not qualified by a single loss percentage. It must be evaluated against step changes, burst loss, capacity changes, idle-to-bulk transitions and estimator error.

## V2-M8A TLS Persona

Persona remains a real standard TLS 1.3 preflight to an operator-controlled endpoint, separate from the DTLS/FEC data lane.

Configuration ownership:

- client selects `off/native/chrome/firefox/safari/edge` from server-supported profiles;
- server/operator owns allowed hostname(s), certificate/private key, protocol/profile support and policy caps;
- client performs normal trust-chain + hostname validation;
- no unrelated website certificate/private key is borrowed.

Qualification records ClientHello bytes/segmentation, TLS version/cipher/ALPN, certificate validation, p50/p95/p99, failure rate and MTU/fragmentation behavior. Browser profile implementations are pinned and pcap-qualified rather than assumed current because profile libraries evolve independently of browser releases.

## V2-M8C account / concurrent sessions

Extend the existing DTLS-protected bearer authorization into a minimal account/device model:

- username/account identity may own several simultaneous device sessions;
- live transport state is keyed by account + unique session ID, never username alone;
- prefer a distinct high-entropy device token/key per client installation;
- support independent device revocation and optional server-side concurrent-session caps;
- FEC, routing, Persona and lane settings remain client-session choices negotiated after auth.

This is deliberately smaller than a general multi-tenant account/control platform.

## V2-M6C / M7 capture and split-routing policy

Common client modes: `off`, `global`, `only-cn`, `only-non-cn`.

All modes must first establish explicit escape routes/policy for the actual WBD underlay server and Persona/bootstrap endpoints via the original physical gateway. Tunnel recursion is a test failure.

Linux/OpenWrt uses TUN + policy routing and compact kernel prefix/interval sets. Windows uses Wintun-class L3 I/O. Full-tunnel Windows should prefer broad routes plus explicit endpoint escape routes instead of rewriting thousands of firewall rules. For Windows split mode, compare compact aggregated routes with WFP/equivalent interception plus a user-space longest-prefix classifier; choose the design that causes the least persistent system mutation while preserving correct bypass semantics.

China/non-China classification is CIDR longest-prefix membership. Do not expand prefixes into an enormous exact-IP hash or rule set. Use a radix/Patricia-style portable classifier or superior platform-native interval/prefix set. IPv4 and IPv6 databases are versioned and atomically swapped.

## V2-M5 / M9 dual-lane admission rule

Do not build `two lanes x full duplicate x 20:20` as a normal 4x mode. First measure whether lane loss/latency is sufficiently de-correlated to justify the second lane.

Preferred experiments are:

- `striped`: sources and independent repairs spread across lanes, near the same `1+alpha` intentional overhead;
- `hedged`: selective second-lane duplicate/repair only when predicted value is high;
- `survival`: explicit emergency source duplication + independent repair, potentially >2x and therefore requiring a separate admission/constitution update.

The two lanes can share one coding family but must use distinct lane IDs, sequence spaces, coding seeds/equations/window phases and schedules so repair traffic is complementary rather than byte-identical duplication.

## Platform state retained

M6A packet-preserving WBDP/TUN code and the M6B privileged real-TUN harness remain in-tree. Actual OpenWrt/Linux and Windows qualification requires real hosts/drivers/raw privileges and repository-backed receipts.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- VLESS/Xray routing/Vision stream semantics as the product data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication.
