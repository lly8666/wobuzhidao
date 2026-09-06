# Exact-head release qualification kick

## Purpose

WBD release qualification must be tied to one exact candidate source HEAD. Expensive Windows/Linux workflows intentionally use path filters, so a normal green push is not enough to authorize artifacts.

This file and `.github/workflows/release-qualification-kick.yml` define the mandatory hosted candidate matrix. The aggregator dispatches every opt-in gate against one immutable feature-branch HEAD, resolves required push gates for that exact SHA, and fails if the branch moves or any mandatory child is not `success`.

Workflow-level `success` alone is insufficient release authority. After each mandatory child completes, the aggregator enumerates that run's complete job inventory and requires every release-authority job to be `completed/success`; a skipped mandatory job is not a pass. The ordinary `ci.yml` workflow also contains four PR-title-gated transport benchmark/research jobs (`transport_prepare`, `transport_smoke`, `transport_bench`, and `transport_aggregate`). Those jobs are explicitly non-authority for this release kick and may remain skipped on an exact-SHA push; the `ci.yml` `test` job is mandatory, and any new/unclassified `ci.yml` job forces the release contract to be reviewed instead of being silently ignored. Before publishing the authority marker, the aggregator also requires three exact-source release artifact receipts: Windows portable, Linux amd64, and Linux arm64. Each receipt must be unexpired, belong to the exact child run selected by this aggregator, carry the candidate SHA in its artifact name, and report that same SHA as the producer run `head_sha`.

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
- Startup/replacement timeout budgets must tolerate paths around 300ms one-way (~600ms RTT) without classifying ordinary propagation delay as protocol failure. Retry intervals and outer process-readiness budgets must not preempt the protocol stage they supervise.
- The mandatory `single-flow-startup-stress.yml` gate includes a real NAT/netns full-stack case with `tc netem` adding 300ms on each router egress direction. That case must reach same-flow Reality admission, DTLS 1.3, LINK READY, authenticated Logical Tunnel config, and a real UDP payload echo before it passes.

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

The mandatory job rule for these push gates is explicit. `ci.yml:test` is release-authority and must execute successfully. Its four PR-only transport benchmark/research jobs are non-authority and may be skipped on the exact-SHA push; they are kept outside release authority so this product phase does not accidentally restart the frozen 20:20 matrix/research program. The other eight push workflows expose only authority jobs for this kick, so every job in those runs must be `completed/success` with zero mandatory skips.

Together the 31 hosted child gates cover Windows native protocol/runtime behavior, Windows portable/TUN/admin/raw-IP/IPv6/DTLS/persona, explicit Game settings, manual/server-CLOSE lifecycle, hosted-Windows physical-route ownership rebind/cleanup, Linux raw/netns/server/firewall/release/shared-TUN/one-NAT, same-flow Reality-like TLS, no-second-SYN, post-switch no-HOL, Game 1..4 lanes, lifecycle/make-before-break, FEC off/20:20 compatibility smoke, weak-network recovery/first-arrival/load, and OpenWrt regressions.

`windows-portable-bundle.yml` names its artifact with the exact run source SHA. `linux-server-release.yml` names both amd64 and arm64 artifacts with that same run source SHA. The release aggregator does not infer artifact success from workflow status: it resolves those exact child run IDs and verifies all three artifact receipts against the same candidate SHA before it can emit `WBD_RELEASE_QUALIFICATION_PASS`. `windows-npcap-physical.yml` separately requires an explicit `source_sha` and `artifact_run_id`, verifies that the artifact-producing run's `head_sha` matches that source, and checks out the same SHA on the physical runner.

`windows-linux-single-flow.yml` intentionally reports `physical_npcap=0`: hosted CI can prove native Windows code/wire semantics plus Linux consumption/raw-netns full stack, but it cannot replace final physical Windows 11 Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.

## Kick generation

`2026-09-06-game-loopback-collision-exact-head-refresh`

This generation starts a fresh exact-head hosted/package qualification after the Windows physical run exposed the first definite broken layer after LINK: the Game child could exit before READY when the fixed local loopback UDP pair `127.0.0.1:48101/48102` was already occupied. The Windows runtime now prefers that historical pair but selects a free fallback pair when needed, and the TUN/Game control plan consumes the same selected endpoints consistently. These UDP sockets are loopback-only process IPC; the public transport remains Npcap/raw FakeTCP on the configured TCP-looking lane and no FakeTCP/Reality/DTLS/LINK/Game/FEC wire format is changed.

The exact-head Windows VM gate includes a real compiled `wbd-game-lane-client.exe` integration case that occupies the default loopback pair and requires `WBD_GAME_LANE_CLIENT_READY` on the fallback pair. This kick changes qualification documentation only; it deliberately creates the immutable candidate SHA that the release aggregator can use to dispatch all 22 workflow-dispatch gates, resolve the nine exact-candidate push gates, and produce/source-fence the Windows portable plus Linux amd64/arm64 artifacts under one SHA.

The user-visible MTU remains the inner/Wintun MTU. MTU 1300 remains valid for the next physical acceptance run; Game LINK plaintext budget remains derived as inner MTU plus the fixed 32-byte Game envelope. Functional release qualification remains FEC off.

No hosted/package or physical evidence from `e601778ceb421aee3557f32e58e0ba9c5a877ec4`, `fb461dc870d35645a9b7c6e54862c3429a37b547`, or any earlier SHA transfers as release authority to the fresh candidate produced by this generation; the earlier e601 Windows VM collision test is development evidence and must re-execute on the new candidate through the exact-head matrix.

## Delivery rule

No Windows/Linux artifact is delivered merely because packaging succeeded. The exact candidate HEAD must first emit `WBD_RELEASE_QUALIFICATION_PASS` after all 31 hosted child workflows execute successfully, every mandatory release-authority job executes successfully with zero mandatory skips, and the three same-source artifact receipts pass their run/SHA fence. Matching Windows portable and Linux amd64/arm64 bundles must identify that same source HEAD. The four `ci.yml` PR-only transport benchmark/research jobs are not release-authority and may remain skipped on a release-candidate push. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 DNS/UDP/TCP/lifecycle/cleanup remains final acceptance before calling a release complete.
