# 20260920-214500 P4 可执行入口 multi-lane/lifecycle 候选

## 本轮目标和阶段

阶段仍为 P4。开始远端 HEAD 为 `8e85b4beedda2315be6d7d4f22fafd91df14e383`，上一产品资格仍是 `79c6e9ddcbc3e7f74b85dcc6d162d5c1a3c4e3f9` / Actions 35500769979。本轮只闭合当前单进程 entry 的 Game 2..4、same-ID replacement/retire、DORMANT/wake 和稳定 lease 生命周期，不进入 P5。

## 修改与原因

- `internal/realityfront/admission.go`：protected admission request/reply 增加 `lane_id(u8)`，合法范围 1..4，server 原值回显，client 校验一致。旧单 lane API 的零值配置规范化为 lane 1；未知/越界/回显不一致 fail-closed。该字段只在已有 TLS 保护内存在，不增加公开 SYN/TLS 标记，也不改变 TLS-like exporter 固定上下文。
- `internal/runtimeentry/lifecycle.go`：新增多 lane 单进程 client/server lifecycle。每 lane 仍是一条独立 FakeTCP association；客户端 raw endpoint 通过 exact four-tuple `SegmentMux` 分发。一个 leased `TunnelOwner` 持续跨 replacement 与 DORMANT/wake，Game 2..4 仍调用现有 `GameOutbound/Inbound` PacketID racing/dedupe。
- replacement 严格串行且同 LaneID。候选在 FakeTCP+TLS+protected admission 失败时不改 authoritative generation；成功后通过现有 `runtimeowner.BeginSameIDReplacement/PromoteSameIDReplacement` 进入 A+B physical overlap，旧 generation 受 fencing，首个新 steady record/ACK 可提前 retire；无业务时也在有界 replacement grace 后释放 retiring incarnation，不让过渡槽永久占用。
- payload idle 单独记账：业务包、platform service frame 才刷新；FakeTCP ACK、repair tick、rotation/qualification 不刷新。DORMANT 清空 physical associations 但不替换 `TunnelOwner`/lease/Game state；wake 重新建立同 lane IDs。
- `cmd/wbd-client` Linux/OpenWrt 与 Windows 新增 `--lanes`、`--idle-dormant`、`--rotate-min/max`；Linux raw 使用共享 `SegmentMux`，Windows 每 incarnation 使用现有 Npcap generation endpoint。source port 使用有界 1024-port rotation window，不无限增长。
- `cmd/wbd-server` 切到 lifecycle server，支持 `--lanes` 和 payload-idle DORMANT。
- OpenWrt `SocketConfig.BeforeBusiness` 在真实透明 TCP/UDP flow 进入 platformflow 前触发 wake；普通 transport timer/ACK 不触发。
- `docs/WIRE_SPEC.md` 记录 protected lane_id 语义，同时明确 exporter context/record v1 固定向量不因 lane_id 改动。

## 复用来源

源 SHA 固定为 `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。只查看并提取：
- `old/cmd/wbd-game-lane-client/activity_control.go` 的 payload activity 边界；
- `old/cmd/wbd-game-lane-client/qualification.go` / replacement qualification test 的候选先资格、旧路径不提前退休原则；
- `old/cmd/wbd-game-lane-server/replacement_recovery.go` / serialized reconnect test 的逐 lane 串行 convergence 原则。

未恢复 archived UDP Game data/control process、CLIENT_LEAVE wire、localhost carrier、旧 Controller、DTLS、platform-proxy 子进程或旧 CLI topology。REUSE_LEDGER 已更新。

## Actions证据

当前仅为候选提交前记录，尚无本轮 SOURCE_SHA / Actions run。状态：NOT_RUN。资格仍只属于上一产品 SHA `79c6e9dd...`，本日志不能把候选描述成 PASS。

计划 gate 仍为现有 `next-foundation` 全矩阵：repository-contract；Windows/Linux unit/build；Linux race + tlsrecord fuzz/reference；P2 kernel fallback；Linux shared-TUN iptables/nft；OpenWrt TPROXY + SocketTunnel。Windows/Npcap physical 仍为 P7 `NOT_RUN`。

## 问题、排查与风险

- 多 association 的 LaneID 不能靠服务端到达顺序推断：TLS reply 与 runtime attach 存在并发窗口，会导致 Game envelope LaneID 与 transport lane 交叉。因此本候选把 LaneID 放进已有 protected admission 并回显校验，不另造公开握手。
- replacement 若只依赖业务流量退休，空闲连接会永久占 retiring slot。因此增加有界 overlap grace；新 steady record/ACK 可提前退休，grace timer 本身不算 payload activity。
- hosted Windows 只能验证编译/mock contract；真实 Npcap driver/物理 NIC 仍不能据此称 PHYSICAL_PASS。
- OpenWrt IPv6 继续 NOT_IMPLEMENTED；本 atom 不改 FEC/recovery 参数。

## 下一项原子任务

把当前候选提交到 `next/tlslike-dataplane`，以该精确 SOURCE_SHA 跑完整 Actions。任何失败只修本 atom；全部 PASS 后写 evidence closure、更新 `last_tested_source_sha`，再依据 ACCEPTANCE 的 P4 条件判断是否关闭 P4，不提前进入 P5。
