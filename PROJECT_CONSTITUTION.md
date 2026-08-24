# wobuzhidao Project Constitution

## Product goal

Build a personal-use VPN transport whose **logical delivery behavior approaches UDP/QUIC under weak networks while every public carrier remains an ordinary kernel TCP connection protected by stock VLESS + XTLS Vision + REALITY**.

The system is not a raw-FakeTCP product. Raw packet experiments may exist under an explicitly experimental track, but the product path must work without client-side raw sockets or protocol-specific firewall/RST rules and must remain viable for a future unrooted Android `VpnService` client.

## Non-negotiable network invariants

1. Every public carrier is real kernel TCP. WBD does not forge TCP sequence/ACK state on the product path.
2. Each public carrier uses stock VLESS + `xtls-rprx-vision` + REALITY + RAW transport unless an explicit architecture decision supersedes it.
3. WBD adds no new public plaintext handshake/header, custom TCP option, auxiliary UDP path, or extra public protocol port. WBD metadata lives inside its protected inner session.
4. One WBD logical session is not one TCP connection. Independent TCP lanes localize head-of-line blocking.
5. Reliable stream identity is independent from lane/TCP identity. Cross-lane reinjection may deliver the same logical bytes before the original TCP lane recovers; duplicates are discarded by logical identity.
6. WBD datagrams preserve datagram semantics. Expired datagrams are not made reliable merely because carriers are TCP.
7. TCP VPN traffic must ultimately be split/terminated at the VPN/proxy boundary rather than tunneled as TCP packets inside TCP carriers.
8. Redundancy is controlled by a shared Redundancy Budget Controller (RBC), not separate incompatible protocols for 1.5x/2x/Auto.
9. Active protection is bounded at 2.0x logical payload. If 2.0x cannot meet the target, report degraded state instead of unbounded replication.
10. Scheduling/backpressure decisions happen before large queues are committed to kernel TCP sockets.

## Protection modes

User-facing modes are `normal`, `weak-1.5x`, `weak-2x`, and `auto`.

- `normal`: no proactive redundancy; gap-driven hedge/reinjection is allowed.
- `weak-1.5x`: target 50% proactive protection budget, shared by repair/FEC and reinjection.
- `weak-2x`: V1 baseline is full cross-lane replication; later implementations may spend the 100% protection budget more efficiently without changing wire semantics.
- `auto`: discrete protection levels from 1.0x to 2.0x, fast-up/slow-down, driven primarily by logical delivery lateness/gaps rather than raw packet-loss counters.

Intentional WBD multiplier excludes unavoidable TCP retransmission and framing overhead; reports must distinguish intentional overhead from observed interface/network bytes.

## Platform path

Development order is Linux sandbox -> Linux host -> OpenWrt -> Windows -> Android. Android is intentionally last; most protocol, scheduler, FEC, session and benchmark work must be proven in the sandbox first.

The eventual Android client must not require root, `CAP_NET_RAW`, raw sockets, or FakeTCP-specific firewall changes. Normal `VpnService` authorization and normal server/cloud firewall port exposure are outside that prohibition.

## Testing authority

GitHub Actions may fetch public dependencies, cross-compile, and provide temporary build artifacts. **Local sandbox testing is the qualification authority.** A binary built by Actions is not qualified until the exact SHA-256 bytes are tested locally.

Persistent binaries live in Google Drive. Git stores only source, manifests, exact Drive file IDs, hashes, sizes and test receipts.

## Development discipline

- Benchmark against native TCP, UDP and QUIC oracles; do not call the system “UDP-like” from intuition.
- Prefer the smallest mechanism that closes a measured gap: multi-lane -> logical ACK/GAP reinjection -> rescue lane -> RBC -> simple FEC -> sliding-window FEC only if admitted by data.
- Do not modify REALITY/Vision merely to make WBD convenient.
- One active main-path atomic task at a time.
- Every substantive session ends with local tests and repository-backed handoff.
