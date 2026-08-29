# wobuzhidao

Personal weak-network VPN transport for **OpenWrt/Linux ↔ Linux or Windows**.

V2.3 uses one public TCP-shaped FakeTCP association for the entire WBD session:

```text
one FakeTCP SYN lineage
  -> temporary reliable ordered bootstrap on the same raw flow
  -> real TLS 1.3 / Reality-like recognition + admission
  -> same 4-tuple and sequence space, no second SYN
  -> DTLS 1.3
  -> immutable WBD LINK
  -> optional fixed FEC
  -> packet/datagram VPN payload
```

The carrier is **TCP-shaped, not an ordinary kernel TCP byte stream**. The ordered stream behavior exists only for the short TLS/bootstrap exchange. Sustained VPN payload returns to datagram semantics so a lost earlier FakeTCP payload does not recreate TCP head-of-line blocking.

ADR-0011 supersedes the former V2.2 shape that used a separate ordinary TCP Reality-like admission connection before a fresh FakeTCP association.

Current release transport policy remains one raw lane, legacy FakeTCP shadow recovery, pinned wolfSSL DTLS 1.3, FEC `off` or fixed systematic `20:20`, and a 40 Mbit/s aggregate-inner operating point on <=100 Mbit/s weak links.

V1 multi-ordinary-TCP is permanently rejected and preserved only as historical benchmark evidence.

Start from `.wbd/handoff/current.json` and `CONTINUE_HERE.md` on the active development branch.
