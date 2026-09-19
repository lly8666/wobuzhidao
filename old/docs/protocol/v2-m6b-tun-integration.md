# V2-M6B Linux namespace / real-TUN integration qualification

Status: **HARNESS IMPLEMENTED; privileged runtime qualification pending**

## Goal

Prove that the M6A packet-preserving TUN adapter works through the already-qualified one-lane transport, not merely through an in-memory or plain-UDP unit test.

The required path is:

```text
client namespace
  application / ping
    → wbd0 TUN
    → wbd-tun WBDP datagram
    → UDPspeeder client
    → DTLS 1.3 client shim
    → udp2raw FakeTCP client
    → veth underlay
    → udp2raw FakeTCP server
    → DTLS 1.3 server shim
    → UDPspeeder server
    → wbd-tun WBDP datagram
    → wbd0 TUN
  server namespace
```

Reverse traffic follows the same stack in the opposite direction.

## Harness

`scripts/qualify_v2_m6b_tun.py` creates two network namespaces and a veth underlay, then starts the complete process chain in both namespaces.

It verifies exact SHA-256 values before starting:

- udp2raw amd64: `c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c`
- UDPspeeder amd64: `f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a`
- qualified quiet DTLS shim: `63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a`

The `wbd-tun` binary hash is recorded in the receipt because it is built from the current repository source rather than an externally pinned upstream binary.

## Initial lab addresses

- underlay client: `198.18.40.1/30`
- underlay server: `198.18.40.2/30`
- WBD IPv4 client: `10.77.0.1/30`
- WBD IPv4 server: `10.77.0.2/30`
- WBD IPv6 client: `fd77:7762::1/64`
- WBD IPv6 server: `fd77:7762::2/64`
- initial TUN MTU: `1400`

These are lab-only addresses, not protocol requirements.

## Required tests

The harness fails unless all of the following pass:

1. namespace/veth underlay ping;
2. real DTLS 1.3 client and server both report READY;
3. IPv4 ping through the WBD TUN path;
4. IPv6 ping through the WBD TUN path;
5. UDP echo round trip through the VPN;
6. ordinary TCP echo round trip **inside** the VPN.

The last test is important: inner application TCP is allowed and expected. The architectural prohibition is only against making the **public weak-network carrier** an ordinary kernel TCP byte stream.

## Receipt

The harness writes `<out>/receipt.json` using schema:

`wbd-v2-m6b-tun-qualification/v1`

It records:

- pass/fail and error;
- binary SHA-256 values;
- FEC mode and MTU;
- namespace/underlay/tunnel addresses;
- each test's return code, elapsed time and bounded output;
- final `wbd-tun` packet/byte/drop statistics where available.

## Example

```bash
go build -o /tmp/wbd-tun ./cmd/wbd-tun
sudo python3 scripts/qualify_v2_m6b_tun.py \
  --wbd-tun /tmp/wbd-tun \
  --udp2raw /path/to/udp2raw_amd64 \
  --udpspeeder /path/to/speederv2_amd64 \
  --dtls-shim /path/to/wbd_dtls_shim \
  --cert-dir /path/to/certs \
  --fec 20:20 \
  --mtu 1400 \
  --out /tmp/wbd-m6b
```

The default path passes udp2raw `-a`, so `iptables` must be available. `--no-auto-rule` is provided only for environments where equivalent RST suppression is configured separately.

## Current environment blocker

The development container used while this harness was added is root and has `ip`, but `/dev/net/tun` is absent. Therefore it cannot be counted as a real M6B qualification host. The harness deliberately fails instead of substituting an emulated TUN test.

CI only checks that the harness parses safely and retains the exact qualified transport hashes. The authoritative M6B result must come from a Linux/OpenWrt environment with a real TUN device and raw-packet privileges.
