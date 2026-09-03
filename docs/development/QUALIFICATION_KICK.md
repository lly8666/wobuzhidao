# Exact-head release qualification kick

## Purpose

WBD release qualification must be tied to one exact candidate source HEAD. Expensive Windows/Linux workflows intentionally use path filters, so a normal green push is not enough to authorize artifacts.

This file and `.github/workflows/release-qualification-kick.yml` define the mandatory hosted candidate matrix. The aggregator dispatches every opt-in gate against one immutable feature-branch HEAD, resolves required push gates for that exact SHA, and fails if the branch moves or any child is not `success`.

Hosted qualification is not physical release acceptance. The final Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 gate remains separate and must use artifacts/source fencing for the same candidate SHA.

## Product authority covered by this matrix

- Per Transport Lane: one raw FakeTCP SYN lineage / 4-tuple / sequence space from SYN through teardown.
- The bounded Reality-like TLS 1.3 admission phase runs inside that same FakeTCP association.
- The in-band barrier creates no FIN/RST/reconnect/new WBD payload SYN inside a lane.
- Post-barrier DTLS/LINK/FEC payload is packet/datagram oriented and must not inherit ordinary kernel-TCP HOL.
- A Logical Tunnel may own 1..4 independent lanes; Normal desired lanes = 1 and Game/weak-network desired lanes = 2..4.
- Game first-arrival/dedup/no-cross-lane-HOL and generation-fenced make-before-break are product behavior; candidate failure preserves healthy A.
- Lifecycle authority includes randomized 30..60m lane age, child/LINK liveness, Windows underlay/default-route convergence, WBD-owned physical-route rebind, manual reconnect, and existing reconnect-capable server CLOSE semantics.
- Linux final topology is per-lane transport -> Game/race -> Logical Tunnel -> one shared WBD TUN -> one WBD-owned host NAT.
- Mature FakeTCP/TCP-like ACK/SACK/RTO/FEC behavior remains frozen unless a deterministic gate proves a lower-layer defect. Functional lifecycle qualification uses FEC off; fixed systematic 20:20 is compatibility smoke only.

## Explicit workflow-dispatch authority set

The aggregator requires new `workflow_dispatch` runs on the exact candidate SHA for:

1. `product-lifecycle-e2e.yml`
2. `game-settings-matrix.yml`
3. `windows-manual-reconnect.yml`
4. `windows-route-rebind.yml`
5. `windows-linux-single-flow.yml`
6. `windows-portable-bundle.yml`
7. `windows-tun-build.yml`
8. `windows-tun-admin-smoke.yml`
9. `windows-rawip-gateway.yml`
10. `windows-faketcp-persona.yml`
11. `windows-ipv6-killswitch.yml`
12. `windows-dtls-build.yml`
13. `linux-server-release.yml`
14. `linux-server-firewall.yml`
15. `single-flow-rawip-e2e.yml`
16. `mux-load-100m.yml`
17. `single-flow-startup-stress.yml`
18. `single-flow-link-fullstack.yml`
19. `faketcp-recovery-ab.yml`
20. `openwrt-fullstack-one-shot.yml`
21. `game-lane-fullstack.yml`
22. `shared-tun-two-client.yml`

`product-lifecycle-e2e.yml` is release-critical for 1..4 lane lifecycle/Game/make-before-break behavior. `game-settings-matrix.yml` is the explicit V2-M10 setting-boundary authority for Normal=1 and Game/weak=2..4. `windows-route-rebind.yml` provides the focused hosted-Windows WBD-owned physical route rebind/cleanup smoke required by the lifecycle matrix. `windows-manual-reconnect.yml` explicitly qualifies the local manual replacement lifecycle plus existing reconnect-capable server CLOSE semantics on the exact candidate. `game-lane-fullstack.yml` and `shared-tun-two-client.yml` qualify the restored product topology above transport. `mux-load-100m.yml` remains mandatory because the frozen weak-network/load operating point must not disappear from exact-head authority merely because no transport source file changed.

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

Together the 31 hosted child gates cover Windows native protocol/runtime behavior, Windows portable/TUN/admin/raw-IP/IPv6/DTLS/persona, explicit Game settings, manual/server-CLOSE lifecycle, hosted-Windows physical-route ownership rebind/cleanup, Linux raw/netns/server/firewall/release/shared-TUN/one-NAT, same-flow Reality-like TLS, no-second-SYN, post-switch no-HOL, Game 1..4 lanes, lifecycle/make-before-break, FEC off/20:20, weak-network recovery/first-arrival/load, and OpenWrt regressions.

`windows-portable-bundle.yml` names its artifact with the exact run source SHA. `linux-server-release.yml` names both amd64 and arm64 artifacts with that same run source SHA. `windows-npcap-physical.yml` separately requires an explicit `source_sha` and `artifact_run_id`, verifies that the artifact-producing run's `head_sha` matches that source, and checks out the same SHA on the physical runner.

`windows-linux-single-flow.yml` intentionally reports `physical_npcap=0`: hosted CI can prove native Windows code/wire semantics plus Linux consumption/raw-netns full stack, but it cannot replace final physical Windows 11 Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.

## Kick generation

`2026-09-03-lifecycle-settings-route-rebind-exact-head-requalification`

This generation folds the current ADR-0012 lifecycle source lineage through `c42536fb30f4c2fddc17a0169513bc1ed16cf6ce` into an intentional final-candidate qualification graph. It adds explicit Game setting-boundary authority, WBD-owned Windows route-rebind/cleanup authority, and manual/server-CLOSE lifecycle authority that were missing from the previous release kick. No prior green run is inherited: all 31 hosted qualification children must execute against this new immutable candidate HEAD before automated qualification can be treated as green.

## Delivery rule

No Windows/Linux artifact is delivered merely because packaging succeeded. The exact candidate HEAD must first emit `WBD_RELEASE_QUALIFICATION_PASS` after all 31 hosted child gates succeed. Matching Windows portable and Linux amd64/arm64 bundles must identify that same source HEAD. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 DNS/UDP/TCP/lifecycle/cleanup remains final acceptance before calling a release complete.
