# V2-M3A minimal control framing

Status: initial codec gate.

All WBD control messages are application data inside an already-completed DTLS 1.3 association. This format does not replace DTLS authentication/encryption and is independent of udp2raw, FEC, lane identity and the rejected V1 DATA/ACK/GAP protocol.

## Envelope

Exactly one control message is carried in one control datagram:

| Field | Size | Meaning |
| --- | ---: | --- |
| magic | 4 bytes | ASCII `WBDC` |
| frame version | 1 byte | `1` |
| type | 1 byte | HELLO=1, ACCEPT=2, ERROR=3 |
| body length | 2 bytes | unsigned big-endian, maximum 1024 |
| body | variable | exact type body; trailing bytes are invalid |

## Messages

HELLO body is `min_protocol:u16, max_protocol:u16`. Both must be non-zero and `min <= max`.

ACCEPT body is `selected_protocol:u16`, non-zero. The server chooses the highest common version.

ERROR body is `code:u16, UTF-8 message`. Code zero is invalid and the message is bounded to 256 bytes. M3A defines code 1 `no common version`, code 2 `malformed HELLO`, and code 3 `policy`.

M3A product protocol version is `1`.

## Validation

Decoders reject bad magic, unknown frame versions/types, truncated or trailing bytes, bodies above 1024 bytes, invalid HELLO ranges, protocol zero, error code zero and invalid UTF-8. No credentials, keepalive, reconnect state, routing/TUN fields, custom crypto or carrier/lane identifiers are part of M3A.
