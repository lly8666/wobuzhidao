# wobuzhidao

Personal weak-network VPN transport for **OpenWrt/Linux ↔ Linux or Windows**.

V2.4 separates the long-lived logical VPN from its replaceable public transports:

```text
Logical Tunnel
  - server-assigned unique tunnel IP lease
  - stable race SessionID / PacketID space
  - 1..N active Transport Lanes

Each Transport Lane:
  one FakeTCP SYN lineage
    -> bounded reliable ordered TLS/bootstrap on the same raw association
    -> real TLS 1.3 / Reality-like recognition + admission
    -> same lane 4-tuple / sequence space
    -> DTLS 1.3
    -> LINK
    -> lane-local optional fixed FEC
    -> packet/datagram VPN payload
```

The carrier is **TCP-shaped, not an ordinary kernel TCP byte stream**. Ordered behavior exists only for the short TLS/bootstrap phase of each lane. Sustained VPN payload returns to datagram semantics so an earlier missing FakeTCP range does not recreate ordinary TCP head-of-line blocking.

Normal mode targets one steady lane. The later Game Lane/race layer may use 2..4 independent complete WBD associations for first-arrival delivery and duplicate suppression, and make-before-break replacement may briefly overlap an old lane with a candidate. This is not the rejected V1 ordinary-kernel-TCP multilane architecture.

Windows raw-L3 product direction is server-assigned unique tunnel IPs, one shared Linux TUN and one WBD-owned host NAT. The per-LiveID netns/veth/double-NAT implementation is historical/reference evidence, not the selected final product.

Current frozen release limits remain: `legacy` FakeTCP shadow recovery, pinned wolfSSL DTLS 1.3, FEC `off` or fixed systematic `20:20`, <=100 Mbit/s weak-link qualification ceiling, and **40 Mbit/s aggregate-inner** conservative release operating point.

Read ADR-0012, `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, `.wbd/handoff/current.json` and `CONTINUE_HERE.md` on the active development branch before editing.
