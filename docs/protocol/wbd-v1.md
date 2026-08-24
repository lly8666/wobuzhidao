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
