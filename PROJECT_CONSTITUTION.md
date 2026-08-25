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

Auto FEC is deliberately deferred to a future advanced-research milestone. It is not required for V2.2 and must not delay the fixed-mode product path.

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

## Immutable link setup

All parameters that affect one WBD data association are fixed during establishment.

The product startup sequence is conceptually:

```text
(optional TLS Persona preflight)
        -> FakeTCP association
        -> DTLS 1.3 association
        -> WBD LINK_INIT proposal
        -> WBD LINK_ACCEPT exact echo / reject
        -> AUTH / AUTH_OK when required
        -> Established immutable data association
```

`LINK_INIT` carries the fixed data-lane proposal needed by the current implementation, including FEC `off|fixed`, coding geometry/scheduler, FEC flush timing, MTU and lane count. The server validates capabilities/resource ceilings and either accepts the **exact** proposal or rejects it; it does not silently rewrite parameters.

Once the association reaches Established, link parameters are immutable. There is no runtime FEC config epoch and no mid-session parameter-control path. To change FEC, MTU, lane count, scheduler or another link-defining transport setting, close the association and establish a new one with a new LINK_INIT.

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
- Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Fixed FEC policy

The **current product** FEC surface is:

- `off`: no proactive repair; FakeTCP shadow retransmission and DTLS remain active;
- `fixed`: an admitted `K:R`/scheduler profile chosen by the client at link establishment.

`auto` remains a reserved future advanced-research value. It is not implemented or accepted in the current product path.

Uplink and downlink profiles may differ, but each negotiated direction is fixed for the lifetime of the association. Changing a profile requires reconnect.

The current live WBD codec is systematic `20:20` tail-RS. `20:10`, micro-block and causal/sliding profiles remain external/reference/research values until their live implementations are qualified; unsupported proposals fail establishment rather than being silently substituted.

Most optional behavior is client/session-owned at establishment: capture mode, fixed FEC profile, Persona profile, and optional future lane mode. The server advertises supported protocol/code versions and hard resource ceilings, then accepts or rejects the proposal. A client must not be able to force unbounded server memory, CPU, wire amplification, MTU or lane count.

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
client capture / split policy
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

### Phase B — fixed FEC / immutable link-setup qualification — CURRENT

Compare current tail-repair systematic RS against lower-latency micro-block and causal/sliding-window research schedules offline. Keep the live codec unchanged until a candidate wins delivery/tail/resource/wire gates.

Implement and qualify one-time `LINK_INIT/LINK_ACCEPT` for `off|fixed`, MTU, lane count and current live profile. After Established, parameters are immutable and changes require a fresh association.

Auto FEC is **not part of Phase B**. It is deferred to a future advanced-research milestone.

### Phase C — Persona / account / platform policy

Implement and qualify optional browser-like TLS Persona, account + multi-device session authorization, Linux/OpenWrt capture modes, then Windows Wintun/global/split policy. Platform tests must verify underlay escape and restoration/cleanup behavior.

### Phase D — optional multi-lane research

Only after one-lane fixed protection has a measured cliff may a second raw lane be admitted. Prefer striped/hedged independent repairs over blind duplication. Any emergency >2x survival mode requires explicit qualification.

## Interpretation of TCP-like and UDP-like

**TCP-like outer** means the public carrier presents qualified TCP-shaped packet/state behavior, including selected shadow ACK/SACK/retransmission semantics, without making ordinary kernel TCP the product reliability owner.

**UDP-like inner** means later independent application/FEC datagrams can make progress despite loss of an earlier datagram; the data plane does not acquire an ordered-byte-stream HOL dependency.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: retired.
- Runtime config epochs / mid-session FEC switching: rejected for the current product path; reconnect instead.
- Auto FEC: future advanced research only.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.
- Two raw lanes: optional post-one-lane optimization only.

## Development discipline

- Preserve already-qualified WBD FEC/DTLS/FakeTCP evidence and exact upstream pins.
- Optimize from first-arrival + delivery + resource + wire measurements, not intuition or block-code aesthetics.
- Do not delay an available systematic source merely to fill a FEC block.
- Do not add a mid-session link-parameter control plane; changing parameters means a fresh association.
- Do not implement Auto FEC on the current V2.2 path.
- Keep client configurability high at establishment while retaining server capability/resource ceilings.
- Do not implement split routing using thousands of persistent Windows Firewall rules.
- Do not enable dual lane by default.
- Every substantive stage ends with repository-backed tests and handoff.