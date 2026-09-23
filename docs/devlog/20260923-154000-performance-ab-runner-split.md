# 20260923-154000 ordered A/B runner split 与 final-race 收口

## 当前性能结论

候选产品 `75b5cc9a82458a7a46d382373339193276d6c51e` 已有两个独立 standalone Normal10 lossless 全PASS样本，但同runner顺序稳定性没有通过，因此性能专项继续 OPEN。

### standalone 已通过

1. seed631 capacity run 35823231138 / job 107059368064 / artifact 10733329818：
   - 五类classification全PASS；
   - 双向三阶段约10Mbps、业务丢失0；
   - AF_PACKET/socket drops=0；
   - Abandoned/RepairEvicted=0，PeakOutstanding 4035/4036；
   - client/server CPU约56.87/57.09 CPU-s / 120s。

2. seed601 performance-recovery run 35823219345 attempt2 / job 107065634129 / artifact 10735098432：
   - 五类classification全PASS；
   - C2S pre/stress/post约10.000064/9.999979/9.999910Mbps；
   - S2C约9.999996/10.000013/9.999979Mbps；
   - 全部packet/socket drops=0；
   - server PeakOutstanding=4080，Abandoned/RepairEvicted/Retransmitted=0；
   - client/server CPU约76.37/76.75 CPU-s / 120s。

### ordered A/B 结果

run **35825736119**，BASE=a924b7c9（已有raw receive scratch修复，仍有post-Sendto clone），FIX=75b5cc9a（删除该clone）。

- AB job **107066929237**：SUCCESS。
  - before与after都五类PASS。
  - before CPU client/server 58.79/58.81s；after 58.36/58.52s。
  - before handler≈19.19us/read；after≈18.93us/read。
  - 两者AF_PACKET drops均0，双向三阶段约10Mbps。
- BA job **107066929161**：FAIL。
  - before与after都CAPACITY_LIMITED。
  - after C2S pre/stress/post约4.288/2.879/3.142Mbps；S2C约7.093/6.186/6.087Mbps。
  - after server/client AF_PACKET drops约543065/77916。
  - before CPU client/server 117.72/115.27s；after 118.53/115.30s。
  - before handler≈199.7us/read；after≈204.8us/read。
  - 删除post-Sendto clone在同一runner内没有形成此前跨runner观察到的2x CPU下降。

结论：该clone确实是无语义价值的重复整包复制，保留删除合理；但不能把之前seed631跨runner的CPU减半归因于它。主要未解决问题是Actions runner之间出现约10倍handler单包耗时、约2倍进程CPU时间的容量分层；慢runner上AF_PACKET随后溢出。由于还没有该慢runner的per-core/PSI/cpu-model compact evidence，本轮不把它简化为“机器不行”，也不降低验收目标。

### Game4

同run Game4 logical3 job **107066929276** SUCCESS，artifact **10735335571**，digest `sha256:b83b0c36794490a9c717172a67498ab955a28010803b6387dfa20662baae5d89`：
- 四lane、每方向合计逻辑3Mbps、FEC20:20、lossless；
- 五类classification全PASS；
- outer/app C2S **20.62515x**、S2C **21.148997x**；
- C2S app raw input 44,999,920B，outer 928,130,258B；
- C2S FEC parity 576,979,518B，Game replication extra 163,184,560B；
- repair=0、padding=0、reconnect=0。
高倍率继续由四lane复制+FEC块最大shard补齐/fragment framing解释，不是transport重传。

## final Linux race

当前head `25a0ad7aea674dee6ccb9e7b3d1c0d61170af1d9` 的 foundation run **35825736077**：
- repository contract、普通active unit、Windows、iptables/nft、OpenWrt privileged均PASS；
- Linux Race FAIL，是真实race而非timeout。

race detector：
- writer: `LifecycleServer.admit` lifecycle.go:1383，queued steady record被认证后直接 `fresh.qualified = true`；
- reader: `LifecycleServer.groupReadyLocked` lifecycle.go:1702，通过 `TunnelQualified` 在 `s.mu` 下读 `lane.qualified`。

最小修复已经准备：queued segment只设置局部 `steadyQualified=true`，全部pending处理完后继续调用现有 `markLaneQualified`；后者在 `s.mu` 下验证当前lane并发布 `lane.qualified=true`。不更改建连、PeerFIN休眠、保活/业务空闲、lease/generation、FEC/repair、4096、wire或padding。因触及生命周期共享状态，提交后必须跑完整36份 lifecycle fullstack。

## strict matrix执行门

现有 `next-strict-weaknet` 会在每次 runtime/lifecycle source push 自动启动18份矩阵，这与WEAKNET_QUALIFICATION §9.4“无损目标仍不稳时不要重复大矩阵”冲突。本轮把push触发收敛为显式 `docs/qualification/STRICT_WEAKNET_TRIGGER`，保留workflow_dispatch。只有Normal/Game无损target稳定后才修改该trigger启动18份正式矩阵。

## 下一步

1. 提交上述strict显式门；
2. 提交LifecycleServer qualification发布race修复，跑core/race + 36份功能验收；
3. 性能继续一次一个原因：审计确认runtimeowner fresh/repair path仍把已owned immutable `pendingRecord.payload` 再clone进Segment，随后MarshalSegment又复制到raw packet。下一候选删除Segment这一层重复clone，但必须先用core/race证明ownership与同Seq同密文，再跑Normal10 compact和同runner A/B+B/A；仍不得扩大4096、buffer或降低目标。
