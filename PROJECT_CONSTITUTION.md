# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- a TCP-shaped raw/FakeTCP public carrier;
- UDP/datagram-like payload semantics with no ordinary-TCP retransmission/HOL dependency;
- fixed-mode FEC for weak-network recovery;
- standards-compliant DTLS 1.3 for authentication, encryption, integrity and anti-replay;
- native TUN/L3 ingress/egress as a platform layer;
- an optional TLS Persona bootstrap with browser-like ClientHello profiles.

The endpoints are operator-controlled devices with sufficient privileges for raw sockets, firewall/RST handling, TUN, pcap/Npcap and Wintun-class packet I/O. Android and unprivileged/no-root portability are out of scope.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004.

## Non-negotiable data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The required public carrier is udp2raw-compatible FakeTCP: TCP-shaped raw packets carrying unordered/real-time datagrams.
3. **TCP-shaped does not mean kernel-TCP-owned.** WBD does not require a real `ESTABLISHED` kernel TCP socket for the product lane.
4. Kernel TCP retransmission, ordered delivery, congestion-control HOL and byte-stream sequence ownership must not become dependencies of product payload delivery.
5. UDPspeeder mode 0 remains the fixed FEC reference: `20:10` for `weak-1.5x`, `20:20` for `weak-2x`.
6. Intentional source+repair bytes stay at or below 2.0x until benchmark evidence explicitly changes this constitution.
7. One raw lane is the product baseline. Extra lanes remain optional optimizations.

## Security invariants

1. The steady-state WBD security layer is **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable` commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. Server identity uses an operator-controlled X.509 certificate. Client validation uses native trust-chain + hostname verification; optional SPKI pinning is additive.
4. 0-RTT remains disabled until replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data. WBD does not invent a second AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams so one lost record does not block later records.
7. Optional post-Finished static-token authorization may be used for personal access control; a multi-user account platform is not a product goal.

## Optional TLS Persona

TLS Persona is an **optional connection-establishment feature**, not the VPN data cipher and not a correctness prerequisite.

Allowed goals include a real standard TLS 1.3 preflight to an operator-controlled endpoint, maintained browser-like ClientHello profiles (`native`, `chrome`, `firefox`, `safari`, `edge`), explicit certificate validation, and an optional short-lived bootstrap binding.

Rules:

- Persona must remain isolated from the unordered DTLS/FEC data plane.
- Persona must never silently disable DTLS verification.
- Browser-like fingerprints change connection appearance/interoperability; they do not make cryptography stronger.
- Standard TLS preflight and the FakeTCP/DTLS data lane are separate protocol roles.
- Do not borrow third-party private keys/certificates.
- Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Protection modes

- `normal`: one FakeTCP lane, no proactive FEC unless measurements justify it.
- `weak-1.5x`: UDPspeeder-compatible mode 0 `20:10`.
- `weak-2x`: UDPspeeder-compatible mode 0 `20:20`.
- `auto`: not admitted until fixed-mode measurements justify an adaptive policy.

The **current characterization campaign intentionally freezes FEC at `20:20`**. This is a test-space reduction, not removal of the other product modes.

## Product stack

```text
TUN / IP packet
        ↓
minimal WBD packet framing + session/control
        ↓
fixed-mode FEC
        ↓
DTLS 1.3
        ↓
udp2raw-compatible FakeTCP raw lane
        ↓
public network
```

For transport-only qualification, the TUN/platform layer may be replaced by a packet/datagram generator and echo sink. The qualified subject is then exactly `application datagrams → FEC → DTLS 1.3 → FakeTCP → network → reverse stack`.

## Qualification phases

### Phase A — transport-only characterization (CURRENT)

TUN is not required. Freeze UDPspeeder at mode 0 `20:20` and sweep network conditions first.

Required first matrix:

- RTT: `20, 50, 100, 200, 400, 600 ms`;
- symmetric independent random loss: `0, 1, 5, 10, 20, 30, 40%` per direction;
- at least three deterministic seeds;
- a fresh FakeTCP + DTLS + FEC association for every case;
- impairment active **before** FakeTCP/DTLS establishment.

Always record:

- connection/handshake success and time;
- delivery ratio and p50/p95/p99/max application RTT;
- throughput/goodput;
- later-datagram bypass / out-of-order evidence relevant to UDP-like no-HOL behavior;
- actual FakeTCP outer packet counts, TCP flags, RST/FIN counts, sequence/ACK progression and duplicates;
- per-component and total CPU time;
- CPU per delivered MiB;
- per-component and aggregate peak RSS;
- wire bytes and intentional FEC bytes separately.

`40%` loss is a cliff-finding stress point, not a release requirement.

### Phase B — platform qualification (DEFERRED FOR EXTERNAL HOST TESTING)

Real TUN/OpenWrt/Linux/Windows tests validate packet adapters, MTU, routing, driver behavior and platform resources. They must not retroactively redefine the transport semantics measured in Phase A.

## Interpretation of TCP-like and UDP-like

**TCP-like outer** means the public carrier remains the selected FakeTCP packet state/shape and stays free of pathological RST/control loops. It does not mean ordinary kernel-TCP reliability.

**UDP-like inner** means later independent application/FEC datagrams can make progress despite loss of an earlier datagram; the data plane does not acquire an ordered-byte-stream HOL dependency.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: **retired from the product roadmap after scope clarification**.
- Two raw lanes: optional optimization only.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.

## Development discipline

- Preserve already-qualified M1/M2/M3 evidence.
- Do not continue kernel-anchor work.
- Run the transport-only `20:20` matrix before expanding parameter dimensions.
- Do not add `20:10`, MTU, timer, burst-loss, Persona or multi-lane dimensions until the first matrix identifies where extra measurements are useful.
- TUN qualification may be handed to external real-device testers; do not block transport characterization on it.
- Optimize from measurements, not intuition.
- Every substantive stage ends with repository-backed tests and handoff.
