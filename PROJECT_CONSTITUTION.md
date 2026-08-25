# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- a TCP-shaped raw/FakeTCP public carrier;
- UDP/datagram-like payload semantics with no ordinary-TCP retransmission/HOL dependency;
- fixed-mode FEC for weak-network recovery;
- standards-compliant DTLS 1.3 for data-plane authentication, encryption, integrity and anti-replay;
- native TUN/L3 ingress/egress;
- an optional TLS Persona bootstrap with browser-like ClientHello profiles.

The endpoints are operator-controlled devices with sufficient privileges for raw sockets, firewall/RST handling, TUN, pcap/Npcap and Wintun-class packet I/O. Android and unprivileged/no-root portability are out of scope.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004.

## Non-negotiable data-plane invariants

1. Product IP packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The required public carrier is udp2raw-compatible FakeTCP: TCP-shaped raw packets carrying unordered/real-time datagrams.
3. **TCP-shaped does not mean kernel-TCP-owned.** WBD does not require a real `ESTABLISHED` kernel TCP socket for the product lane.
4. Kernel TCP retransmission, ordered delivery, congestion-control HOL and byte-stream sequence ownership must not become dependencies of product payload delivery.
5. The first FEC reference remains pinned UDPspeeder mode 0: `20:10` for `weak-1.5x`, `20:20` for `weak-2x`.
6. Total intentional source+repair bytes stay at or below 2.0x until benchmark evidence explicitly changes this constitution.
7. One raw lane is the required product baseline. Additional lanes are optional optimizations only after same-total-byte-budget benchmarks justify them.

## Security invariants

1. The WBD data-plane security layer is **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable` commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. Server identity uses an operator-controlled X.509 certificate. Client validation uses the DTLS implementation's native trust-chain + hostname verification path; optional SPKI pinning is additive.
4. 0-RTT remains disabled until application replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data. WBD does not invent a second AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams so one lost record does not block later records.
7. Optional post-Finished static-token authorization may be used for personal access control; a multi-user account platform is not a product goal.

## Optional TLS Persona

TLS Persona is an **optional connection-establishment feature**, not the VPN data cipher and not a prerequisite for correctness.

Allowed goals:

- use a real standard TLS 1.3 preflight/bootstrap connection to an operator-controlled endpoint;
- select a browser-like ClientHello profile using a maintained uTLS-style implementation;
- support profiles such as `native`, `chrome`, `firefox`, `safari`, `edge` and an explicitly tested randomized profile;
- carry a short-lived bootstrap/session-binding token that can be presented inside the later authenticated WBD session;
- record ClientHello size, fragmentation and handshake latency during qualification.

Rules:

- Persona must remain isolated from the unordered DTLS/FEC data plane.
- A Persona failure may fail closed or fall back only according to explicit configuration; it must never silently disable DTLS verification.
- Browser-like fingerprints are an appearance/interoperability option. They do **not** replace DTLS encryption or make cryptography stronger.
- Do not switch from a completed ordinary TLS byte stream into FakeTCP payload on the same TCP stream. Standard TLS preflight and the FakeTCP/DTLS data lane are separate protocol roles.
- Do not borrow third-party private keys/certificates. The default deployment uses an operator-controlled hostname/certificate.
- Xray/REALITY/Vision may be studied as implementation references, but WBD does not import VLESS, Xray routing, or Vision stream semantics into the data plane.

## Protection modes

- `normal`: one FakeTCP lane, no proactive FEC unless measurements justify a small baseline.
- `weak-1.5x`: UDPspeeder-compatible mode 0 `20:10`.
- `weak-2x`: UDPspeeder-compatible mode 0 `20:20`.
- `auto`: not admitted until fixed-mode large-scale measurements justify an adaptive policy.

Intentional source/repair bytes must be reported separately from DTLS and raw IP/TCP-shaped overhead.

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

Optional connection bootstrap:

```text
standard TCP/TLS 1.3 preflight
  + optional browser-like ClientHello profile
  + operator-controlled certificate
  + short-lived WBD bootstrap binding
        ↓
admit normal WBD FakeTCP/DTLS data lane
```

The bootstrap does not carry the steady-state VPN data path.

## Platform path

Development order:

`Linux test harness → OpenWrt/Linux TUN path → Linux endpoint → Windows/Npcap + Wintun`

Linux/OpenWrt is the preferred server side. Windows is a required client target.

## Retired / optional research

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes: permanently rejected.
- Kernel TCP anchor / real-return-packet hybrid: **retired from the product roadmap after scope clarification**. Existing packet-capture evidence may remain as historical research; no additional product work is required.
- Two raw lanes: optional optimization only.
- Xray/VLESS/Vision/REALITY as a nested product stack: not used. Only isolated TLS Persona ideas/code patterns may be referenced.
- WireGuard inner glue: not used.
- Android/no-root: out of scope.

## Testing authority

GitHub Actions may build and run deterministic unit/integration tests. Local privileged Linux/OpenWrt/Windows execution remains authority for raw packet, TUN, DTLS and impairment qualification.

Required impairment families for final tuning:

1. clean 40–60 ms RTT / 0% loss;
2. ~50 ms / 1–2% loss;
3. 80–150 ms / ~2–5% loss;
4. 150–300 ms / 10% and 20% loss;
5. correlated/burst loss;
6. 250–600 ms / ~30% loss;
7. MTU/fragmentation sweeps;
8. long-duration reconnect/resource tests.

Always report p50/p95/p99/max, delivery/completion, intentional bytes, DTLS/FEC overhead, CPU/RAM, packet loss model, and configuration.

For TLS Persona, additionally report ClientHello bytes/segments, handshake p50/p95/p99, failure rate, negotiated TLS/ALPN, and certificate validation outcome.

## Development discipline

- Preserve already-qualified M1/M2/M3 evidence.
- Do not continue kernel-anchor work.
- Build the smallest complete one-lane Linux/OpenWrt TUN path next.
- Add Windows interoperability before optional multi-lane work.
- Keep TLS Persona modular and independently disableable.
- Run broad parameter sweeps only after the end-to-end core path works; optimize from measurements, not intuition.
- One atomic main-path task at a time.
- Every substantive session ends with tests and repository-backed handoff.
