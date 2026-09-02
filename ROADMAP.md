# Roadmap

> **Status: V2.2 ACTIVE.** The two product cores remain: inner UDP/datagram-like earliest-complete delivery and outer TCP-shaped FakeTCP behavior. Current weak-network qualification uses a **100 Mbit/s physical-link ceiling**. Reality-like setup, account admission and platform capture stay outside the sustained data-plane critical path.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; external baseline retained |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; encryption/integrity qualified; personal client supports explicit no-cert/no-hostname verification |
| V2-M3A-E | minimal native session/control + bearer auth + legacy fixed config foundation | **DONE AS FOUNDATION**; legacy AUTH retained only where needed for compatibility |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** |
| V2-M5 | optional two raw lanes | **DEFERRED / POST-100M ONE-LANE EXPERIMENT** |
| V2-M6A | Linux packet-preserving L3/TUN regression core | **IMPLEMENTED** |
| V2-M6B | privileged real-TUN integration harness | **IMPLEMENTED AS TEST HARNESS** |
| V2-M6C | OpenWrt transparent capture | **PLANNED FINAL SHAPE: TPROXY + POLICY ROUTING** |
| V2-M7A | Windows client capture | **PLANNED FINAL SHAPE: TUN/WINTUN-CLASS L3** |
| V2-M7B | Windows global/split capture + underlay escape | **PLANNED** |
| V2-M8A | Reality-like same-entry bootstrap | **IMPLEMENTED; SIMPLE SHARED USER/PASS PATH QUALIFIED** |
| V2-M8B-T1 | native FakeTCP + WBD FEC first-arrival / pcap qualification | **FOCUSED GATE PASSED** |
| V2-M8B-T2 | fixed FEC presets + immutable setup + periodic low-load refresh | **TRANSPORT REFERENCE RETAINED; LEGACY RECOVERY IS DEFAULT** |
| V2-M8C | shared-account concurrent transport/session fan-out | **CURRENT**; atomic ticket claim + LiveID demux implemented; public FakeTCP/DTLS mux under two-client qualification |
| V2-M9 | optional two-lane striped/hedged/survival research | only if one-lane 100M cliff justifies it |
| V2-M10 | release qualification | protocol regression -> OpenWrt TPROXY one-shot VPN -> Windows TUN one-shot VPN |
| V2-X1 | advanced continuously learning Auto FEC / automatic capacity inference | **FUTURE RESEARCH; NOT REQUIRED** |

## Product order of operations

Development must finish in this order:

1. preserve UDP-like inner semantics and TCP-shaped FakeTCP outer semantics on a 100 Mbit/s weak link;
2. keep fixed FEC, DTLS 1.3 and immutable LINK_INIT/LINK_ACCEPT qualified;
3. finish Reality-like same-entry recognition plus simple shared username/password -> atomic one-time ticket admission;
4. finish one-public-listener multi-association FakeTCP/DTLS + LiveID data-session fan-out and run the full protocol regression matrix;
5. freeze the protocol;
6. integrate OpenWrt **TPROXY** and make one end-to-end VPN attempt succeed from clean state;
7. integrate Windows **TUN/Wintun-class** capture and make one end-to-end VPN attempt succeed from clean state.

Platform work must not change already-qualified transport semantics merely to make routing easier.

## V2-M8B-T1 evidence retained

The native public carrier is WBD-owned TCP-shaped raw packets, not an ordinary kernel TCP byte stream. The focused 20% loss pcap gate demonstrates SYN/SYN-ACK/ACK, MSS, SACK-Permitted, Window Scale, cumulative ACK, merged live SACK ranges, three-duplicate-ACK fast retransmit and RTO backoff while complete out-of-order inner datagrams continue to bypass sequence holes.

The WBD FEC fast path streams systematic source shards immediately and sends repair later. On GitHub Actions full-stack run `32841039689`, all six RTT `20/100 ms` x loss `0/10/20%` points passed. At 20% loss:

- RTT 20 ms: 800/800 delivered, p50 `10.374 ms`, p95 `17.825 ms`, p99 `20.077 ms`;
- RTT 100 ms: 800/800 delivered, p50 `50.379 ms`, p95 `58.115 ms`, p99 `59.769 ms`.

Low-load recovery tests showed SACK/RACK can improve delivery without materially changing first arrival at those loads. The later loaded 100 Mbit/s gate is authoritative for the product default.

## 100 Mbit/s weak-link and recovery decision

Current critical transport qualification uses `rate 100mbit` or lower. A 200 Mbit/s laboratory link may remain in historical benchmark documents, but it is not the current product assumption.

The loaded FakeTCP recovery gate offers `65 Mbit/s` inner payload on a `100 Mbit/s` public link at RTT `20/100 ms` and loss `10/20%`. After fixing a repeated RACK fast-retransmission storm, SACK/RACK still produced only small delivery/goodput gains while adding several milliseconds of p50 and substantially worse p95/p99 queueing latency. That violates the first-complete-inner-datagram priority.

Therefore:

- `wbd-faketcp` product default is **legacy** shadow recovery;
- `sack-rack` remains explicit experimental/low-load research mode;
- loaded recovery A/B remains diagnostic and must keep both modes executable, but release CI does not require experimental SACK/RACK to beat legacy;
- a future advanced recovery path must have explicit lower-priority/bandwidth budgeting before it can replace legacy.

For configured path capacity `C <= 100 Mbit/s`, target utilization `u`, FEC factor `F`, packet/header expansion and shadow retransmission factor `A`, the client may limit inner offered payload approximately as:

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

### T2 retained gates

- fixed-scheduler simulator and first-arrival tests green;
- immutable LINK_INIT/LINK_ACCEPT tests green;
- FEC off and fixed path packet-preserving startup green;
- product recovery default remains legacy until a future loaded candidate beats it on latency-first criteria;
- live fixed preset family implemented only after each candidate is qualified;
- any changed profile is applied by association rotation, never in place.

## V2-M8A — Reality-like same-entry front

Preferred connection setup uses one TCP listener:

```text
ClientHello
  -> recognized marker: same socket TLS 1.3 takeover
       -> one encrypted username/password request
       <- one-time ticket
  -> unrecognized marker: exact bytes continue to fixed fallback target
```

Sustained VPN payload never uses this TLS/TCP stream.

The personal client explicitly supports no server-certificate/hostname verification. The front and DTLS may accept an arbitrary self-signed certificate with a name unrelated to configured SNI. This gives encryption without server certificate identity authentication and is an intentional personal-use tradeoff.

The old target-mirror/witness/DEMO_BIND path remains only as a diagnostic compatibility tool.

## V2-M8C — shared account / concurrent sessions — CURRENT

The server is intentionally not a multi-tenant account service.

- one configured username/password pair is the shared account credential;
- the same pair may be used by several devices simultaneously;
- recognized TLS sends username/password once inside TLS, without an extra nonce/HMAC challenge round trip;
- each successful login gets a fresh random 32-byte one-time ticket;
- product bind atomically claims the ticket and returns its account label; concurrent consumers cannot both use one ticket;
- live identity is ticket/`LiveID`, not username;
- learned DTLS plaintext peer is only a hot routing index;
- each LiveID owns its own immutable `linkdata.Path`, including independent FEC encoder/decoder state;
- no per-device credential database, KDF, revocation table or single-login lock is required;
- simple `max-sessions`/process-resource ceilings are sufficient for this personal deployment.

### Current fan-out architecture

The old FakeTCP and wolfSSL test servers are one-association implementations. V2.2 keeps the cryptographic worker simple and fans out around it:

```text
one public FakeTCP raw listener
  -> raw 4-tuple association table
      -> association A -> loopback UDP -> wolfSSL DTLS worker A --\
      -> association B -> loopback UDP -> wolfSSL DTLS worker B ----> shared WBD link/session server
      -> association C -> loopback UDP -> wolfSSL DTLS worker C --/
                                                     |
                                                     -> peer/LiveID session.DataPlane
```

Implemented pieces:

- `internal/realityfront.ConsumeTicketForAccount`: atomic one-shot ticket claim;
- `internal/session.AccountRegistry`: LiveID identity + peer index, same account may own many sessions;
- `internal/session.DataPlane`: Reserve -> immutable Activate -> peer/LiveID Inbound/Outbound, independent FEC state;
- `internal/faketcp.ServerAssociationTable`: independent handshake/seq/SACK/RTO/no-HOL state per raw 4-tuple;
- `internal/dtlsworker`: bind loopback `:0`, pass socket as inherited fd 3, supervise one worker;
- `native/dtls/wbd_dtls_shim.c`: inherited server transport fd support;
- `cmd/wbd-faketcp-mux`: one public raw listener wired to per-association UDP relay + DTLS worker;
- focused `faketcp-mux-two-client` CI gate: two client namespaces, one public FakeTCP port, two independent DTLS workers and distinct UDP echo markers.

### M8C exit gate

1. association mux + DTLS worker unit/compile gates green;
2. two simultaneous clients share one public FakeTCP listener and complete independent DTLS echo without cross-session data;
3. WBD link server accepts several DTLS plaintext peers through `session.DataPlane`;
4. two devices using the **same username/password** receive distinct tickets, atomically bind them, complete separate LINK_INIT/LINK_ACCEPT and exchange independent traffic;
5. repeat in FEC off and fixed 20:20 modes;
6. re-run 100 Mbit/s first-arrival/full-stack/pcap and record two-session CPU/RSS before protocol freeze.

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
2. shared-account two-client public-listener session fan-out green;
3. start from clean server/client routing state;
4. install the platform VPN capture adapter;
5. establish Reality-like front -> ticket -> FakeTCP -> DTLS 1.3 -> LINK_INIT/LINK_ACCEPT once;
6. pass real DNS plus TCP/UDP application traffic;
7. verify underlay escape and no recursive capture;
8. stop the client and prove routing/firewall state is restored;
9. repeat once for OpenWrt TPROXY and once for Windows TUN.

The target is **one successful clean attempt per platform after the protocol is frozen**, then that sequence becomes the release regression.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- runtime FEC config epochs / mid-session link switching;
- SACK/RACK as unconditional product default after the loaded 100 Mbit/s latency failure;
- continuously learning/high-frequency Auto FEC on the current critical path;
- mandatory per-device credential/revocation infrastructure;
- mandatory client certificate-chain/hostname verification in personal mode;
- VLESS/Xray routing/Vision stream semantics as the data plane;
- WireGuard inner glue;
- Android/no-root;
- blind default multi-lane duplication.
