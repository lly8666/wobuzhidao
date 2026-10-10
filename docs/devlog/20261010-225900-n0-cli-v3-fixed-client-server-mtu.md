# N0 production CLI fixed/off V3 exposure — Actions pending

## Immutable source record
Working branch next/adaptive-fec-aes-tun-20261010. Exact parent HEAD 5f60d169ad67c6191281ae8e424bc3ec6f916a6f, tree dc713054e6230ff560344fdd7bd467ddf7e5deaf. New exact SOURCE is the SHA of the commit containing this devlog and unique STATUS, as identified by GitHub/Actions GITHUB_SHA. Inherited product source 7fb98fab79834a351a1dbe04eebb207f66bea28b historical, not current qualification.

## Parent SOURCE actual GitHub Actions
next-foundation https://github.com/lly8666/wobuzhidao/actions/runs/38061330856, exact SHA 5f60d169: completed SUCCESS, repository-contract + Linux/Windows active-go-tests, p2 kernel fallback, OpenWrt TPROXY, Linux shared-TUN iptables/nft. This is scoped regression, not real client heterogeneity/p99/performance/Windows native Wintun. Previous 9729 and da885 fail logs/evidence retained in STATUS unchanged.

## Changed product wiring
- Introduce FixedPolicyForClient mapping existing shipped fec-parity=0/4/8/10/12/16/20 and Normal/Game desired lanes into V3 authenticated canonical policy, remaining fixed/off, ChaCha only, quality=0; 0 is genuinely off from first packet. Reject other values, never silently activate auto on an old config.
- Linux and native Windows client CLI create V3 admission with that policy (also Windows check-config validator); same captured policy reused for rotation as part of frozen TunnelClientConfig. Server already accepts heterogeneous per-Tunnel V3 and V2 historical parser, checks policy conflicts, and derives each lane's FEC from the protected reply regardless of server global fec-parity.
- Linux shared server TUN now reserves FEC-on wrapper overhead statically for clients of different FEC profiles; per-Tunnel lane record/LINK/FEC budgets still derive from negotiated peer MSS/limits. Does not resize shared TUN on client off/on. Linux nft routing implementation untouched.
- Existing client fec-parity parameter semantics unchanged; update wire and PARAMETERS prose, not a new option. New auto/AES/quality flags are NOT implemented; cannot advertise in CLI/JSON/GUI.
- Directed validation unit confirms fixed parity and transport mapping never migrates to auto.

## Status and evidence boundary
All build/unit/race/integration on GitHub Actions, no local Go. This newest code SOURCE tests NOT_RUN_PENDING_ACTIONS at writing. No measurements of CPU flags/quotas/PSI/steal, throughput injection/local drop or single-sample performance. No Windows real TUN/direct routing changes/functional verification; physical NOT_RUN. This is N0 in progress, not accepted.
Open: multi-client same-port realpath, protected V3 policy vs rotation, server allow-list, early S2C, TLS actual ciphers, N1..N6, historical Q2 FAIL, ~80s S2C and multi-second late. No deployed machine, mainline merge, secrets or raw pcap.
