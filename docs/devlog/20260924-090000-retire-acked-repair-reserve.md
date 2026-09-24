# 20260924-090000 ACK已覆盖reserve的有界持续清理

## V6.1测试结果

SOURCE_SHA `7081d9518fff791d0a69c7811e07bf773e3bfa28`（V6产品实现 + 测试编译修正）代码回归：

- next-runtimeowner-recovery `35940147998` PASS
- next-p4-steady-targeted `35940148006` PASS
- next-foundation `35940147996` PASS
- next-lifecycle `35940148024` PASS

### Normal/lossless/seed101

run `35940368181`，五分类全PASS，socket/link drop=0，FastRepairs=0。
summary `10784513653`，full `10784598368`。说明one-shot reserve没有在无损路径制造假重传。

### Normal/5205/seed101

run `35940798659`，五分类全PASS，socket/link drop=0。

20% stress:
- C2S FastRepairs = 23821
- S2C FastRepairs = 23609
- 两向 Abandoned = 0
- FreshBlocked = 0
- FreshWindowBypass = 0
- unique业务仍维持约10Mbps

main只读repair diag `35943158821/10785822437`：
- reserve peak client/server = 864/871，低于1024
- stress reserve repairs ≈ 23641/23609
- reserve无capacity eviction/drop/expiry

因此5205的“几乎没有retransmission”已被修复，并保持fresh no-HOL和资源有界。

成本也必须记录：相对历史qualified `689dea19...`，整场outer/app：
- C2S 5.23016x -> 5.47834x（约 +4.75%）
- S2C 5.36199x -> 5.61494x（约 +4.72%）

吞吐和probe RTT没有出现可归因改善；恢复TCP-like repair本身带来了约4.7%的额外outer成本。不能把它描述成性能加速。

### Normal/5305/seed101

run `35943316442`，五分类全PASS，socket/link drop=0。

30% stress:
- FastRepairs = 285/向（compact sender方向）
- Abandoned ≈ 116046 / 116449
- fresh仍接近10Mbps；高loss允许相应业务损失
- outer/app相对历史仅约 +1.76% / +1.72%

只读repair diag `35943686505/10786260610` 进一步显示：
- reserve最终peak = 1024
- client stress Stored=115601, Evicted=114006, Repairs=552, Outstanding≈995
- server stress Stored=115753, Evicted=114505, Repairs=285, Outstanding≈942
- credit充足、FreshBlocked=0、RecoveryTicks正常

## 根因

这次不应直接解释成“1024一定太小”。

`pruneRepairReserveACKLocked` 每次累计ACK前进只清理固定64条，这是有界设计；但 `retireSelectiveACKLocked` 对 duplicate ACK 直接返回。30% loss/pressure forgiveness 下，cumulative ACK会偶尔跨过大量reserve记录，一次64条清理不完。随后大量相同cumulative ACK的duplicate ACK不会继续清理。

结果是已经被 `lastAck` 覆盖、实际上确认成功的stale reserve仍占着1024槽。fresh压力下 `stashRepairReserveLocked` 看到reserve满，只会把FIFO head当optional record淘汰，于是这些本应记为Retired的记录被误记为Abandoned/RepairReserveEvicted；真正未来head可用的保留深度被显著压缩。

## V7最小修复

不增加active 4096，不增加reserve 1024，不改FEC/wire/RTO/repair credit/门槛。

1. cumulative ACK前进路径仍一次最多prune 64。
2. 若后续ACK与当前 `lastAck` 相同，duplicate ACK继续调用同一个固定64预算的reserve prune；stale ACK（小于lastAck）不做额外清理。
3. fresh把一个active shadow移入已满reserve时，如果FIFO head的 `end <= lastAck`，只O(1)将这条按ACK-confirmed record正常retire，再插入新shadow；不计Abandoned/RepairReserveEvicted。
4. 若reserve head仍未ACK，原有规则完全不变：current head/armed/in-flight不绕过、不扫描；其它optional head才真正capacity eviction。
5. 每条reserve record仍只会被移除一次，duplicate ACK每次工作量上限仍64；fresh路径最多额外O(1) retire一个head，因此不恢复O(N)扫描。

## 新增回归

- 建立128条reserve后，一个大累计ACK只按budget清理64；同一个duplicate ACK必须继续清掉剩余64，且RepairReserveEvicted/Abandoned保持0。
- 建立满1024 reserve，大ACK仅清64后，在没有下一次ACK前继续fresh；reserve再次满时必须O(1) retire一个已被lastAck覆盖的head，而不是误记capacity eviction。FreshBlocked=0、FreshWindowBypass=0、RepairEvictionMaxScan<=1。
- 原reserve one-shot repair、bounded no-HOL、strong RACK、lossless reorder、credit/RTO等测试全部保留。

## 当前状态

V7提交创建时 correctness/race/performance 均NOT_RUN。历史V1–V6.1所有FAIL/PASS/artifact永久保留。Actions全绿后，必须在V7 exact SHA重新独立跑lossless、5205、5305；不得复用V6.1样本。
