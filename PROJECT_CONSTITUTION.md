# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** whose public data plane is an unordered FakeTCP/FEC carrier and whose security/session layer is owned by WBD itself.

V1 (`dev/wbd-multilane-v1`, PR #2) is permanently rejected by M10-004. V2 may use privileged raw packet access, pcap/Npcap and FakeTCP-specific firewall/RST handling. Android and unprivileged mobile compatibility are explicitly out of scope.

**Xray, VLESS, Vision, REALITY and WireGuard are not part of the V2 product stack.** Their useful security properties are replaced by a standards-compliant WBD DTLS 1.3 session rather than by a nested proxy/VPN stack.

## Non-negotiable V2 invariants

1. Product payload/FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The reference public carrier is udp2raw-compatible FakeTCP: TCP-shaped raw packets with unordered/real-time payload delivery and no kernel TCP retransmission/HOL dependency for product data.
3. The first FEC reference is the pinned UDPspeeder implementation: `20:10` for 1.5x and `20:20` for 2.0x.
4. Total intentional protection is capped at 2.0x until benchmark evidence explicitly changes the constitution.
5. The native WBD security layer is **DTLS 1.3**, not a custom cipher and not an imitation of browser TLS. The first pinned implementation candidate is wolfSSL `v5.9.2-stable` at commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
6. Server identity uses a real X.509 certificate for a hostname controlled by the operator. The client verifies the trust chain and hostname with the DTLS library's native peer-verification path; optional SPKI pinning may be added. Do not borrow a third-party site's certificate or disable verification for the product path.
7. After DTLS Finished, product traffic remains standards-compliant DTLS application data for the lifetime of the association. There is no post-handshake switch to a home-grown encryption mode.
8. FEC is logically above DTLS encryption: UDPspeeder/source-repair datagrams are each sent as independent DTLS application datagrams. Thus every transmitted source/repair shard is independently AEAD-authenticated, while a lost shard does not block later DTLS records.
9. One raw lane is the required baseline. If two lanes are admitted later, they are distinct raw 4-tuples and distinct DTLS associations. FEC/source symbols are striped above the two associations and merged after DTLS verification; no ordered aggregate byte stream is created.
10. A kernel TCP anchor/dummy socket may be used for real OS handshake/control/RST behavior, but application payload is never written into that socket. Claims about kernel-generated ACKs covering raw-injected sequence space require packet-capture proof first.
11. Preserve the exact M10-004 no-go benchmark and PR #2. V2 must not reinterpret the V1 failure as a tuning problem.
12. WBD engineering may use standard protocol features for security and interoperability, but must not tune record sizes, timing, headers or fingerprints against a specific DPI/detector.

## Pinned initial upstream baseline

- `wangyu-/udp2raw` tag `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63`, MIT.
- `wangyu-/UDPspeeder` tag `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454`, MIT.
- `wolfSSL/wolfssl` tag `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`, DTLS 1.3 client/server candidate. Distribution/licensing obligations must be reviewed before packaging.

No floating upstream branch is authorized for qualification. Upgrade only in an isolated change with regression tests.

## Protection modes

- `normal`: one FakeTCP lane, no proactive FEC unless measurements justify a small baseline.
- `weak-1.5x`: UDPspeeder mode 0 `20:10` reference.
- `weak-2x`: UDPspeeder mode 0 `20:20` reference.
- `auto`: deferred until fixed one-lane/two-lane modes and DTLS overhead are qualified on real packet paths.

Intentional source/repair bytes are reported separately from raw IP/TCP-shaped and DTLS record overhead.

## Native WBD security/session boundary

Connection establishment is staged:

```text
FakeTCP raw lane established
        ↓
DTLS 1.3 handshake over the lane's datagram service
  real server certificate
  certificate chain + hostname verification
  ephemeral key exchange
  AEAD traffic keys
        ↓
DTLS Finished
        ↓
WBD/FEC application datagrams
```

Initial product policy:

- DTLS 1.3 only; no downgrade to DTLS 1.2 for the mainline.
- 0-RTT disabled until replay semantics are explicitly designed.
- Key update and session resumption are later milestones after the full handshake path is qualified.
- Optional username/password or account authorization, if needed, is carried only inside authenticated DTLS application data after Finished.
- Certificate issuance/provisioning may use normal CA/ACME tooling outside WBD; the protocol's responsibility is secure loading and peer validation.

## Platform path

Development order:

`Linux sandbox/privileged host → OpenWrt/Linux → Linux endpoint → Windows client`

Linux/OpenWrt is the preferred server side. Windows uses Npcap/easy-faketcp or another explicitly qualified packet-I/O path and later Wintun or equivalent for VPN ingress.

## Testing authority

GitHub Actions may fetch pinned dependencies, cross-build and relay temporary artifacts. **Local sandbox/host execution remains qualification authority.** An Actions-built binary is not qualified until the exact SHA-256 bytes are executed locally.

Persistent binaries live only in Google Drive. Git stores source, hashes, upstream identities, receipts and Drive IDs.

Required impairment progression:

1. clean 40–60 ms RTT / 0% loss;
2. ~50 ms RTT / 1% loss;
3. 80–150 ms RTT / ~2%;
4. 150–300 ms / 10% and 20%;
5. correlated burst loss;
6. 250–600 ms / ~30% or worse.

Always report p50/p95/p99, delivery/completion, intentional bytes, DTLS/FEC overhead, CPU/memory and whether impairment is independent or correlated across lanes.

## Development discipline

- First reproduce the pinned one-lane udp2raw + UDPspeeder baseline locally.
- Then qualify one-lane DTLS 1.3 with a real certificate before adding WBD account/session features.
- Test the kernel-anchor/real-return-packet hypothesis independently from DTLS/FEC tuning.
- Add two lanes only after the secured one-lane baseline is stable.
- Do not add Xray or WireGuard back as a dependency; build the minimal native WBD L3/TUN integration instead.
- One atomic main-path task at a time.
- Every substantive session ends with local tests and repository-backed handoff.
