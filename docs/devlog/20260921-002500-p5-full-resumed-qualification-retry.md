# P5 full/resumed qualification retry

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Failed candidate SOURCE_SHA: `82b391408cb61703e69dc4ed1db3eae9d8671004`
- Failed Actions: `35522085204`
- Current product qualification remains: `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f`
- Retry candidate SOURCE_SHA: pending this docs-only evidence commit
- P5 harness/product implementation change in retry: **none**

## First full/resumed candidate result

Actions `35522085204` completed 7/8 PASS.

Passed:

- repository contract;
- strengthened P5 full vs resumed handshake gate;
- Windows active Go tests;
- Ubuntu unit/build, Linux race, directed tlsrecord fuzz/reference;
- OpenWrt privileged gate;
- Linux shared-TUN privileged iptables;
- Linux shared-TUN privileged nft.

The P5 gate independently validated:

`WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha=82b391408cb61703e69dc4ed1db3eae9d8671004 flows=2 outer_connections=1 sequential_close=pass handshakes=full,resumed fec=off padding=off`

P5 artifact:

- ID `10609066398`
- size `21065` bytes
- digest `sha256:441630efa8e5c0b9c1a86032de21c2acc34f7c6597631f9b121f2b7c90847623`.

This establishes that the new handshake measurement logic itself ran successfully, but it is not product qualification because the full workflow was not green.

## Only failed job: existing P2 privileged fallback

`p2-kernel-fallback` failed inside the existing
`TestKernelTLSFallbackVerifiedHTTPAndNormalClose` after approximately 10 seconds:

`kernel_fallback_linux_test.go:318: unexpected EOF`

P2 failure artifact:

- ID `10608672257`
- size `5072` bytes
- digest `sha256:16bfe37f4bd696e51d83cfee9c929548948ea9f24f562def7a981869e8e891da`.

The candidate diff does not modify P2 code, its workflow step, FakeTCP fallback behavior, or network setup. The changed implementation surface is limited to the P5 measurement test, P5 validator, and governance docs.

Therefore this atom does not make an unrelated P2 code change from a single privileged runtime EOF. The failure is preserved and the complete qualification is retried on a new exact SHA. If P2 reproduces independently again, it will be investigated from both runs before changing scope.

## Retry rule

The retry candidate must pass all eight jobs on the same exact SHA. The P5 gate must again prove:

- TLS 1.3 full handshake on flow 1;
- a real session-ticket cache Put;
- TLS 1.3 resumed handshake on flow 2;
- a real cache Hit;
- first flow fully closes before flow 2;
- exactly one outer SYN and stable outer lane;
- FEC off, production padding off, no network injection.

`last_tested_source_sha` remains `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f` until a new all-green exact SHA exists.
