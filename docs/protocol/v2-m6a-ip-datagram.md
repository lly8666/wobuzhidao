# V2-M6A WBD IP datagram framing

Status: **IMPLEMENTED, qualification pending**

## Purpose

M6A provides the packet-preserving boundary between native TUN/L3 and the already-qualified local FEC/DTLS/FakeTCP composition.

It does not construct a byte stream.

## Wire format

Each IP packet is one WBD data datagram:

```text
0..3   magic        "WBDP"
4      frame version = 1
5      type          = 1 (IP)
6..7   packet length uint16 big-endian
8..    exact IPv4 or IPv6 packet bytes
```

Rules:

- maximum admitted IP packet is 9000 bytes;
- product default TUN MTU starts at 1400 and is tuned by later measurements;
- trailing bytes are rejected;
- IPv4 total-length must exactly match the datagram packet length;
- IPv6 payload-length must exactly match the datagram packet length;
- IPv6 jumbograms are not admitted in the first product version;
- one decoded frame produces exactly one TUN write.

## Local composition

Client reference:

```text
TUN
  → wbd-tun client
  → framed UDP datagram to local UDPspeeder listener
  → UDPspeeder
  → WBD DTLS shim
  → udp2raw FakeTCP
```

Server reference:

```text
udp2raw
  → WBD DTLS shim
  → UDPspeeder decoder
  → framed UDP datagram to wbd-tun server listener
  → TUN
```

The server-side UDP adapter learns the local decoder peer from the first received datagram; this is a single-user local composition, not a public unauthenticated listener.

## M6A exit tests

- IPv4 framing round trip;
- IPv6 framing round trip;
- malformed length/trailing bytes rejected;
- unsupported packet versions rejected;
- bidirectional packet bridge preserves boundaries;
- oversize packets counted as drops;
- `go test ./...` passes on CI.

Privileged `/dev/net/tun` and full raw/FEC/DTLS end-to-end execution are M6B local-authority tests.
