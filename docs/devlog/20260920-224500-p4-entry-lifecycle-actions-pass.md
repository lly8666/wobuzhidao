# 20260920-224500 P4 multi-lane/lifecycle Actions PASS

## 本轮目标和阶段

仍为 P4。产品资格对象为 SOURCE_SHA `fa9b60315ca01ec140c34a65799dc62916f2275d`，其父链包含本 atom 的 protected LaneID、multi-lane runtimeentry、replacement/DORMANT/wake、ledger fix、qualification race fix 与 readiness observability。Actions run 35505737597 已 completed/success，7/7 jobs PASS。

## 修改与原因

本轮是 evidence closure，不新增产品行为：
- protected admission 的 `lane_id=1..4` 把每条独立 FakeTCP association 明确绑定同一 Logical Tunnel 的 authoritative lane / same-ID replacement；旧单lane零值仍规范化为lane1。
- client/server runtimeentry 保持一个 stable leased `TunnelOwner`，Game 2-lane hosted场景真实完成 TLS/admission、PacketID racing、失败candidate保留旧generation、A->A+B->B、retire、DORMANT/wake与反向egress。
- server egress资格继续要求每条desired lane至少出现一个post-admission steady payload；`TunnelQualified`只是只读观测，不放宽产品门槛。
- Linux/OpenWrt raw client通过exact four-tuple `SegmentMux`共享endpoint；Windows按incarnation使用既有Npcap generation endpoint；命令面支持 `--lanes 1..4`、idle-dormant和bounded rotation。
- 不恢复old Controller、UDP Game子进程/控制socket、localhost carrier、DTLS或platform-proxy topology。

## 复用来源

源 SHA 固定为 `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。本 atom 仅定向参考：
- `old/cmd/wbd-game-lane-client/activity_control.go` 的payload idle边界；
- `old/cmd/wbd-game-lane-client/qualification.go` 的candidate-before-retire原则；
- `old/cmd/wbd-game-lane-server/replacement_recovery.go` 与 serialized reconnect test 的逐lane收敛原则。

REUSE_LEDGER 已在前序候选拆成真实一对一路径，当前更新note为exact-SHA PASS；无新增归档代码迁移。

## Actions证据

SOURCE_SHA: `fa9b60315ca01ec140c34a65799dc62916f2275d`
Run: https://github.com/lly8666/wobuzhidao/actions/runs/35505737597
结论：7 jobs全部PASS。

- repository-contract: PASS。
- Ubuntu 24.04 full unit/build: `internal/runtimeentry` PASS 0.064s，`internal/realityfront` PASS 0.609s。
- Ubuntu race: `go test -race ./... -count=1` PASS；`internal/runtimeentry` 1.118s，`internal/realityfront` 1.687s。
- tlsrecord directed fuzz 15s: 121101 execs，PASS。
- Windows Server 2022: `internal/runtimeentry` PASS 0.077s，`internal/realityfront` PASS 0.666s；Wintun marker `WBD_WINDOWS_CLIENT_PLAN schema=wbd-windows-client-state/v1 adapter=WBD lease=10.66.0.7/32`；Npcap hosted `WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN`。
- P2 kernel fallback: `TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS 1.24s；29 packets captured / 58 received by filter / 0 dropped；analyzer `result=PASS`。
- Linux shared-TUN: iptables PASS 0.10s、nft PASS 0.15s；两者均 `active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged`。
- OpenWrt: `WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED`，test 0.16s PASS；`WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED`，test 0.12s PASS。
- Artifacts:
  - foundation 10603124698 sha256:8ff39f138ce9b5a3c9cdc3df74f9597c8bdd353743459beaf673381b41bf2aff
  - tlsrecord-reference 10603796058 sha256:a6b382296adc5657856c72cb54af4de7ecb0771e5702c559f6c5bd31d5a1db2a
  - P2 10603084841 sha256:81d0efaa059571c8d08012b8b9f8d33c9b89a57c032ada0d2cd771533582f619
  - shared-TUN iptables 10604110334 sha256:548ea40f98bd2c671a322a7723b172754bf4ef153f12d8b6de0e1f7a36b7f242
  - shared-TUN nft 10603781037 sha256:b713fac151f9c6799ff5d50f1c386da25facf4f3f3014d69fa1b0fb5cfe6e6b8
  - OpenWrt 10603429122 sha256:418f10bbf66d9af53c4ab9a7acc27f1457830b0411cbca8e32c903dfdd9e5347

真实 Windows Npcap driver / 管理员物理NIC capture/injection仍为 `NOT_RUN`，只属于P7，不能称PHYSICAL_PASS。

## 问题、排查与风险

本 atom 保留了三次失败证据：
1. `173f9de0...` / 35505359301：REUSE_LEDGER多路径字段导致repository-contract FAIL，产品jobs未运行。
2. `a1bf7014...` / 35505438219：6/7 PASS，Ubuntu race暴露旧single-lane资格bool落锁时序；修为基于同transport `Received>0` 的无等待刷新。
3. `a696e502...` / 35505592364：6/7 PASS，新lifecycle test只等待tunnel-wide `GameDelivered`，未等待所有lane steady资格；产品门槛不放宽，改用只读 `TunnelQualified` 等待明确状态。

当前仍不能关闭整个P4的唯一理由是入口级直接证据尚未显式跑3/4-lane以及自动rotation/idle timer路径；owner级这些语义已有既有资格，但closure audit不应把owner单元测试冒充最终入口矩阵。

## 下一项原子任务

P4 closure audit：只扩展 `internal/runtimeentry` hosted lifecycle测试/必要只读观测，显式覆盖 desiredLanes=3/4 的TLS/admission->Game racing/steady qualification/cleanup，并覆盖自动rotation interval和payload-idle DORMANT/wake；ACK/timer不得刷新idle。不改wire、FEC/recovery或平台拓扑。exact-SHA全矩阵PASS后再决定P4 CLOSED；不在同一atom启动P5。
