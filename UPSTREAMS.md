# Upstream policy

Exact upstream commits are pinned before code starts depending on them.

Initial reference set to refresh/pin in milestone M1/M14 work:

- `XTLS/Xray-core` — product carrier and later TUN integration.
- `XTLS/REALITY` / Xray-pinned REALITY module — informational source for carrier behavior; do not fork by default.
- QUIC RFCs / selected Go implementation — benchmark oracle, not public product carrier.
- `wangyu-/udp2raw` — historical/raw-FakeTCP reference only; not the product transport.

Rules:

1. Never `go get latest` as an incidental fix.
2. Record repository, commit/tag, purpose and license in a machine-readable lock before implementation depends on it.
3. Upstream upgrades are dedicated workstreams with local regression.
4. Stock REALITY/Vision behavior is an architecture invariant unless an explicit ADR, tests and benchmark evidence justify a change.

## Benchmark oracle

- QUIC oracle plan: `github.com/quic-go/quic-go` tag `v0.61.0` (`579ee19`), recorded in `deps/oracle-lock.json`. It requires Go 1.25.0, so it is not linked into the current Go 1.23 WBD module in M10.
