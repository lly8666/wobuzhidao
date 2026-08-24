# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** whose public data carrier is unordered/datagram-like rather than an ordinary kernel TCP byte stream.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004 and remains historical evidence. V2 is allowed to use privileged raw packet access, pcap/Npcap and FakeTCP-specific firewall/RST handling. Android and unprivileged mobile compatibility are explicitly out of scope.

## Non-negotiable V2 invariants

1. Product payload/FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The reference public carrier is udp2raw-compatible FakeTCP: TCP-shaped packets with unordered/real-time payload delivery and no kernel TCP retransmission/HOL dependency for product data.
3. The first FEC reference is the pinned UDPspeeder implementation: `20:10` for 1.5x and `20:20` for 2.0x.
4. Total intentional protection is capped at 2.0x until benchmark evidence explicitly changes the constitution.
5. One lane is the required baseline. Two raw lanes are an optional enhancement and must beat one lane at the same total byte budget under independent, correlated and burst loss before becoming default.
6. If two lanes are used, FEC/source symbols are striped across distinct raw 4-tuples. Do not rebuild an ordered aggregate stream across lanes.
7. A kernel TCP anchor/dummy socket may be used for real OS handshake/control/RST behavior, but application payload is never written into that socket. Any claim that kernel-generated ACKs can safely cover raw-injected payload must be proven by packet/state tests first.
8. Linux/OpenWrt classic FakeTCP with privileged firewall/RST handling is the correctness fallback. Windows uses the upstream multiplatform/easy-faketcp/Npcap path first; Windows server mode is not assumed.
9. Stock Xray may be composed above/inside the V2 virtual link. It must not become the public outer ordinary-TCP carrier on the V2 mainline, because that would reintroduce the V1 HOL failure.
10. Preserve the exact M10-004 no-go benchmark and PR #2. Do not rewrite history to make V2 appear like an incremental success of V1.

## Pinned initial upstream baseline

- `wangyu-/udp2raw` tag `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63`, MIT.
- `wangyu-/UDPspeeder` tag `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454`, MIT.

No floating `main`/`unified` dependency is authorized for qualification. Upgrade only in an isolated change with full regression.

## Protection modes

V2 fixed modes are deliberately simple at first:

- `normal`: one FakeTCP lane, no proactive FEC unless measurements justify a small baseline.
- `weak-1.5x`: UDPspeeder mode 0 `20:10` reference.
- `weak-2x`: UDPspeeder mode 0 `20:20` reference.
- `auto`: not implemented until one-lane and two-lane fixed modes have real weak-network qualification.

The reference numbers are total intentional coded bytes relative to source payload. Raw/IP/TCP-shaped framing overhead is reported separately.

## Platform path

Development order is:

`Linux sandbox/privileged host → OpenWrt/Linux → Linux endpoint → Windows client`

Android is removed from the roadmap. Windows depends on Npcap/easy-faketcp or another explicitly qualified packet I/O path. OpenWrt/Linux is the preferred server side.

## Xray composition

The first product composition candidate is:

```text
application/TUN
  → stock Xray client (VLESS/Vision/REALITY)
  → private virtual IP path
  → WireGuard or equivalent minimal UDP/L3 glue
  → FEC
  → udp2raw FakeTCP lane(s)
  → public network
```

and the reverse stack at the server. Xray semantics remain stock, but the lower unordered/FEC path handles public weak-network loss before the inner Xray TCP stream observes it.

A public-outer `REALITY/Vision → ordinary TCP` experiment is not the V2 mainline and must be treated as a regression/control because it recreates the rejected carrier condition.

## Testing authority

GitHub Actions may fetch pinned dependencies, cross-build and relay temporary artifacts. **Local sandbox/host execution remains qualification authority.** A binary from Actions is not qualified until the exact SHA-256 bytes are tested locally.

Persistent binaries live only in Google Drive. Git stores source, hashes, upstream identities, receipts and Drive IDs.

Required benchmark progression:

1. clean 40–60 ms RTT / 0% loss;
2. ~50 ms RTT / 1% loss;
3. 80–150 ms RTT / ~2%;
4. 150–300 ms / 10% and 20%;
5. correlated burst loss;
6. 250–600 ms / ~30% or worse.

Always report p50/p95/p99, delivery/completion, intentional bytes, CPU/memory and whether impairment is independent or correlated across lanes.

## Engineering boundary

Development focuses on correctness, weak-network latency, FEC, packet-state consistency, routing, portability between the scoped platforms and interoperability. Do not tune packet timing, headers or lane behavior against a specific DPI/detector.

## Development discipline

- First reproduce the pinned one-lane udp2raw + UDPspeeder baseline locally.
- Then test the kernel-anchor/real-return-packet hypothesis independently from FEC.
- Add two lanes only after the one-lane baseline is stable.
- Reuse WireGuard/Xray before writing a custom TUN or VPN stack.
- One atomic main-path task at a time.
- Every substantive session ends with local tests and repository-backed handoff.
