# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- a TCP-shaped raw/FakeTCP public carrier;
- UDP/datagram-like payload semantics with no ordinary-TCP retransmission/HOL dependency;
- optional WBD-owned FEC that is currently `off` or an explicit fixed profile;
- standards-compliant DTLS 1.3 for authentication, encryption, integrity and anti-replay;
- native TUN/L3 ingress/egress as a platform layer;
- optional full-tunnel and China/non-China split capture on supported clients;
- an optional TLS Persona bootstrap with browser-like ClientHello profiles;
- a minimal account/session model in which one account may own multiple concurrent device sessions.

Continuously learning/high-frequency Auto FEC is deliberately deferred to a future advanced-research milestone. V2.2 may use a much narrower **periodic fixed-profile refresh**: every configurable 30/60 minutes, wait for a low-load window, estimate current FakeTCP first-transmission loss from existing sender counters, choose from a small qualified fixed preset table, and apply any changed preset only on a fresh association.

The endpoints are operator-controlled devices with sufficient privileges for raw sockets, limited RST/filter handling, TUN, pcap/Npcap and Wintun-class packet I/O. Android and unprivileged/no-root portability are out of scope.

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

## Immutable link setup

All parameters that affect one WBD data association are fixed during establishment.

The product startup sequence is conceptually:

```text
(optional TLS Persona preflight)
        -> FakeTCP association
        -> DTLS 1.3 association
        -> WBD LINK_INIT proposal
        -> WBD LINK_ACCEPT
        -> AUTH / AUTH_OK when required
        -> Established immutable data association
```

The current LINK_INIT/LINK_ACCEPT implementation carries one fixed data-lane config and accepts the exact proposal or rejects it. Asymmetric periodic refresh will evolve establishment to carry immutable client-TX and server-TX FEC profiles: the client chooses its transmit profile from its own low-load sender sample; the server selects its transmit profile from its own last good low-load sample and returns it in LINK_ACCEPT. Shared MTU/lane/protocol parameters remain explicit and bounded.

Once the association reaches Established, link parameters are immutable. There is no runtime FEC config epoch and no mid-session parameter-control path. To change FEC, MTU, lane count, scheduler or another link-defining transport setting, establish a new association and switch over after it reaches Established.

A periodic fixed-profile refresh therefore means **association rotation**, preferably make-before-break. It never means changing the codec of an existing association in place.

Legacy one-shot M3E CONFIG frames may remain in-tree for historical compatibility tests but are not part of the current product data-association path.

## Security invariants

1. The steady-state WBD security layer is **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable` commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. Server identity uses an operator-controlled X.509 certificate. Client validation uses native trust-chain + hostname verification; optional SPKI pinning is additive.
4. 0-RTT remains disabled until replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data. WBD does not invent a second AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams so one lost record does not block later records.
7. Account authorization occurs only inside the authenticated DTLS association. The same account username may own multiple simultaneous device sessions; session state is never keyed by username alone.
8. High-entropy per-device access tokens/keys are preferred so one device can be revoked independently. Human-memorable passwords, if ever added, require a proper password KDF.

## Optional TLS Persona

TLS Persona is an **optional connection-establishment feature**, not the VPN data cipher and not a correctness prerequisite.

Allowed goals include a real standard TLS 1.3 preflight to an endpoint identity the operator is authorized to use, maintained browser-like ClientHello profiles (`native`, `chrome`, `firefox`, `safari`, `edge`), explicit certificate validation, and an optional short-lived bootstrap binding.

Rules:

- Persona must remain isolated from the unordered DTLS/FEC data plane.
- Persona must never silently disable DTLS or certificate verification.
- Browser-like fingerprints change connection appearance/interoperability; they do not make cryptography stronger.
- Standard TLS preflight and the FakeTCP/DTLS data lane are separate protocol roles.
- The browser ClientHello profile is a client/session choice from the server-supported set.
- Persona hostname, certificate and private key are operator/server assets and must be identities the operator is authorized to use.
- Public third-party services may be used as measurement baselines, but WBD does not borrow third-party private keys/certificates or present an unrelated third-party identity as its own endpoint.
- A diagnostic tool may connect genuinely to a public control site and record its real certificate/SPKI fingerprint and network performance. Copying a certificate fingerprint is not a substitute for the private key required by TLS CertificateVerify.
- Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Fixed FEC policy

The **current product** FEC surface is:

- `off`: no proactive repair; FakeTCP shadow retransmission and DTLS remain active;
- `fixed`: an admitted `K:R`/scheduler profile chosen at link establishment.

Advanced continuously adapting `auto` remains a reserved future research value. The current periodic refresh is deliberately simpler: a low-frequency classifier chooses only among qualified fixed presets, and a changed choice takes effect only on the next association.

Uplink and downlink profiles may differ. Each endpoint measures the direction it sends using unique first-retransmission marks from the existing FakeTCP sender. Profile selection uses a conservative confidence bound from a low-load observation window rather than raw total retransmission-attempt counts.

The target fixed preset family is `off`, `20:4`, `20:8`, `20:12`, `20:16`, `20:20`, with the strongest single-lane proactive source+repair ratio still capped at 2.0x. At present the live WBD codec admits only `off` and systematic `20:20` tail-RS; intermediate presets must be implemented and qualified before establishment may advertise them.

For a configured physical capacity `C`, a selected fixed FEC factor and measured shadow-retransmission factor define an inner offered-rate ceiling so repair/retransmission traffic does not saturate the path. The limiter must preserve immediate systematic-source forwarding and must not become an ordered/congestion-controlled TCP dependency.

Most optional behavior remains client/session-owned at establishment: capture mode, client transmit FEC preference, Persona profile, and optional future lane mode. The server owns its transmit-profile selection from its own low-load measurement plus hard capability/resource ceilings. A client must not be able to force unbounded server memory, CPU, wire amplification, MTU or lane count.

## Account/session model

WBD is not intended to become a general multi-tenant SaaS control plane, but the server **must** support several concurrent sessions/devices under the same account username.

- `username` identifies an account principal, not a transport session.
- live state is keyed by `(account_id, session_id)` or a stronger equivalent.
- one account may hold multiple device credentials and simultaneous sessions.
- per-device credential revocation should not require changing all other devices.
- optional account limits such as concurrent-session caps are server policy; FEC/routing/Persona settings remain session-local and are selected at establishment.

## Client capture / routing modes

Supported client policy targets are:

- `off` / manual routing;
- `global` full-tunnel capture;
- `only-cn` capture only destinations classified in the China prefix database;
- `only-non-cn` capture destinations outside that database.

Every mode has a mandatory **underlay escape invariant**: server endpoints, Persona/bootstrap endpoints, and required local-link control traffic must continue through the original physical/default route and must never recursively enter the tunnel.

Linux/OpenWrt should use TUN plus policy routing and compact kernel prefix/interval sets rather than thousands of per-prefix firewall rules. Windows should use Wintun-class L3 I/O; global capture uses broad routes plus explicit endpoint escape routes, while split capture must use compact route/WFP/equivalent classification rather than a huge persistent Windows Firewall ruleset.

CIDR membership is longest-prefix matching, not a naive exact-address hash table. A portable radix/Patricia-style classifier is acceptable; platform-native interval/prefix sets may be used where superior. The domestic prefix database is versioned and atomically replaceable for IPv4 and IPv6.

## Product stack

```text
TUN / IP packet
        ↓
client capture / split policy + configured-capacity rate guard
        ↓
minimal WBD framing + immutable startup negotiation
        ↓
optional fixed FEC
        ↓
DTLS 1.3
        ↓
udp2raw-compatible WBD FakeTCP raw lane
        ↓
public network
```

## Qualification phases

### Phase A — one-lane transport qualification

Always record connection/handshake success, earliest-complete delivery p50/p95/p99/max, goodput, no-HOL/out-of-order evidence, FakeTCP ACK/SACK/retransmission behavior, FEC direct-vs-reconstructed availability, CPU/RSS, repair bytes and retransmit bytes.

### Phase B — fixed FEC / immutable setup / periodic refresh — CURRENT

Compare current tail-repair systematic RS against lower-latency research schedules offline. Keep the live codec unchanged until a candidate wins delivery/tail/resource/wire gates.

Qualify immutable `LINK_INIT/LINK_ACCEPT`, then add the narrow periodic fixed-profile refresh defined by ADR-0007: 20-second low-load sender-counter sample every configured 30/60 minutes, conservative fixed-preset lookup, configured-capacity inner-rate ceiling, and fresh-association application of changed parameters.

Implement and qualify live systematic fixed presets `20:4/8/12/16/20` before the selector may choose them in production. No in-place FEC transition is allowed.

Advanced continuously learning Auto FEC is **not part of Phase B**.

### Phase C — Persona / account / platform policy

Implement and qualify optional browser-like TLS Persona and network-treatment diagnostics, account + multi-device session authorization, Linux/OpenWrt capture modes, then Windows Wintun/global/split policy. Platform tests must verify underlay escape and restoration/cleanup behavior.

### Phase D — optional multi-lane research

Only after one-lane fixed protection has a measured cliff may a second raw lane be admitted. Prefer striped/hedged independent repairs over blind duplication. Any emergency >2x survival mode requires explicit qualification.

## Interpretation of TCP-like and UDP-like

**TCP-like outer** means the public carrier presents qualified TCP-shaped packet/state behavior, including selected shadow ACK/SACK/retransmission semantics, without making ordinary kernel TCP the product reliability owner.

**UDP-like inner** means later independent application/FEC datagrams can make progress despite loss of an earlier datagram; the data plane does not acquire an ordered-byte-stream HOL dependency.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: retired.
- Runtime config epochs / mid-session FEC switching: rejected for the current product path; reconnect/rotate instead.
- High-frequency continuously learning Auto FEC and automatic capacity inference: future advanced research only.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.
- Two raw lanes: optional post-one-lane optimization only.

## Development discipline

- Preserve already-qualified WBD FEC/DTLS/FakeTCP evidence and exact upstream pins.
- Optimize from first-arrival + delivery + resource + wire measurements, not intuition or block-code aesthetics.
- Do not delay an available systematic source merely to fill a FEC block.
- Do not add a mid-session link-parameter control plane; changing parameters means a fresh association.
- Keep periodic adaptation deliberately coarse: low-load observation, fixed table, configured capacity, association rotation. Do not turn it into the deferred advanced Auto controller.
- Keep client configurability high at establishment while allowing the server to select its own transmit FEC from its local path measurement and retain resource ceilings.
- Do not implement split routing using thousands of persistent Windows Firewall rules.
- Do not enable dual lane by default.
- Every substantive stage ends with repository-backed tests and handoff.