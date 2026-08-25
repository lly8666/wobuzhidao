# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- a TCP-shaped raw/FakeTCP public carrier;
- UDP/datagram-like payload semantics with no ordinary-TCP retransmission/HOL dependency;
- optional WBD-owned FEC that is currently `off` or an explicit fixed profile;
- standards-compliant DTLS 1.3 for encryption, integrity and anti-replay;
- a Reality-like same-entry TLS front used only for connection recognition and shared username/password admission;
- OpenWrt final transparent capture through **TPROXY**;
- Windows final client capture through a **TUN/Wintun-class L3 adapter**;
- Linux TUN retained as a development/reference harness rather than the OpenWrt release shape;
- optional full-tunnel and China/non-China split capture on supported clients;
- one shared account username/password allowed to own multiple concurrent device sessions.

The current weak-network qualification ceiling is **100 Mbit/s physical link capacity**. Tests may use lower rates, but current product tuning must not assume a 200 Mbit/s path. Higher-capacity optimization is out of the critical path until the 100 Mbit/s release gate is complete.

Continuously learning/high-frequency Auto FEC is deliberately deferred to a future advanced-research milestone. V2.2 may use a much narrower periodic fixed-profile refresh: every configurable 30/60 minutes, wait for a low-load window, estimate current FakeTCP first-transmission loss from existing sender counters, choose from a small qualified fixed preset table, and apply any changed preset only on a fresh association.

The endpoints are operator-controlled devices with sufficient privileges for raw sockets, limited RST/filter handling, TPROXY/policy routing on OpenWrt, pcap/Npcap and TUN/Wintun-class packet I/O on Windows. Android and unprivileged/no-root portability are out of scope.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004.

## Non-negotiable data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The required public carrier is udp2raw-compatible FakeTCP: TCP-shaped raw packets carrying unordered/real-time datagrams.
3. **TCP-shaped does not mean kernel-TCP-owned.** WBD does not require a real `ESTABLISHED` kernel TCP socket for the product lane.
4. Shadow ACK/SACK/retransmission may continue for TCP-like outer behavior, but ordinary kernel-TCP ordered delivery, congestion-control HOL and byte-stream ownership must not become dependencies of product payload delivery.
5. WBD FEC is systematic and optional. **Do not delay an available systematic source merely to fill a FEC block.**
6. UDPspeeder mode 0 remains an external fixed-mode reference. Compatibility names remain `weak-1.5x` = `20:10` and `weak-2x` = `20:20`.
7. Normal one-lane proactive source+repair bytes stay at or below 2.0x until benchmark evidence explicitly changes this constitution. An explicit future emergency multi-lane survival mode may exceed 2.0x only after separate qualification.
8. One raw lane is the product baseline. Extra lanes remain optional optimizations.
9. FEC/lane optimization is judged by earliest complete original datagram, delivery, CPU/RSS and total wire cost together; block completion time alone is not a product metric.
10. Do not cap or delay new inner datagrams merely to reduce retransmission/FEC memory. Inner transport performance is the first optimization priority.
11. The inner offered-rate limiter may account for known FEC/header/retransmission expansion to avoid self-induced queue saturation; this is a physical-capacity guard, not shadow-TCP congestion control.
12. Current release qualification assumes a maximum configured physical capacity of 100 Mbit/s. Recovery/FEC changes that only look acceptable on a faster test link are not qualified.

## Immutable link setup

All parameters that affect one WBD data association are fixed during establishment.

The product startup sequence is conceptually:

```text
(optional Reality-like TLS front: recognition + username/password -> one-time ticket)
        -> FakeTCP association
        -> DTLS 1.3 association
        -> one-time ticket bind when the front is used
        -> WBD LINK_INIT proposal
        -> WBD LINK_ACCEPT
        -> Established immutable data association
```

A ticket admitted by the Reality-like front is already the result of account authentication. The ticket path therefore does not add a second bearer-token/password exchange inside DTLS. Legacy/non-front test modes may still retain the older AUTH frame for compatibility until removed.

The current LINK_INIT/LINK_ACCEPT implementation carries one fixed data-lane config and accepts the exact proposal or rejects it. Asymmetric periodic refresh may evolve establishment to carry immutable client-TX and server-TX FEC profiles. Shared MTU/lane/protocol parameters remain explicit and bounded.

Once the association reaches Established, link parameters are immutable. There is no runtime FEC config epoch and no mid-session parameter-control path. To change FEC, MTU, lane count, scheduler or another link-defining transport setting, establish a new association and switch over after it reaches Established.

A periodic fixed-profile refresh therefore means **association rotation**, preferably make-before-break. It never means changing the codec of an existing association in place.

Legacy one-shot M3E CONFIG frames may remain in-tree for historical compatibility tests but are not part of the current product data-association path.

## Security and admission invariants

1. The steady-state WBD security layer is **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable` commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. In the personal product mode, encryption is required but server certificate-chain and hostname verification are **not required**. The client may explicitly accept an arbitrary/self-signed certificate and unrelated hostname. This means the transport is encrypted without server-identity authentication; that tradeoff is intentional for this single-operator deployment.
4. 0-RTT remains disabled until replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data. WBD does not invent a second AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams so one lost record does not block later records.
7. Product account admission is deliberately simple: recognized Reality-like TLS sessions send the shared `username + password` once inside TLS; the server performs bounded constant-time equality checks and returns an independent one-time ticket.
8. The **same username/password may authenticate multiple simultaneous devices/sessions**. No per-device credential, KDF, revocation database or multi-tenant account system is required for the personal product path.
9. Each successful login produces a fresh random session ticket. Live session identity must therefore be ticket/session based, never username alone.
10. Username/password, one-time ticket bytes, WBDC control and application plaintext must not appear in public-path captures.

## Reality-like same-entry front

The optional front is a connection-establishment adapter, not the VPN data cipher and not a data-stream carrier.

One TCP listener first reads the ClientHello. A recognized WBD marker causes the **same socket** to be locally taken over by TLS 1.3; an unrecognized ClientHello is forwarded byte-for-byte to the configured fallback target. Sustained VPN payload never runs in this ordinary TLS/TCP connection.

The personal client defaults to `verify-server=false`: the SNI may be chosen for the configured front/fallback appearance while the locally presented WBD certificate may be self-signed and unrelated. This is explicit configuration, not hidden verification failure.

After recognized TLS establishment the client sends one bounded username/password request. The server compares it against the one configured shared pair and, on success, creates a new one-time ticket. Multiple concurrent requests using the same pair are valid and must receive distinct tickets.

The older target-mirror/witness path may remain as a diagnostic compatibility tool but is not the preferred product connection join. Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Fixed FEC policy

The current product FEC surface is:

- `off`: no proactive repair; FakeTCP shadow retransmission and DTLS remain active;
- `fixed`: an admitted `K:R`/scheduler profile chosen at link establishment.

Advanced continuously adapting `auto` remains a reserved future research value. The current periodic refresh is deliberately simpler: a low-frequency classifier chooses only among qualified fixed presets, and a changed choice takes effect only on the next association.

Uplink and downlink profiles may differ. Each endpoint measures the direction it sends using unique first-retransmission marks from the existing FakeTCP sender. Profile selection uses a conservative confidence bound from a low-load observation window rather than raw total retransmission-attempt counts.

The target fixed preset family is `off`, `20:4`, `20:8`, `20:12`, `20:16`, `20:20`, with the strongest single-lane proactive source+repair ratio still capped at 2.0x. At present the live WBD codec admits only `off` and systematic `20:20` tail-RS; intermediate presets must be implemented and qualified before establishment may advertise them.

For configured physical capacity `C <= 100 Mbit/s`, a selected fixed FEC factor and measured shadow-retransmission factor define an inner offered-rate ceiling so repair/retransmission traffic does not saturate the path. The limiter must preserve immediate systematic-source forwarding and must not become an ordered/congestion-controlled TCP dependency.

## Account/session model

WBD is a personal single-account-style server, not a multi-tenant control plane.

- one configured `username/password` pair may be reused by several devices at the same time;
- `username` identifies the shared account, not a transport session;
- each successful front login produces a fresh random one-time ticket/session identity;
- live state is keyed by ticket/session identity (optionally with the account label for logs), never by username alone;
- simultaneous-session count may be bounded only by simple process/resource limits such as `max-conns`; there is no required per-device revocation/cap database;
- link/FEC/routing choices remain session-local and immutable for each established association.

## Client capture / routing modes

Supported client policy targets are:

- `off` / manual routing;
- `global` full capture;
- `only-cn`;
- `only-non-cn`.

Every mode has a mandatory **underlay escape invariant**: server endpoints and front/bootstrap endpoints must continue through the original physical/default route and must never recursively enter the tunnel.

### OpenWrt release shape

OpenWrt final product capture uses **TPROXY**, not a TUN device. nftables/iptables TPROXY plus policy routing redirects selected TCP/UDP traffic to the local WBD transparent adapter while explicit marks/routes exempt the WBD underlay connection itself. Global/split policy should use compact nft sets/ipsets rather than thousands of individual rules.

The existing Linux TUN bridge remains useful for protocol regression, packet-preservation tests and server/Linux experiments, but passing that harness is not the final OpenWrt release gate.

### Windows release shape

Windows final product capture uses a **TUN/Wintun-class L3 adapter**. Full-tunnel mode uses broad routes plus explicit underlay endpoint escapes. Split mode must use compact route/WFP/equivalent classification rather than thousands of persistent Windows Firewall rules.

CIDR membership is longest-prefix matching, not a naive exact-address hash table. A portable radix/Patricia-style classifier is acceptable; platform-native interval/prefix sets may be used where superior. The domestic prefix database is versioned and atomically replaceable for IPv4 and IPv6.

## Product stack

```text
OpenWrt: TPROXY TCP/UDP adapter       Windows: TUN/Wintun L3 adapter
                  \                     /
                   \-> WBD packet/session adapter
                         ↓
              configured-capacity rate guard
                         ↓
              immutable startup negotiation
                         ↓
                 optional fixed FEC
                         ↓
                     DTLS 1.3
                         ↓
             WBD FakeTCP raw TCP-shaped lane
                         ↓
                    public network
```

## Qualification phases

### Phase A — one-lane transport qualification

Always record connection/handshake success, earliest-complete delivery p50/p95/p99/max, goodput, no-HOL/out-of-order evidence, FakeTCP ACK/SACK/retransmission behavior, FEC direct-vs-reconstructed availability, CPU/RSS, repair bytes and retransmit bytes. Current critical transport runs use a 100 Mbit/s physical-link ceiling.

### Phase B — fixed FEC / immutable setup / periodic refresh — CURRENT

Compare current tail-repair systematic RS against lower-latency research schedules offline. Keep the live codec unchanged until a candidate wins delivery/tail/resource/wire gates.

Qualify immutable `LINK_INIT/LINK_ACCEPT`, then add the narrow periodic fixed-profile refresh: 20-second low-load sender-counter sample every configured 30/60 minutes, conservative fixed-preset lookup, configured-capacity inner-rate ceiling, and fresh-association application of changed parameters.

Implement and qualify live systematic fixed presets `20:4/8/12/16/20` before the selector may choose them in production. No in-place FEC transition is allowed.

Advanced continuously learning Auto FEC is **not part of Phase B**.

### Phase C — front / account / platform integration

Finish the same-entry Reality-like front, shared-credential multi-session ticket admission and full protocol regression. Then integrate the frozen protocol into the two actual client shapes: OpenWrt TPROXY and Windows TUN.

### Phase D — release one-shot VPN gate

The final milestone is not another isolated transport benchmark. It must start from a clean client/server state, install/configure the VPN adapter, establish the front + FakeTCP + DTLS + immutable link once, pass real application traffic, verify underlay escape, and cleanly restore routing/firewall state. The release target is **one successful end-to-end attempt** on OpenWrt TPROXY and one on Windows TUN after all protocol gates are already green.

### Phase E — optional multi-lane research

Only after the one-lane 100 Mbit/s weak-link product has a measured cliff may a second raw lane be admitted. Prefer striped/hedged independent repairs over blind duplication.

## Interpretation of TCP-like and UDP-like

**TCP-like outer** means the public carrier presents qualified TCP-shaped packet/state behavior, including selected shadow ACK/SACK/retransmission semantics, without making ordinary kernel TCP the product reliability owner.

**UDP-like inner** means later independent application/FEC datagrams can make progress despite loss of an earlier datagram; the data plane does not acquire an ordered-byte-stream HOL dependency.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: retired.
- Runtime config epochs / mid-session FEC switching: rejected; reconnect/rotate instead.
- High-frequency continuously learning Auto FEC and automatic capacity inference: future advanced research only.
- Per-device credential/revocation control plane: not required for the personal product.
- Mandatory certificate-chain/hostname verification: not required in the explicit personal insecure-verification mode.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.
- Two raw lanes: optional post-one-lane optimization only.

## Development discipline

- Preserve already-qualified WBD FEC/DTLS/FakeTCP evidence and exact upstream pins.
- Optimize from first-arrival + delivery + resource + wire measurements, not intuition or block-code aesthetics.
- Do not delay an available systematic source merely to fill a FEC block.
- Do not add a mid-session link-parameter control plane; changing parameters means a fresh association.
- Use 100 Mbit/s as the current weak-link capacity ceiling in critical qualification.
- Keep account admission deliberately simple: one shared username/password, many independent session tickets.
- Do not silently reintroduce mandatory certificate or hostname verification into the personal client.
- OpenWrt final capture is TPROXY; Windows final capture is TUN/Wintun-class.
- Do not implement split routing using thousands of persistent Windows Firewall rules.
- Do not enable dual lane by default.
- Every substantive stage ends with repository-backed tests and handoff.
