# P5 full/resumed handshake atom closed

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `1fb65c90b272454e09a624eaa23e6a432de7955a`
- GitHub Actions: `35522367146`
- Result: **8/8 PASS**
- Previous product qualification: `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f`
- This closure commit is docs-only and must not replace the product qualification SHA.

## Closed atom

The third P5 atom is closed: controlled real inner-HTTPS TLS 1.3 full versus resumed handshake measurement over the current formal runtime path.

The qualified sequence remains one established outer FakeTCP + protected TLS/admission Normal lane with two distinct sequential inner business TCP flows. Flow 1 fully closes before flow 2 starts, and both client/server platformflow registries return to zero between them.

Handshake requirements are now explicit:

1. flow 1 uses TLS 1.3 and `DidResume=false`;
2. the client receives/stores real session state in an explicit shared LRU client-session cache;
3. flow 2 uses TLS 1.3 and `DidResume=true`;
4. flow 2 performs a real lookup hit in that same cache;
5. the outer client initial SYN count remains exactly one and the outer lane ref remains stable.

The cache is part of the controlled inner HTTPS client only. This evidence does not relabel or change the WBD outer protected-admission TLS session.

## Measurement evidence

Each HTTPS flow event now records:

- TLS version;
- negotiated cipher suite;
- handshake mode (`full` or `resumed`);
- explicit resumed boolean;
- per-flow session-cache Get/Hit/Put deltas;
- existing handshake timing, business latency, wire bytes, response status, distinct business FlowID, and close state.

The manifest records the TLS1.3 requirement, expected `[full,resumed]` modes, shared-LRU cache identity/capacity, and final cache counters.

The exact-SHA P5 harness emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=1fb65c90b272454e09a624eaa23e6a432de7955a flows=2 outer_connections=1 sequential_close=pass handshakes=full,resumed fec=off padding=off records=41`

The independent validator emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha=1fb65c90b272454e09a624eaa23e6a432de7955a flows=2 outer_connections=1 sequential_close=pass handshakes=full,resumed fec=off padding=off`

P5 artifact:

- ID `10609266231`
- size `21045` bytes
- digest `sha256:866acf9477379ec69f8f60c073b00d41d3efab5b783a1e000dd63a58343c18eb`
- raw PCAP / JSONL events / manifest / summary / test log uploaded.

## Preserved failed candidates

The atom retains both failed exact-SHA runs:

- `82b391408cb61703e69dc4ed1db3eae9d8671004` / Actions `35522085204`: the new P5 full/resumed gate itself passed, but an unrelated existing P2 privileged fallback run ended with `unexpected EOF`. P2 later passed unchanged.
- `e0650911ed2268c5b2f0744006016de924ea462d` / Actions `35522232171`: P2 and all other gates passed, while P5 failed before business traffic because the harness performed a one-shot read of server admission/service readiness.

The final fix was only a condition-driven readiness wait for the authoritative server tunnel/service, with a 2s deadline and immediate return when ready. It executes before `recorder.markSteady()`, so it is outside the measured business timing interval. It is not a fixed traffic-pacing sleep.

## Same-SHA regression authority

Actions `35522367146` passed all eight jobs on the exact product SHA:

- repository contract;
- P5 real-HTTPS measurement gate;
- Windows active Go tests;
- Ubuntu unit/build, Linux race and directed fuzz/reference;
- P2 kernel fallback;
- OpenWrt privileged TPROXY/SocketTunnel;
- Linux shared-TUN privileged iptables;
- Linux shared-TUN privileged nft.

## Non-claims

- No different-certificate-chain qualification yet.
- No weak-network, load, or soak result.
- No sparse/natural-concurrency matrix yet.
- No FEC-on measurement yet.
- No classifier or indistinguishability claim.
- Production padding remains `0/off`.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 TPROXY/capture remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No `old/` implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Cover only the controlled **different certificate chain** dimension:

- use at least two independent controlled certificate trust chains;
- require real certificate verification for each scenario;
- record certificate/chain scenario identity in raw event and manifest provenance;
- keep TLS1.3, FEC off, production padding off, and no network injection;
- do not mix in load, concurrency, FEC-on, weak-network matrices, classifier work, or performance conclusions.
