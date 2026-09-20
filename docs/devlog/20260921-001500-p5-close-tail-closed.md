# P5 close-tail atom closed

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f`
- GitHub Actions: `35520092389`
- Result: **8/8 PASS**
- Previous measurement-base qualification: `043da6298a2be091b55fb470b63c69e605bb61d8` / `35516700104`
- This closure commit is docs-only and must not replace the product qualification SHA.

## Closed atom

The second P5 atom is closed: sequential inner HTTPS close-tail / concurrent-retire correctness over the existing formal runtime path.

The qualified sequence is now:

1. establish one outer FakeTCP + TLS/protected-admission Normal lane;
2. create inner business FlowID 1;
3. perform a real TLS handshake and HTTP request/response;
4. perform normal HTTP `Connection: close` and TLS close;
5. wait until both client and server platformflow TCP flow registries reach zero;
6. tolerate bounded legitimate late tail frames for the retired FlowID without terminating the tunnel runtime;
7. create distinct inner business FlowID 2;
8. perform a second real HTTPS request/response over the same outer lane;
9. close FlowID 2 normally.

The outer client initial SYN count remains exactly one and the outer lane ref remains stable.

## Lifecycle change

The active implementation keeps the close-tail change narrow:

- a retired FlowID tombstone is bounded to 5 seconds and at most 4096 entries;
- late Data/ACK/Close for a recently retired ID are terminal-idempotent;
- a late duplicate server TCPOpen for a retired ID does not redial the upstream;
- unknown never-seen or expired IDs still fail closed;
- handlers that already obtained a flow pointer before another goroutine retires it treat the now-closed flow as terminal-idempotent;
- local business-socket write / CloseWrite failures abort only that business flow;
- a service ACK send error is suppressed only when the flow is confirmed retired/closed; genuine active-flow tunnel errors still propagate.

No RTO, retransmit count, send window, FEC, padding, lane count, or transport topology is changed.

## Exact-SHA evidence

Actions `35520092389` completed successfully with all eight jobs PASS:

- repository contract;
- Windows active Go tests;
- Ubuntu active Go tests, Linux race, tlsrecord directed fuzz/reference;
- P2 kernel fallback;
- Linux shared-TUN privileged iptables;
- Linux shared-TUN privileged nft;
- OpenWrt privileged TPROXY/SocketTunnel;
- strengthened P5 HTTPS measurement gate.

The P5 harness emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f flows=2 outer_connections=1 sequential_close=pass fec=off padding=off records=42`

The independent validator emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha=a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f flows=2 outer_connections=1 sequential_close=pass fec=off padding=off`

P5 artifact:

- ID `10607729016`
- size `21587` bytes
- digest `sha256:da4f511bd95c31ffc897e4a407ec73d0dfb9c58e58964ec0ab5f02c05e82f590`
- uploaded raw PCAP / JSONL events / manifest / summary / test log.

## Preserved failure evidence

The first close-tail candidate remains preserved:

- SOURCE_SHA `6533234b6ab0ca18e89da3ca7dd682263423b9b1`
- Actions `35519764312`
- result: 6/8 PASS, strengthened P5 gate FAIL
- artifact `10607029636`
- digest `sha256:d8fef2c69647a09ba2ad8810df32e6e3c49c1784207060de780d24856338e1a2`.

It proved that lookup-miss tombstones alone were insufficient because a handler could already hold a flow pointer when concurrent retirement happened.

## Non-claims

- No weak-network, load, or soak result.
- No resumed TLS handshake result yet.
- No different-certificate-chain matrix yet.
- No FEC-on measurement yet.
- No classifier or indistinguishability claim.
- Production padding remains `0/off`.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 TPROXY/capture remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No archive implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Add only controlled real-HTTPS **full vs resumed TLS handshake** measurement:

- keep the same formal runtimeentry/Tunnel/lane path;
- preserve exact raw capture/event schema and source provenance;
- explicitly record whether the application TLS connection resumed and the session-cache provenance;
- keep FEC off, padding 0/off, and network injection disabled;
- make no performance conclusion from this atom;
- do not mix in different certificate chains, weak-network matrices, load, or classifier work.
