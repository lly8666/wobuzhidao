# 20260924-080000 recently-evicted one-shot repair reserve

## V5结果与剩余根因

V5 SOURCE_SHA `d8c79cd5cd97ed3867970aae15590acd966ef90e`：

- next-runtimeowner-recovery `35936257311` PASS
- next-p4-steady-targeted `35936257318` PASS
- next-foundation `35936257462` PASS
- next-lifecycle `35936257420` PASS
- Normal/lossless/seed101 `35936587721`：五分类PASS、socket/link drop=0、repair=0
- Normal/5205/seed101 `35937061116`：五分类PASS、socket/link drop=0，但20% stress FastRepairs=0

5205 summary `10783711143`，full `10783193967`。
main只读repair reader `35939424449` / artifact `10784138669`：

- client stress: FastRepairEvidence=0, Armed=0, Fired=0, FastRepairs=0, SACKed=311087, RepairEvicted=77188, credit=131072, RecoveryTicks=590
- server stress: FastRepairEvidence=0, Armed=0, Fired=0, FastRepairs=0, SACKed=316718, RepairEvicted=78145, credit=131072, RecoveryTicks=600
- FreshBlocked=0、FreshWindowBypass=0

V3/V5已经保护“此刻的lastAck head”，但高损receiver会在有界receive pressure下推进累计ACK到下一gap。那个“未来head”在变成lastAck之前仍是普通optional shadow，可能已经被4096 active window淘汰。因此ACK到来后exact head ciphertext不存在，后续SACK evidence无法绑定，V5 stress evidence仍为0。

## V6：recently-evicted one-shot reserve

不扩大active repair state。active仍严格4096，并继续作为SACK/RTO/repair scan集合。

新增独立1024条 recently-evicted business ciphertext reserve：

1. fresh在active 4096满时仍只做一次O(1) evict；被淘汰的普通业务shadow不立即丢payload，而是移入reserve。
2. reserve不计入active `Outstanding`，不进入RTO scan、repair linked list或fresh admission；因此不会重新制造fresh HOL或放大每tick扫描。
3. reserve insertion ordered、map按seq精确查找，容量固定1024。满时O(1)淘汰最旧reserve；若最旧reserve恰好是current head/armed/in-flight，则不扫描别处，直接放弃新进入reserve的optional shadow，fresh照常继续。
4. cumulative ACK推进时有界清理已覆盖reserve（单次最多64条）；tick也有界清理超过3s horizon的reserve。
5. 如果 `lastAck` exact record只存在于reserve，它可参与现有strong-RACK或SACK-progress first-repair判定；arm/persistence规则不变。
6. reserve record只允许一次fast repair。成功发送后立即从reserve删除，不进入repeat repair/RTO，不扩大长期可靠性。
7. fresh/5 repair credit、128KiB burst、原密文重发、FEC/wire/loss threshold全部不变。

### 大小依据

V5 Normal/5205 stress：
- fresh约394852 records / 60s = 6580.9 records/s
- active 4096只覆盖约622.4ms
- probe p99 RTT约623.9ms
- recovery tick约100ms

要让刚越过active retention的future head在反馈到达后仍跨一个完整recovery周期，需要额外约 `(623.9+100-622.4)ms * 6.58 records/ms ≈ 668` 条。取1024固定槽，额外约155.6ms，总保留包络约778ms。它是有界、证据驱动余量，不改变kernel/socket/FEC或测试参数。

## 统计语义

新增：
- RepairReserveStored
- RepairReserveRetired
- RepairReserveEvicted
- RepairReserveDropped
- RepairReserveExpired
- RepairReserveRepairs
- RepairReservePeak / Outstanding / Bytes

active→reserve不再立即记Abandoned；只有reserve最终被容量/期限真正丢弃时才记Abandoned/RepairEvicted。这样Abandoned重新表示“修复机会真正消失”，而不是“离开active集合”。

## 回归

新增：
- 4096满后seq1被fresh eviction进入reserve；累计ACK推进使seq1成为新head；三次novel SACK progress可在reserve上arm，经持久化周期发一次原密文repair。
- active+reserve持续压力下active始终4096、reserve始终<=1024、FreshBlocked=0、FreshWindowBypass=0、RepairEvictionMaxScan<=1。
- 原lossless reorder、strong-RACK、credit、RTO、in-flight Emit/ACK、all-protected bypass、gap index等测试继续。

## 当前状态

V6提交创建时 correctness/race/performance NOT_RUN。历史689dea19 final18/P6与V1–V5全部保留。Actions全绿后重新独立跑Normal lossless与5205；只有stress恢复有限非零reserve repair且fresh/drop/五分类不退化，才继续5305。
