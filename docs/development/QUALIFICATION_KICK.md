# Exact-head release qualification kick

## Purpose

WBD release qualification must be tied to one exact candidate source HEAD. Expensive Windows/Linux workflows intentionally use path filters, so a normal green push is not enough to authorize artifacts.

This file and `.github/workflows/release-qualification-kick.yml` define the mandatory hosted candidate matrix. The aggregator dispatches every opt-in gate against one immutable feature-branch HEAD, resolves required push gates for that exact SHA, and fails if the branch moves or any child is not `success`.

## Product authority covered by this matrix

- Per Transport Lane: one raw FakeTCP SYN lineage / 4-tuple / sequence space from SYN through teardown.
- The bounded Reality-like TLS 1.3 admission phase runs inside that same FakeTCP association.
- The in-band barrier creates no FIN/RST/reconnect/new WBD payload SYN inside a lane.
- Post-barrier DTLS/LINK/FEC payload is packet/datagram oriented and must not inherit ordinary kernel-TCP HOL.
- A Logical Tunnel may own 1..4 independent lanes; Game/race and make-before-break are product behavior.
- Linux final topology is per-lane transport -> Game/race -> Logical Tunnel -> one shared WBD TUN -> one WBD-owned host NAT.
- Mature FakeTCP/TCP-like ACK/SACK/RTO/FEC behavior remains frozen unless a deterministic gate proves a lower-layer defect.

## Explicit workflow-dispatch authority set

The aggregator requires new `workflow_dispatch` runs on the exact candidate SHA for:

1. `windows-linux-single-flow.yml`
2. `windows-portable-bundle.yml`
3. `windows-tun-build.yml`
4. `windows-tun-admin-smoke.yml`
5. `windows-rawip-gateway.yml`
6. `windows-faketcp-persona.yml`
7. `windows-ipv6-killswitch.yml`
8. `windows-dtls-build.yml`
9. `linux-server-release.yml`
10. `linux-server-firewall.yml`
11. `single-flow-rawip-e2e.yml`
12. `mux-load-100m.yml`
13. `single-flow-startup-stress.yml`
14. `single-flow-link-fullstack.yml`
15. `faketcp-recovery-ab.yml`
16. `openwrt-fullstack-one-shot.yml`
17. `game-lane-fullstack.yml`
18. `shared-tun-two-client.yml`

The two additions that are release-critical for the current product layer are `game-lane-fullstack.yml` (1..4 lanes, FEC off/20:20, race/dedup/no-cross-lane-HOL behavior) and `shared-tun-two-client.yml` (two authenticated Logical Tunnels sharing one Linux WBD TUN and one host NAT with lease/source isolation).

## Exact-candidate push authority set

The aggregator additionally resolves and waits for these exact-SHA push runs:

1. `ci.yml`
2. `faketcp-native.yml`
3. `faketcp-pcap-20loss.yml`
4. `faketcp-first-arrival.yml`
5. `fullstack-first-arrival.yml`
6. `openwrt-tcp-tproxy.yml`
7. `single-flow-e2e.yml`
8. `single-flow-no-hol.yml`
9. `single-flow-tcp-persona.yml`

Together the 27 child gates cover hosted Windows native protocol/runtime behavior, Windows portable/TUN/admin/raw-IP/IPv6/DTLS/persona, Linux raw/netns/server/firewall/release/shared-TUN/one-NAT, single-flow Reality-like TLS, no-second-SYN, post-switch no-HOL, Game 1..4 lanes, FEC off/20:20, weak-network recovery/first-arrival/load, and OpenWrt regressions.

`windows-linux-single-flow.yml` intentionally reports `physical_npcap=0`: hosted CI can prove native Windows code/wire semantics plus Linux consumption/raw-netns full stack, but it cannot replace final physical Windows 11 Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.

## Kick generation

`2026-09-01-seq80-fix-stale-linux-settings-contract`

## Delivery rule

No Windows/Linux artifact is delivered merely because packaging succeeded. The exact candidate HEAD must first emit `WBD_RELEASE_QUALIFICATION_PASS` after all 27 hosted child gates succeed. Matching Windows/Linux bundles must then identify that same source HEAD. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 remains final acceptance before calling a release complete.
