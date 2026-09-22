# 2026-09-23 strict weaknet exact-SHA trigger and cost ledger

Base SHA: `d8b1325bb26895ba271de87a85d8e8d62acdd7fe`.

The existing strict qualification target is retained unchanged: Normal = one lane, FEC 20:20, 10 Mbps application input in each direction; Game = four lanes, FEC 20:20, 3 Mbps logical application input in each direction total, not 3 Mbps per lane; mixed 64/256/1200 byte packets; 300 ms one-way delay; lossless and 5→20/30→5 stages; seeds 101/202/303; one runner per sample.

This round adds a push trigger for the strict workflow so the current source SHA produces formal evidence without manual dispatch. No performance thresholds or target rates were weakened.

The sample cost ledger now separately reports:
- FEC parity bytes from sender endpoint counters;
- Game logical bytes, lane-copy bytes and replication extra bytes;
- repair repeated TCP payload bytes and repair outer-IP bytes from pcap;
- HEALTH record count and exact 40-byte TLS-like record wire size before FakeTCP/IP overhead;
- padding bytes;
- startup handshake outer bytes and reconnect new-flow counts/control-byte lower bound, with new-flow carrier totals explicitly marked as an upper total that may include steady business.

This accounting is descriptive and does not double-count recovery components as independent delivered benefit. The older AF_PACKET/uplink-capacity investigation remains HOLD and is not resumed by this workflow.
