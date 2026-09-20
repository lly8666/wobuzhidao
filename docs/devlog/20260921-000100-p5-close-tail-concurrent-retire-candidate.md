# P5 close-tail concurrent-retire candidate

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Parent SOURCE_SHA: `6533234b6ab0ca18e89da3ca7dd682263423b9b1`
- Failed Actions preserved: `35519764312`
- Current product qualification remains: `043da6298a2be091b55fb470b63c69e605bb61d8`
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## First close-tail candidate result

The first close-tail candidate added a bounded retired-FlowID tombstone and tightened the P5 gate to require a complete close of flow 1 before flow 2.

Actions `35519764312` completed with 6/8 jobs PASS.

Passed:
- repository contract;
- Windows active Go tests;
- Ubuntu normal unit/build, including `internal/platformflow`;
- P2 kernel fallback;
- Linux shared-TUN privileged iptables;
- Linux shared-TUN privileged nft;
- OpenWrt privileged TPROXY/SocketTunnel.

Failed:
- strengthened P5 HTTPS gate;
- Ubuntu race, separately in the unchanged existing P4 `TestLifecycleEntryGameThreeAndFourLaneMatrix/lanes-3` timing assertion. This atom does not modify P4.

The P5 artifact `10607029636` has digest
`sha256:d8fef2c69647a09ba2ad8810df32e6e3c49c1784207060de780d24856338e1a2`.

Its raw events prove flow 1 completed successfully and was recorded as:

- `business_flow_id=1`
- `flow_closed=true`
- HTTP 200
- real TLS handshake/business timing and wire bytes present.

The test then failed while reading the second HTTPS response with `connection reset by peer`. After the first flow event, raw outer events continued at roughly the existing 500ms retransmission cadence. This means the lookup-miss tombstone fixed one path but not every concurrent close path.

## Narrow second fix

There is a race window where a service handler can obtain an active flow pointer, another goroutine retires/closes that flow, and the first handler then observes a closed flow or tries to send an ACK through an already-closed `TunnelFlow`.

The second candidate changes only per-flow lifecycle error containment:

- `handleAck` / `handleData` return success when the already-obtained flow pointer is now closed; this is the same terminal idempotence as the tombstone lookup path.
- If delivery or half-close into the local business socket fails because that individual connection has closed, abort only that business flow and do not terminate the outer tunnel runtime.
- If sending the service ACK fails, suppress the error only when the flow is now confirmed retired/closed.
- Genuine send errors while the flow is still active continue to propagate unchanged.
- The 5s / 4096 retired-FlowID tombstone bounds remain unchanged.
- RTO, retransmit count, send window, FEC, padding, lane count and topology remain unchanged.

The focused platformflow regression is extended to exercise ACK/Data handlers holding an already-closed flow pointer.

The P5 harness also checks the client runtime error channel after each fully-closed HTTPS flow so any remaining tunnel-level propagation is reported directly rather than only surfacing later as an inner TCP reset.

## Qualification boundary

No local result is qualification authority. This candidate must pass the full exact-SHA `next-foundation` workflow, including the strengthened P5 gate with:

- two distinct inner business FlowIDs;
- flow 1 closed on both client and server before flow 2 opens;
- `sequential_close_before_next=true`;
- exactly one outer client initial SYN;
- stable outer lane ref;
- validator marker `sequential_close=pass`.

No `old/` implementation is reused, so `docs/REUSE_LEDGER.json` is unchanged.

Windows/Npcap physical remains P7 `NOT_RUN`; OpenWrt IPv6 remains `NOT_IMPLEMENTED`; production padding remains 0/off.
