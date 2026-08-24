# WBD v1 protocol seed

M1 deliberately defines only four frame types. It is not yet a complete session protocol.

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

Envelope `FIN` marks the final logical stream extent carried by this frame. Later stream state work will define exact FIN consistency rules.

## DATAGRAM

Body:

```text
flow_id          uvarint
datagram_id      uvarint
transmission_id  uvarint
payload_len      uvarint
payload          bytes
```

Datagram reliability/deadlines are deliberately not encoded yet. M2 owns expiration semantics.

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

## Explicit non-goals of M1

- no lane scheduler,
- no FEC/repair frame,
- no RBC,
- no Xray/REALITY integration,
- no TUN/VPN,
- no authentication,
- no public traffic-shaping behavior.
