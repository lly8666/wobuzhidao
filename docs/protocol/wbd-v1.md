# WBD v1 protocol seed

M1 defines the minimal four-frame wire vocabulary. M2 adds deterministic in-memory receive semantics without introducing any carrier, FEC, Xray or TUN dependency.

## Envelope

```text
version  1 byte
frame    1 byte
flags    1 byte
bodyLen  unsigned varint
body     bodyLen bytes
```

All integers are unsigned and limited to 62 bits. Frames are carried inside the future protected WBD inner session; this framing is not a public plaintext network layer.

## Identity rule

Logical application identity and transport identity are distinct:

```text
logical stream item = (flow_id, stream_offset, payload)
logical datagram    = (flow_id, datagram_id, payload)
transmission attempt = transmission_id
lane/carrier context = lane_id (connection state, intentionally not repeated in DATA/DATAGRAM wire bodies)
```

Cross-lane reinjection keeps logical identity and allocates a new transmission ID. The receiving logical layer deduplicates by logical identity, not by TCP sequence or lane.

## DATA

Body:

```text
flow_id          uvarint
stream_offset    uvarint
transmission_id  uvarint
payload_len      uvarint
payload          bytes
```

Envelope `FIN` marks the final logical stream extent at `stream_offset + payload_len`. M2 requires every FIN for a flow to agree on exactly one final offset; data beyond that offset is rejected. A FIN-only frame is valid and can close an empty stream.

## DATAGRAM

Body:

```text
flow_id          uvarint
datagram_id      uvarint
transmission_id  uvarint
payload_len      uvarint
payload          bytes
```

M2 treats datagrams as expiring logical messages. A hard deadline is supplied to the receive engine as deterministic session metadata; it is intentionally not added to the v1 DATAGRAM wire body yet. The later session-control design will decide whether lifetime is negotiated per flow or signalled per datagram.

## ACK

ACK is logical, not TCP ACK. It acknowledges received logical ranges for one flow.

```text
flow_id      uvarint
kind         byte (STREAM or DATAGRAM)
range_count  uvarint
range[]:
  start      uvarint
  end        uvarint   # half-open [start,end)
```

Ranges must be ordered, non-overlapping and non-empty.

## GAP_HINT

A receiver may report a logical hole without waiting for the original carrier TCP to finish recovery.

```text
flow_id  uvarint
kind     byte
start    uvarint
end      uvarint
```

This frame does not itself define a retransmission algorithm. M5/M6 will use it for cross-lane reinjection.

## M2 receive semantics

### STREAM

- Logical identity is `(flow_id, stream_offset, bytes)`; `transmission_id` does not affect deduplication.
- Out-of-order bytes are buffered until the contiguous prefix starting at `next_offset` is available.
- Exact/reinjected duplicates are ignored. Buffered overlapping bytes must agree; conflicting overlaps fail closed.
- Partial duplicates may contribute only the not-yet-delivered suffix.
- FIN fixes one final offset. Conflicting FINs or data beyond the final offset fail closed.
- A rejected frame is transactional: it cannot mutate buffered data or the final offset.
- Each flow has a bounded out-of-order byte budget; M2 defaults to 8 MiB. Session-wide flow control remains later work.

### DATAGRAM

- Logical identity is `(flow_id, datagram_id)`; `transmission_id` does not affect deduplication.
- The first valid copy before its hard deadline is delivered immediately.
- Later identical copies are dropped as duplicates. A same-ID, different-payload copy fails closed while the dedup entry is live.
- Arrival at or after the hard deadline is expired and never delivered. There is no late reliable retransmission semantic.
- Dedup entries are pruned against the injected clock after their deadline.

### Flow type

A `flow_id` is either STREAM or DATAGRAM for its lifetime in the receiver. Reusing the same flow ID with the other type fails closed.

## M3 single-carrier stream framing

The same envelope is self-delimiting on a real TCP byte stream. `ReadFrame` reads the fixed 3-byte prefix, incrementally decodes `bodyLen`, bounds it before allocation, reads exactly that body and then reuses the normal frame decoder. TCP write/segment boundaries are never protocol boundaries. Multiple frames may be coalesced into one TCP read or one frame may arrive across many reads.

`internal/lane.TCP` is intentionally a thin one-carrier wrapper: it serializes concurrent frame writes, permits one framed reader, exposes deadlines/addresses/half-close, and contains no scheduling, reinjection, redundancy or session policy. M3 qualifies this behavior on real `127.0.0.1` TCP sockets.

## Explicit non-goals of M3

- no lane scheduler,
- no FEC/repair frame,
- no RBC,
- no Xray/REALITY integration,
- no TUN/VPN,
- no authentication,
- no public traffic-shaping behavior.

## M4 multi-lane pool semantics

`internal/lane.Pool` joins independent real kernel TCP carriers without changing the WBD wire format. Each carrier has a local `LaneID`; the ID is event/scheduler context and remains outside DATA/DATAGRAM bodies.

M4 provides only deterministic carrier mechanics:

- `SendOn(lane_id, frame)` explicitly selects one active lane.
- `SendNext(frame)` selects active lanes in ascending-`LaneID` round-robin order.
- one receive goroutine per TCP lane fans frames into `Event{LaneID, Frame, Err}`.
- a terminal carrier error marks only that lane inactive; remaining carriers continue to schedule new frames.
- pool shutdown closes all carriers, unblocks readers and closes the fan-in stream.

Two independent TCP sequence spaces therefore can progress independently. A delayed lane can leave a STREAM logical hole while DATAGRAMs or other flows arriving through another lane are delivered immediately. Later STREAM bytes are buffered by M2 until the missing logical prefix arrives. Identical logical bytes received on two lanes with different `transmission_id` values are deduplicated by the M2 receiver.

M4 deliberately does **not** infer loss from delay and does not resend a missing logical range. Logical receipt tracking, ACK/GAP generation and cross-lane reinjection remain M5/M6 work.

## Explicit non-goals of M4

- no ACK-driven recovery policy,
- no automatic cross-lane reinjection,
- no FEC/repair frame,
- no RBC/adaptive scheduling,
- no Xray/REALITY integration,
- no TUN/VPN,
- no authentication.

## M5 logical receipt semantics

M5 turns the already-defined ACK/GAP_HINT frames into receiver-generated logical state without adding sender-side recovery policy.

### ACK ranges

`Receiver.ReceiptFor(flow_id)` derives normalized logical receipt ranges from successful receive state, independent of the lane on which each transmission arrived:

- STREAM ACKs cover the contiguous delivered prefix plus every normalized out-of-order buffered segment.
- DATAGRAM ACKs cover live, delivered datagram IDs; expired arrivals are not acknowledged.
- If more than `MaxAckRanges` normalized ranges exist, the snapshot is split into multiple valid ACK frames.
- a STREAM ACK may carry the envelope FIN flag to report that a consistent logical FIN was observed. This allows an empty stream to be acknowledged with `FIN=true` and zero byte ranges.

Because ACK ranges are half-open, DATAGRAM IDs are capped at `MaxDatagramID = MaxValue-1`; the largest ID therefore remains representable as `[MaxDatagramID, MaxValue)`.

### First observable gap

A receipt exposes at most one current `GAP_HINT`, always the earliest hole that can be proven from received logical state:

- STREAM: `[next_contiguous_offset, first_buffered_offset)`, or a FIN-revealed tail hole when final offset is known.
- DATAGRAM: the first internal gap between live delivered datagram IDs. M5 does not assume IDs before the first observed datagram were necessarily sent.

A GAP_HINT is diagnostic/control information only. It does not mean the original TCP carrier failed and does not trigger retransmission in M5.

### Lane independence

ACK and GAP_HINT belong to the logical session, not to a TCP carrier. A receipt produced from DATA received on lane 2 may be sent on lane 1 (or any later healthy lane). Real two-lane localhost tests qualify this behavior.

## Explicit non-goals of M5

- no sender outstanding-data scoreboard,
- no automatic cross-lane reinjection,
- no gap/ACK timer policy,
- no FEC/repair frame,
- no RBC/adaptive scheduling,
- no Xray/REALITY integration,
- no TUN/VPN.

## M6 cross-lane STREAM reinjection

M6 adds a sender-side logical outstanding scoreboard for reliable STREAM data. Source DATA bytes are copied into the scoreboard after initial scheduling and remain available until logical ACK state proves they no longer need recovery.

### Scoreboard identity

- source records are keyed by logical `(flow_id, offset, payload, FIN)` rather than TCP sequence or lane;
- source byte ranges may not overlap;
- the most recent carrier used for a source record is local recovery state, not wire identity;
- every reinjection allocates a fresh `transmission_id` while preserving logical flow/offset/bytes;
- FIN-bearing source records are retained until both their payload range and logical FIN have been acknowledged.

### GAP-driven reinjection

For a STREAM `GAP_HINT [start,end)`, the sender intersects the gap with still-tracked source data, subtracts already ACKed subranges, and emits only the remaining missing logical bytes. Each rescue attempt chooses a healthy lane different from the most recent lane for that source record. A failed local send consumes its transmission ID and may fall through to another eligible lane.

ACK and GAP may cross in flight on different TCP carriers. Therefore a stale gap for data already pruned by ACK is a normal no-op. A gap for a flow the sender never tracked is rejected.

M6 deliberately does not implement timer-based loss inference. In particular, a FIN that is not observed and does not create a receiver-visible byte gap will need later timer/flight policy. M6 also does not reinject DATAGRAMs; their hard-deadline policy belongs with the later RBC/deadline controller.

### Real-TCP HOL invariant

The M6 localhost integration test uses two independent real TCP connections. The original stream prefix is written to lane 1 while the receiver intentionally leaves that lane outside its active pool, so those bytes remain unavailable to the logical receiver. A later stream tail on lane 2 reveals a gap; the GAP returns over lane 2, and the sender reinjects the original prefix on lane 2 with a new transmission ID. The logical stream completes before receiver lane 1 is activated. When lane 1 is finally activated, the original TCP-delivered copy is accepted only as a logical duplicate.

This proves the core M6 property: WBD logical delivery can bypass one carrier's head-of-line delay without altering that carrier's real kernel TCP behavior.

## Explicit non-goals of M6

- no bounded per-lane flight window or rescue-lane reservation (M7),
- no timer-based retransmission or FIN timeout policy,
- no DATAGRAM reinjection policy,
- no FEC/repair frame,
- no RBC/adaptive scheduling,
- no Xray/REALITY integration,
- no TUN/VPN.
