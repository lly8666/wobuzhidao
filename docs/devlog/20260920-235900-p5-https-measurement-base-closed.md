# P5 HTTPS measurement base closed

- Date: 2026-09-20
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `043da6298a2be091b55fb470b63c69e605bb61d8`
- GitHub Actions: `35516700104`
- Result: **8/8 PASS**
- Product qualification authority: exact SOURCE_SHA above
- This closure commit is docs-only and must not replace `last_tested_source_sha`.

## Closed atom

The first P5 atom is now closed: a minimum reproducible controlled real-HTTPS hosted measurement base over the current formal runtime path.

The qualified path is:

`real TLS+HTTP client -> platformflow TCP -> leased TunnelOwner / Normal lane -> runtimeentry steady transport -> same outer FakeTCP association -> server platformflow TCP -> real TLS HTTP server`.

Two independent inner HTTPS connections are measured:

1. first HTTPS flow on the initial established outer connection;
2. subsequent independent HTTPS flow on the already-established lane.

The outer lane reference remains stable and the harness observes exactly one client initial FakeTCP SYN across the two business flows.

## Evidence produced

The dedicated P5 gate writes and independently validates:

- `outer.pcap` — DLT_RAW IPv4/TCP serialized at the hosted SegmentIO emit boundary;
- `events.jsonl` — outer-packet, steady TLS-like-record, and HTTPS-flow events;
- `manifest.json` — exact SOURCE_SHA/harness SHA, scenario/config, runner/toolchain, capture accounting, transport/owner inventory;
- `summary.json` — independent validator result;
- `test.log` — raw harness output.

The exact-SHA run emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=043da6298a2be091b55fb470b63c69e605bb61d8 flows=2 outer_connections=1 fec=off padding=off records=30`

and the validator emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha=043da6298a2be091b55fb470b63c69e605bb61d8 flows=2 outer_connections=1 fec=off padding=off`.

P5 artifact:

- ID `10606459773`
- size `19031` bytes
- digest `sha256:8eda8c0f3ec2df0bf08afd1d771c251a2800a6fcd881df0c14a73a67c84fb66f`
- five uploaded files: PCAP, JSONL events, manifest, summary, test log.

The same exact SHA also passed repository-contract, Windows active Go tests, Ubuntu active Go tests and race, tlsrecord directed fuzz/reference, P2 kernel fallback, Linux shared-TUN privileged iptables/nft, and OpenWrt privileged TPROXY/SocketTunnel gates.

## Preserved failure evidence and scope boundary

Two failed candidates remain part of the record:

- `bd96f618b001139dbd971e46d7bacc6ba6dbae2d` / Actions `35516224912`: initial `net.Pipe` fixture did not provide the TCP half-close semantics needed by platformflow.
- `74f1b602bc664446f34d5a8c42e5a4d21391439e` / Actions `35516466731`: with a real loopback TCP fixture, normal first-flow `Connection: close` exposed a late-frame lifecycle issue; after the server flow had been retired a tail frame could produce `platformflow: malformed frame: unknown TCP server flow` and terminate runtime.

The final measurement-base qualification deliberately does not pretend the second issue is fixed. Both inner HTTPS connections remain alive until both measurement events have been captured. Therefore this atom qualifies “first vs subsequent independent HTTPS flow on one existing outer lane”, but **not** “fully close first flow, then open second flow”.

That close-tail behavior becomes the next P5 atomic task. It should be fixed as terminal-frame/idempotent lifecycle correctness, not as a rewrite of TCP reliability.

## Non-claims

- No weak-network, load, or soak conclusion.
- No different-certificate-chain or resumed-handshake result yet.
- No FEC-on measurement yet.
- No classifier or indistinguishability result.
- Production padding remains `0/off`.
- Hosted SegmentIO PCAP is not a physical NIC capture.
- Windows/Npcap real driver/admin physical-NIC qualification remains P7 `NOT_RUN`.
- OpenWrt IPv6 TPROXY/capture remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No archive implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Fix only the platformflow TCP close-tail lifecycle needed for sequential HTTPS measurement:

- after a normal first HTTPS flow closes, legitimate late terminal/ACK frames for the retired FlowID must be harmless and bounded;
- they must not terminate the tunnel/runtime;
- a second real HTTPS flow must then open over the same already-established outer lane and complete;
- add targeted unit/integration evidence and rerun the full exact-SHA `next-foundation` workflow;
- do not change FEC, padding, RTO, weak-network parameters, or transport topology.
