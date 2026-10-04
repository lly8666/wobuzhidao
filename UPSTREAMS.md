# Upstream policy

Exact upstream commits are pinned before code starts depending on them.

Initial reference set to refresh/pin in milestone M1/M14 work:

- `XTLS/Xray-core` — product carrier and later TUN integration.
- `XTLS/REALITY` / Xray-pinned REALITY module — informational source for carrier behavior; do not fork by default.
- QUIC RFCs / selected Go implementation — benchmark oracle, not public product carrier.
- `wangyu-/udp2raw` — historical/raw-FakeTCP reference and external benchmark comparator only; not the product transport.
- `wangyu-/UDPspeeder` — external FEC benchmark comparator only; not WBD wire semantics.

Rules:

1. Never `go get latest` as an incidental fix.
2. Record repository, commit/tag, purpose and license in a machine-readable lock before implementation depends on it.
3. Upstream upgrades are dedicated workstreams with local regression.
4. Stock REALITY/Vision behavior is an architecture invariant unless an explicit ADR, tests and benchmark evidence justify a change.
5. Raw/FEC comparison tools never redefine the product boundary: WBD public carriers remain real kernel TCP with stock REALITY/Vision.

## Benchmark oracles

- QUIC oracle: `github.com/quic-go/quic-go` tag `v0.61.0` (`579ee19`), recorded in `deps/oracle-lock.json`. It requires Go 1.25.0, so it is not linked into the current Go 1.23 WBD module in M10.
- Raw/FEC external comparator: udp2raw tag `20230206.0` at `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63` plus UDPspeeder tag `20230206.0` at `61b24a369700c3d8248dd18fa9a524b778741454`. Primary comparison uses UDPspeeder mode 0, `20:20`, timeout 8 ms (about 2.0x); `20:10` (about 1.5x) is retained as a secondary sanity point. Exact binaries must be SHA-qualified locally before results count.
