# 2026-08-30 — Single-flow observability and netns raw-IP gateway

## Recovery point

The one-public-flow transport is already the active direction on `feat/single-flow-reality-faketcp`:

`FakeTCP SYN lineage -> Reality-like reliable bootstrap -> same association -> DTLS 1.3 -> LINK -> M6A raw IP`.

The FakeTCP recovery/FEC/DTLS/LINK wire is frozen for this work. The physical Windows->Ubuntu run proved the single public flow can reach DTLS and LINK. The next deterministic blocker is Linux sustained L3 egress above LINK.

The previous raw-IP gateway implementation still uses a VRF plus conntrack-zone prototype. That conflicts with the already-recorded design conclusion: two LiveIDs may legitimately reuse the identical Windows inner tuple (`10.66.0.2`, including the same TCP source port), and public return traffic cannot select the correct root-namespace conntrack zone before lookup. The implementation and qualification must therefore move to one network namespace per raw-IP session.

## Required netns model

For each raw-IP backend session:

1. allocate a WBD-owned slot;
2. create a TUN and keep its file descriptor in the gateway process;
3. create a dedicated network namespace;
4. move the TUN into that namespace;
5. create a root<->namespace veth /30 from the WBD transit prefix;
6. assign `10.66.0.1/30` to the namespace TUN; Windows remains `10.66.0.2/30`;
7. enable IPv4 forwarding in the namespace;
8. apply inner NAT in the namespace (`10.66.0.0/30 -> unique transit IP`);
9. apply one WBD-owned host NAT for the transit prefix to the physical egress path;
10. return traffic uses host conntrack to select the unique veth and namespace conntrack to reverse inner NAT;
11. deterministic cleanup removes namespace, veth, TUN and WBD-owned NAT state.

No public/wire protocol changes are permitted for this repair.

## Operator log contract

Default logging is event-driven. Per-packet TCP sequence/ACK, DTLS records, FEC shards and TUN packets are forbidden in default logs and belong behind explicit trace flags later.

### Session correlation

A six-hex-character `sid` is derived with SHA-256 from the existing session-bound admission identity. The source ticket/LiveID is never printed. Windows derives the same SID after admission; Linux LINK derives it from the consumed LiveID. Raw-IP gateway receives the SID over a localhost-only backend metadata frame before the first M6A packet.

Never log username, password, ticket, LiveID bytes/prefix, route-key or authentication proof.

### First boundary crossings

Emit each marker at most once per session/direction. Initial target set:

Windows: `WBD_TUN_TX_FIRST`, `WBD_LINK_TX_FIRST`, `WBD_DTLS_TX_FIRST`, `WBD_FAKETCP_TX_FIRST` and corresponding first return boundaries where practical.

Linux: `WBD_FAKETCP_RX_FIRST`, `WBD_DTLS_RX_FIRST`, `WBD_LINK_RX_FIRST`, `WBD_RAWIP_RX_FIRST`, `WBD_NETNS_TUN_TX_FIRST`, plus first gateway return packet.

The raw-IP gateway implementation in this change owns `WBD_RAWIP_RX_FIRST`, `WBD_NETNS_TUN_TX_FIRST`, and the reverse first-TUN marker. Existing lower layers will be migrated incrementally without changing their packet behavior.

### L3 session lifecycle

Creation must emit a single line similar to:

`WBD_RAWIP_SESSION_READY sid=xxxxxx netns=wbdg00 tun=wt00 inner=10.66.0.1/30 veth_host=wh00 veth_ns=we00 transit_host=198.18.240.1 transit_ns=198.18.240.2 nat=ready`

Cleanup must emit:

`WBD_RAWIP_SESSION_CLEAN sid=xxxxxx netns=removed veth=removed tun=removed nat=removed reason=...`

These lines intentionally make repeated-connect stale-state failures obvious.

### Counters

Maintain counters in memory instead of logging packets. Raw-IP session cleanup prints `rawip_tx`, `rawip_rx`, `tun_tx`, `tun_rx`, `drop`. Other layers should expose equivalent aggregate counters at disconnect / failure snapshot boundaries.

### Failure snapshot

A later step in this same branch adds a compact diagnostic snapshot on startup/DTLS/LINK/DNS/route/cleanup failures. It must include commit/version, SID, stage timings, underlay identity hashes, FakeTCP tuple, RST-seen state, route/DNS/IPv6 state and aggregate counters, but no credentials or session secrets. Linux snapshots include gateway sessions, netns/veth/TUN/routes, WBD firewall state, conntrack summary and counters.

## Qualification before delivery

Do not publish a new physical-test package until all of the following are green on the same source head:

- Go unit tests / main CI;
- Windows build and portable bundle;
- Linux amd64 + arm64 server release;
- single-flow FakeTCP/Reality-like/DTLS/LINK qualification;
- raw-IP gateway qualification with two simultaneous clients both using `10.66.0.2/30` and the same TCP source tuple;
- iptables and nft variants where the runner supports them;
- deterministic second-connect cleanup/recreate checks;
- handoff verifier.

Physical Windows 11 + Npcap + real NAT/ISP remains the final non-emulatable qualification, but CI must first exercise all upstream/downstream components that can be virtualized.
