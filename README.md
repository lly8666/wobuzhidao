# wobuzhidao

Personal weak-network VPN transport for **OpenWrt/Linux ↔ Linux or Windows**.

V2.2 keeps the public data plane deliberately simple:

```text
TUN / IP packets
  → minimal WBD packet framing
  → fixed-mode FEC
  → DTLS 1.3
  → TCP-shaped FakeTCP raw carrier
  → public network
```

The carrier is **TCP-shaped, not an ordinary kernel TCP byte stream**. Product payload must retain datagram semantics so packet loss does not recreate TCP head-of-line blocking.

An optional TLS Persona bootstrap may use a standard TLS 1.3 connection with browser-like ClientHello profiles (`native`, `chrome`, `firefox`, etc.) before the data lane is admitted. Persona is optional appearance/metadata protection; DTLS 1.3 remains the security authority for the VPN data plane.

V1 multi-ordinary-TCP is permanently rejected and preserved only as historical benchmark evidence.

Start from `CONTINUE_HERE.md` on the active development branch.
