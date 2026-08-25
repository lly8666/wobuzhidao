# Roadmap

> **Status: V2.2 ACTIVE.** The two product cores remain: inner UDP/datagram-like earliest-complete delivery and outer TCP-shaped FakeTCP behavior. Current weak-network qualification uses a **100 Mbit/s physical-link ceiling**. Reality-like setup, account admission and platform capture are deliberately kept outside the sustained data plane.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; external baseline retained |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; encryption/integrity qualified; personal client also supports explicit no-cert/no-hostname verification |
| V2-M3A-E | minimal native session/control + bearer auth + legacy fixed config foundation | **DONE AS FOUNDATION**; legacy AUTH retained only where needed for compatibility |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** |
| V2-M5 | optional two raw lanes | **DEFERRED / POST-100M ONE-LANE EXPERIMENT** |
| V2-M6A | Linux packet-preserving L3/TUN regression core | **IMPLEMENTED** |
| V2-M6B | privileged real-TUN integration harness | **IMPLEMENTED AS TEST HARNESS** |
| V2-M6C | OpenWrt transparent capture | **PLANNED FINAL SHAPE: TPROXY + POLICY ROUTING** |
| V2-M7A | Windows client capture | **PLANNED FINAL SHAPE: TUN/WINTUN-CLASS L3** |
| V2-M7B | Windows global/split capture + underlay escape | **PLANNED** |
| V2-M8A | Reality-like same-entry bootstrap | **IMPLEMENTED; SIMPLE SHARED USER/PASS PATH UNDER CI QUALIFICATION** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | fixed FEC presets + immutable setup + periodic low-load refresh | **CURRENT TRANSPORT WORK** |
| V2-M8C | shared-account concurrent session admission | **IN PROGRESS**; same username/password -> distinct one-time tickets, data-session demux still to finish |
| V2-M9 | optional two-lane striped/hedged/survival research | only if one-lane 100M cliff justifies it |
| V2-M10 | release qualification | protocol regression -> OpenWrt TPROXY one-shot VPN -> Windows TUN one-shot VPN |
| V2-X1 | advanced continuously learning Auto FEC / automatic capacity inference | **FUTURE RESEARCH; NOT REQUIRED** |

## Product order of operations

Development must finish in this order:

1. freeze and qualify UDP-like inner semantics and TCP-like FakeTCP outer semantics on a 100 Mbit/s weak link;
2. qualify fixed FEC, DTLS 1.3, immutable LINK_INIT/LINK_ACCEPT and recovery behavior;
3. qualify Reality-like same-entry recognition plus simple shared username/password -> one-time ticket admission;
4. run the full protocol regression matrix before platform integration;
5. integrate the frozen protocol into OpenWrt **TPROXY** and make one end-to-end VPN attempt succeed from clean state;
6. integrate the frozen protocol into Windows **TUN/Wintun-class** capture and make one end-to-end VPN attempt succeed from clean state.

Platform work must not be allowed to change the already-qualified transport semantics merely to make routing easier.

## V2-M8B-T1 evidence retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

A later same-binary `legacy` versus `sack-rack` recovery A/B showed that SACK/RACK did not materially tax body first-arrival latency at the tested load while recovering many packets that legacy never delivered inside the observation window. That recovery decision is not permanent until the loaded **100 Mbit/s** gate also passes.

## 100 Mbit/s weak-link rule

Current critical transport qualification must use `rate 100mbit` or lower. A 200 Mbit/s laboratory link may remain in historical benchmark documents, but it is not the current product assumption.

The loaded FakeTCP recovery gate offers `65 Mbit/s` inner payload on a `100 Mbit/s` public link at RTT `20/100 ms` and loss `10/20%`. This leaves only bounded room for headers, ACKs and retransmissions and therefore exposes whether advanced recovery steals queue/CPU time from new datagrams.

For configured path capacity `C <= 100 Mbit/s`, target utilization `u`, FEC factor `F`, packet/header expansion and shadow retransmission factor `A`, the client limits inner offered payload approximately as:

```text
B_inner_max = C * u * (1-ack_reserve) / (F * packet_expansion * A)
```

This is a physical-capacity guard, not TCP congestion control. It must never delay an available systematic source merely to fill a block.

## V2-M8B-T2 — fixed FEC + immutable setup

One data association is established as:

```text
FakeTCP -> DTLS 1.3 -> optional one-time ticket bind -> LINK_INIT -> LINK_ACCEPT -> Established
```

After Established, link-defining parameters never change in place. A different FEC profile means a fresh association, preferably make-before-break.

The current live admission remains FEC `off` or systematic `20:20` tail-RS. The intended fixed family is `off`, `20:4`, `20:8`, `20:12`, `20:16`, `20:20`; intermediate profiles must not be advertised until implemented and first-arrival qualified.

The narrow periodic refresh may sample sender first-loss counters during low load and choose another qualified fixed profile only for the next association. Advanced continuously learning Auto FEC remains future research.

### T2 exit gate

- fixed-scheduler simulator and first-arrival tests green;
- immutable LINK_INIT/LINK_ACCEPT tests green;
- FEC off and fixed path packet-preserving startup green;
- loaded 100 Mbit/s `legacy` versus `sack-rack` recovery gate shows advanced recovery does not materially delay new inner datagrams;
- live fixed preset family implemented only after each candidate is qualified;
- any changed profile is applied by association rotation, never in place.

## V2-M8A — Reality-like same-entry front

Preferred connection setup now uses one TCP listener:

```text
ClientHello
  -> recognized marker: same socket TLS 1.3 takeover
       -> one encrypted username/password request
       <- one-time ticket
  -> unrecognized marker: exact bytes continue to fixed fallback target
```

Sustained VPN payload never uses this TLS/TCP stream.

The personal client defaults to explicit no server-certificate/hostname verification. The front and DTLS may therefore accept an arbitrary self-signed certificate with a name unrelated to the configured SNI. This gives encryption without server certificate identity authentication and is an intentional personal-use tradeoff.

The old target-mirror/witness/DEMO_BIND path remains only as a diagnostic compatibility tool.

## V2-M8C — shared account / concurrent sessions

The server is intentionally not a multi-tenant account service.

- one configured username/password pair is the shared account credential;
- the same pair may be used by several devices simultaneously;
- recognized TLS sends username/password once inside TLS, without an extra nonce/HMAC challenge round trip;
- each successful login gets a fresh random one-time ticket;
- tickets carry the account label for logging but are independent session identities;
- live state must be keyed by session/ticket identity, not username;
- no per-device credential database, KDF, revocation table or single-login lock is required;
- simple process/resource limits such as `max-conns` are sufficient admission controls for the personal deployment.

The remaining M8C work is multi-session data-plane demultiplexing so several ticket-authenticated DTLS/WBD associations can remain active concurrently behind one server instance.

## OpenWrt TPROXY release path

OpenWrt final product mode uses TPROXY for selected TCP/UDP traffic. The integration layer must:

- install compact nftables/iptables TPROXY rules;
- use packet marks + policy routing to deliver selected traffic locally;
- exempt WBD front/FakeTCP underlay endpoints before broad capture is enabled;
- support `global`, `only-cn`, `only-non-cn` using compact sets rather than thousands of rules;
- restore all rules/routes/marks on exit or failed startup;
- prove ordinary DNS/TCP/UDP application traffic crosses the WBD association in one clean end-to-end run.

The existing Linux TUN bridge remains a regression harness and does not satisfy the OpenWrt release gate by itself.

## Windows TUN release path

Windows final product mode uses a TUN/Wintun-class L3 adapter. The integration layer must:

- create/open the adapter and configure addresses/MTU;
- install full/split routes with explicit underlay endpoint escape;
- avoid thousands of persistent Windows Firewall rules;
- pass packet-preserving IPv4/IPv6 traffic through the frozen WBD association;
- restore routes/adapter state on exit or failed startup;
- pass one clean end-to-end application traffic run after protocol qualification is already green.

## V2-M10 final release gate

The final test sequence is intentionally end-to-end rather than another synthetic transport-only benchmark:

1. protocol/unit/pcap/weak-link regressions all green;
2. start from clean server/client routing state;
3. install the platform VPN capture adapter;
4. establish Reality-like front -> ticket -> FakeTCP -> DTLS 1.3 -> LINK_INIT/LINK_ACCEPT once;
5. pass real DNS plus TCP/UDP application traffic;
6. verify underlay escape and no recursive capture;
7. stop the client and prove routing/firewall state is restored;
8. repeat once for OpenWrt TPROXY and once for Windows TUN.

The target is **one successful clean attempt per platform after the protocol is frozen**, then that sequence becomes the release regression.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- runtime FEC config epochs / mid-session link switching;
- continuously learning/high-frequency Auto FEC on the current critical path;
- mandatory per-device credential/revocation infrastructure;
- mandatory client certificate-chain/hostname verification in personal mode;
- VLESS/Xray routing/Vision stream semantics as the data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication.
