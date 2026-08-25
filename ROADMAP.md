# Roadmap

> **Status: V2.2 ACTIVE.** One-lane WBD-owned FEC + DTLS 1.3 + native TCP-shaped FakeTCP has focused first-arrival and 20%-loss pcap evidence. Current work is fixed-FEC qualification, immutable one-time setup, and a deliberately narrow **periodic fixed-profile refresh** based on low-load FakeTCP loss samples. A bounded Reality-style fixed-target mirror is now available as an isolated network-treatment diagnostic. Continuously learning Auto FEC remains deferred advanced research.

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
| V2-M6C | Linux/OpenWrt capture policy: global / only-cn / only-non-cn | **PLANNED AFTER LINK/PERSONA FOUNDATION** |
| V2-M7A | Windows Wintun L3 client | **PLANNED** |
| V2-M7B | Windows global/split capture with underlay escape and minimal persistent rules | **PLANNED** |
| V2-M8A | optional TLS Persona bootstrap + network-treatment diagnostic | **REALITY-STYLE TARGET-MIRROR DIAGNOSTIC IMPLEMENTED; PERSONA STILL PLANNED** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | fixed FEC presets + immutable setup + periodic low-load refresh | **CURRENT** |
| V2-M8C | account + per-device credential + concurrent multi-session server state | **PLANNED** |
| V2-M9 | optional two-lane striped/hedged/survival research | only if one-lane measured cliff justifies it |
| V2-M10 | release qualification | final Linux/OpenWrt/Windows + security + transport regression |
| V2-X1 | advanced continuously learning Auto FEC / automatic capacity inference | **FUTURE RESEARCH; NOT REQUIRED** |

## V2-M8B-T1 evidence retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

## V2-M8B-T2 current gate — fixed FEC + immutable setup + periodic refresh

The iid mathematical reference for an ideal systematic `(K+R,K)` MDS block is:

```text
P_fail = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l)
```

For `K=20`, a representative strong iid block-failure target requires approximately `R=4/8/12/16/20` around `p=1/5/10/15/20%`. `20:20` is therefore a strong-loss reference rather than a universal default.

The offline fixed-scheduler comparison remains available, but the near-term live target is simpler: preserve immediate systematic source transmission and implement a qualified fixed preset family `off`, `20:4`, `20:8`, `20:12`, `20:16`, `20:20` before attempting more exotic scheduler changes.

`internal/fec/simulator.go` and `cmd/wbd-fec-sweep` remain deterministic qualification tools. Simulator evidence does not automatically enable an unimplemented live profile.

### Immutable setup rule

One association is established as:

```text
FakeTCP -> DTLS 1.3 -> LINK_INIT -> LINK_ACCEPT -> optional AUTH -> Established
```

After Established, link-defining parameters never change in place. A different FEC profile means a fresh association, preferably make-before-break.

The current implementation still carries one symmetric LinkConfig. The next directional setup change will separate client-TX and server-TX FEC while keeping shared MTU/lane/protocol parameters explicit. The client chooses its transmit profile; the server chooses its transmit profile from its own local low-load measurement and returns that immutable choice in LINK_ACCEPT.

### Periodic fixed-profile refresh

This is intentionally **not** the old high-complexity Auto plan.

Default design from ADR-0007:

- refresh interval: configurable `30m` or `60m`;
- measurement window: about `20s`;
- only sample when sender original traffic is below a low-load gate, initially 5% of configured physical capacity;
- estimate one-way sender loss as `delta(unique first-loss marks) / delta(original segments)`;
- use a Wilson 95% upper confidence bound for the fixed-preset lookup;
- organic traffic supplies samples at zero additional wire cost;
- optional idle probe only fills a sample deficit, then stops;
- changed FEC applies only on a new association;
- advanced automatic capacity inference and continuous online optimization remain out of scope.

The FakeTCP sender now exposes unique first-loss and original-byte counters. `internal/linkadapt` provides counter-window math, Wilson bounds, low-load gating, coarse fixed-preset recommendation, probe-deficit accounting, and a deterministic inner-rate budget.

### Inner-rate capacity guard

For configured path capacity `C`, target utilization `u`, FEC factor `F`, packet/header expansion and shadow retransmission factor `A`, the client limits inner offered payload approximately as:

```text
B_inner_max = C * u * (1-ack_reserve) / (F * packet_expansion * A)
```

`A` is conservatively at least `1/(1-p_hi)` and may be raised to the actual measured retransmission-byte factor. This is not TCP congestion control; it prevents known FEC + shadow retransmission expansion from persistently filling the physical queue.

Example ignoring headers/ACK reserve: 200 Mbit/s physical capacity, 20% loss upper bound, 20:20 FEC and 80% target utilization gives a 64 Mbit/s inner payload ceiling.

### T2 exit gate

- fixed-scheduler simulator and existing first-arrival tests remain green;
- immutable LINK_INIT/LINK_ACCEPT startup and reliable retry tests remain green;
- unique first-loss counters are verified not to double-count repeated retries;
- `internal/linkadapt` fixed selector/rate-budget tests pass;
- live systematic `20:4/8/12/16/20` presets are implemented and pcap/first-arrival qualified;
- establishment supports immutable directional client-TX/server-TX FEC;
- a 30/60-minute scheduler can wait for low load, produce a 20-second sample, and rotate associations without interrupting inner traffic;
- loaded-vs-low-load tests show the selector does not treat self-induced saturation as baseline path loss.

Advanced continuously learning Auto FEC is not part of this exit gate.

## V2-M8A TLS Persona and network-treatment diagnostics

Persona remains a real standard TLS 1.3 preflight, separate from the DTLS/FEC data lane.

- client selects a pinned browser-like profile from the server-supported set;
- server/operator owns endpoint hostname(s), certificate/private key and policy;
- client performs normal trust-chain + hostname validation;
- public services such as speed-test sites may be used as **genuine control connections** for network-treatment measurements;
- diagnostics may record the public site's real leaf-certificate/SPKI fingerprint, handshake timing, download timing and path statistics;
- WBD does not claim that third-party identity, copy its private key, or disable verification.

A certificate fingerprint alone cannot make a WBD endpoint authenticate as that site because TLS CertificateVerify requires the matching private key. Browser-profile implementations are pinned and pcap-qualified rather than trusting a moving `Auto` alias.

The first REALITY-inspired diagnostic is now implemented as `cmd/wbd-reality-mirror` plus `internal/realitymirror` and `scripts/bench_reality_mirror.py`. It mirrors one fixed genuine TLS target: the client's exact ClientHello is sent to that target and the target's real TLS records return to the client. SNI must exactly match the configured target identity before the server dials upstream. The listener defaults to loopback and has session, byte and concurrency bounds so it is not an open fallback proxy. The paired benchmark alternates direct and mirror samples and verifies the same real target certificate/SPKI is observed.

This does **not** implement authenticated REALITY and does not carry sustained WBD payload inside the mirrored TCP/TLS stream. ADR-0008 records the boundary. Further REALITY-like work is admitted only if real-network paired evidence shows a repeatable material advantage and any follow-up preserves the unordered WBD data-plane invariant.

## V2-M8C account / concurrent sessions

Extend DTLS-protected bearer authorization into a minimal account/device model:

- one username/account may own several simultaneous device sessions;
- live state is keyed by account + unique session ID, never username alone;
- prefer a distinct high-entropy device token/key per client installation;
- support independent device revocation and optional server-side concurrent-session caps;
- link/FEC/Persona choices are session-local and fixed during each association.

## V2-M6C / M7 capture and split-routing policy

Common client modes: `off`, `global`, `only-cn`, `only-non-cn`.

All modes first establish explicit escape routes/policy for actual WBD underlay server and Persona/bootstrap endpoints via the original physical gateway. Tunnel recursion is a test failure.

Linux/OpenWrt uses TUN + policy routing and compact kernel prefix/interval sets. Windows uses Wintun-class L3 I/O. Full-tunnel Windows prefers broad routes plus explicit endpoint escape routes. Split mode must avoid thousands of persistent Windows Firewall rules and use compact routing/WFP/equivalent longest-prefix classification.

## V2-M5 / M9 dual-lane admission rule

Do not build `two lanes x full duplicate x 20:20` as a normal 4x mode. First measure cross-lane loss/latency correlation.

Preferred later experiments are `striped`, `hedged`, and explicit emergency `survival`. Two lanes may share one coding family but should use distinct lane IDs, sequence spaces, coding equations/seeds/window phases and schedules.

## V2-X1 advanced Auto FEC — deliberately deferred

The deferred item is a continuously learning controller or automatic capacity estimator: joint loss/recovery/capacity inference, queue-pressure detection, continuous hysteresis, scheduler learning and high-frequency transitions. The simple periodic fixed-table refresh in T2 does not depend on those features.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- runtime FEC config epochs / mid-session link parameter switching;
- continuously learning/high-frequency Auto FEC on the current critical path;
- VLESS/Xray routing/Vision stream semantics as the product data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication.
