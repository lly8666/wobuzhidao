# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- a TCP-shaped raw/FakeTCP public carrier;
- UDP/datagram-like payload semantics with no ordinary-TCP retransmission/HOL dependency;
- optional WBD-owned FEC that can be off, fixed, or adaptively tuned after qualification;
- standards-compliant DTLS 1.3 for authentication, encryption, integrity and anti-replay;
- native TUN/L3 ingress/egress as a platform layer;
- optional full-tunnel and China/non-China split capture on supported clients;
- an optional TLS Persona bootstrap with browser-like ClientHello profiles;
- a minimal account/session model in which one account may own multiple concurrent device sessions.

The endpoints are operator-controlled devices with sufficient privileges for raw sockets, limited RST/filter handling, TUN, pcap/Npcap and Wintun-class packet I/O. Android and unprivileged/no-root portability are out of scope.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004.

## Non-negotiable data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The required public carrier is udp2raw-compatible FakeTCP: TCP-shaped raw packets carrying unordered/real-time datagrams.
3. **TCP-shaped does not mean kernel-TCP-owned.** WBD does not require a real `ESTABLISHED` kernel TCP socket for the product lane.
4. Kernel TCP retransmission, ordered delivery, congestion-control HOL and byte-stream sequence ownership must not become dependencies of product payload delivery.
5. WBD FEC is systematic and optional. A systematic source that is ready to send must not be intentionally held merely to wait for a repair block.
6. UDPspeeder mode 0 remains an external fixed-mode reference: `20:10` for a 1.5x reference point and `20:20` for a 2.0x strong-loss reference point.
7. Normal one-lane proactive source+repair bytes stay at or below 2.0x until benchmark evidence explicitly changes this constitution. An explicit future emergency multi-lane survival mode may exceed 2.0x only after a separate admission/qualification decision; it is never an Auto default.
8. One raw lane is the product baseline. Extra lanes remain optional optimizations and must demonstrate value under measured cross-lane loss/latency correlation.
9. FEC/lane optimization is judged by earliest complete original datagram, delivery, CPU/RSS and total wire cost together; block completion time alone is not a product metric.

## Security invariants

1. The steady-state WBD security layer is **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable` commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. Server identity uses an operator-controlled X.509 certificate. Client validation uses native trust-chain + hostname verification; optional SPKI pinning is additive.
4. 0-RTT remains disabled until replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data. WBD does not invent a second AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams so one lost record does not block later records.
7. Account authorization occurs only inside the authenticated DTLS association. The same account may own multiple simultaneous device sessions; session state is never keyed by username alone.
8. High-entropy per-device access tokens/keys are preferred so one device can be revoked independently. Human-memorable passwords, if ever added, require a proper password KDF.

## Optional TLS Persona

TLS Persona is an **optional connection-establishment feature**, not the VPN data cipher and not a correctness prerequisite.

Allowed goals include a real standard TLS 1.3 preflight to an operator-controlled endpoint, maintained browser-like ClientHello profiles (`native`, `chrome`, `firefox`, `safari`, `edge`), explicit certificate validation, and an optional short-lived bootstrap binding.

Rules:

- Persona must remain isolated from the unordered DTLS/FEC data plane.
- Persona must never silently disable DTLS verification.
- Browser-like fingerprints change connection appearance/interoperability; they do not make cryptography stronger.
- Standard TLS preflight and the FakeTCP/DTLS data lane are separate protocol roles.
- The browser ClientHello profile is a client/session choice from the server-advertised supported set.
- Persona hostname, certificate and private key are operator/server assets; the client validates an allowed operator-controlled hostname and cannot invent an unrelated identity.
- Do not borrow third-party private keys/certificates.
- Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Protection modes and configuration ownership

The product FEC surface is conceptually:

- `off`: no proactive repair; FakeTCP shadow retransmission and DTLS remain active;
- `fixed`: an admitted `K:R`/scheduler profile chosen by the client, including reference presets such as `20:10` and `20:20`;
- `auto`: a client-side controller that may choose directional protection from measured loss, recovery latency, RTT, goodput and queue pressure **only after estimator/controller qualification**.

FEC may differ by direction. Runtime changes use a negotiated configuration epoch and take effect only at coding-window boundaries so old shards remain unambiguous and no reconnect is required.

Most optional behavior is client/session-owned: capture mode, FEC policy/target, Persona profile, and optional lane policy. The server remains intentionally simple but is not policy-less: it advertises supported protocol/code versions and hard resource ceilings, then accepts, clamps or rejects client proposals. A client must not be able to force unbounded server memory, CPU, wire amplification, MTU or lane count.

## Account/session model

WBD is not intended to become a general multi-tenant SaaS control plane, but the server **must** support several concurrent sessions/devices under the same account username.

- `username` identifies an account principal, not a transport session.
- live state is keyed by `(account_id, session_id)` or a stronger equivalent.
- one account may hold multiple device credentials and simultaneous sessions.
- per-device credential revocation should not require changing all other devices.
- optional account limits such as concurrent-session caps are server policy; FEC/routing/Persona settings remain session-local and client-proposed.

## Client capture / routing modes

Supported client policy targets are:

- `off` / manual routing;
- `global` full-tunnel capture;
- `only-cn` capture only destinations classified in the China prefix database;
- `only-non-cn` capture destinations outside that database.

Every mode has a mandatory **underlay escape invariant**: server endpoints, Persona/bootstrap endpoints, and required local-link control traffic must continue through the original physical/default route and must never recursively enter the tunnel.

Linux/OpenWrt should use TUN plus policy routing and compact kernel prefix/interval sets rather than thousands of per-prefix firewall rules. Windows should use Wintun-class L3 I/O; global capture uses broad routes plus explicit endpoint escape routes, while split capture must choose a compact aggregated-route or WFP/equivalent classifier design rather than a huge persistent Windows Firewall ruleset.

CIDR membership is longest-prefix matching, not a naive exact-address hash table. A portable radix/Patricia-style classifier is acceptable; platform-native interval/prefix sets may be used where superior. The domestic prefix database is versioned and atomically replaceable for IPv4 and IPv6.

## Product stack

```text
TUN / IP packet
        ↓
client capture / split policy
        ↓
minimal WBD packet framing + session/control
        ↓
optional FEC (off/fixed/qualified-auto)
        ↓
DTLS 1.3
        ↓
udp2raw-compatible WBD FakeTCP raw lane(s)
        ↓
public network
```

For transport-only qualification, the TUN/platform layer may be replaced by a packet/datagram generator and echo sink.

## Qualification phases

### Phase A — one-lane transport qualification

The current WBD-owned one-lane stack is qualified from the perspective of first-complete datagram behavior before platform integration is broadened.

Always record:

- connection/handshake success and time;
- earliest-complete delivery ratio and p50/p95/p99/max;
- throughput/goodput;
- later-datagram bypass / out-of-order evidence relevant to UDP-like no-HOL behavior;
- actual FakeTCP outer packet counts, flags, sequence/ACK/SACK/retransmission progression and RST/FIN anomalies;
- FEC direct-vs-reconstructed first availability;
- per-component and total CPU time;
- CPU per delivered MiB;
- per-component and aggregate peak RSS;
- wire bytes, repair bytes and retransmit bytes separately.

Random-loss surfaces are necessary but not sufficient. Burst/correlated loss, offered-load/capacity pressure and MTU are targeted after the core scheduler/controller is defined.

### Phase B — adaptive FEC / control qualification

Compare current tail-repair systematic RS against lower-latency micro-block and causal/sliding-window repair schedules. `off` and `fixed` runtime switching plus configuration epochs are implemented before `auto`. Auto is admitted only after estimator stability, hysteresis, resource caps and first-arrival regression tests pass.

### Phase C — Persona / account / platform policy

Implement and qualify optional browser-like TLS Persona, account + multi-device session authorization, Linux/OpenWrt capture modes, then Windows Wintun/global/split policy. Platform tests must verify underlay escape and restoration/cleanup behavior.

### Phase D — optional multi-lane research

Only after one-lane adaptive protection has a measured cliff may a second raw lane be admitted. Prefer striped/hedged independent repairs over blind duplication. Any emergency >2x survival mode requires an explicit constitution update backed by measurements.

## Interpretation of TCP-like and UDP-like

**TCP-like outer** means the public carrier presents qualified TCP-shaped packet/state behavior, including the selected shadow ACK/SACK/retransmission semantics, without making ordinary kernel TCP the product reliability owner.

**UDP-like inner** means later independent application/FEC datagrams can make progress despite loss of an earlier datagram; the data plane does not acquire an ordered-byte-stream HOL dependency.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: retired.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.
- Two raw lanes: optional post-one-lane optimization only.

## Development discipline

- Preserve already-qualified WBD FEC/DTLS/FakeTCP evidence and exact upstream pins.
- Optimize from first-arrival + delivery + resource + wire measurements, not intuition or block-code aesthetics.
- Do not delay an available systematic source merely to fill a FEC block.
- Do not enable Auto FEC until the estimator/controller has bounded-state tests and hysteresis/failure behavior.
- Keep client configurability high but retain server capability/resource ceilings.
- Do not implement split routing using thousands of persistent Windows Firewall rules.
- Do not enable dual lane by default; measure cross-lane correlation first.
- Every substantive stage ends with repository-backed tests and handoff.
