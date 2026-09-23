# 20260924-060000 首次repair改用新SACK进展证据

## V3结果

V3 SOURCE_SHA `a229489568827432781db0344b907d0ba2db8833`：

- next-runtimeowner-recovery `35932423623` PASS
- next-p4-steady-targeted `35932423616` PASS
- next-foundation `35932423659` PASS
- next-lifecycle `35932423640` PASS
- Normal/lossless/seed101 `35932693118`：五分类PASS、socket/link drop=0、FastRepairs=0
- Normal/5205/seed101 `35933149687`：五分类PASS、socket/link drop=0，但20% stress两方向FastRepairs仍为0

V3 5205 compact `10782341069` / full `10781887899`。只读repair reader run `35933560537` / artifact `10781643937`。

stress证据：
- client：FastRepairArmed=0、Fired=0、FastRepairs=0、SACKed=317549、SACKedOutstanding阶段末=3、RepairEvicted=77267、FreshBlocked=0
- server：FastRepairArmed=0、Fired=0、FastRepairs=0、SACKed=316613、SACKedOutstanding阶段末=0、RepairEvicted=78238、FreshBlocked=0
- RecoveryTicks约600/60s，repair credit充足

V3已经确保current cumulative head shadow不会在SACK返回前被普通fresh eviction淘汰。因此剩余问题不是head缺失，而是first-repair门槛仍写成 `sackedOutstanding >= 3`。这个值只统计“当前仍存在于4096 pending map中的SACKed record”。高吞吐一个RTT内后继shadow大量被淘汰；每次ACK决策点可能只有0–2条新SACKed shadow仍在pending，即使累计SACK进展非常大，也永远达不到门槛。

## V4设计

首次repair证据改成同一cumulative ACK上的“新SACK进展事件”，而不是同时留存的后继payload数量：

1. 每个ACK处理SACK blocks时，先用现有 sender SACK history 求真正未见过的range片段。
2. 该ACK只要包含至少一段新的SACK进展，就为当前未重传cumulative head增加1个evidence；一个ACK最多加1。
3. 重复完全相同的SACK block由于没有novel range，不增加evidence。
4. cumulative ACK一旦前进，evidence立即清零并绑定新head。
5. evidence最多3；达到3后：
   - 若现有transmit-time RACK强证据已成立，可沿原路径立即first repair；
   - 否则沿V1已有规则arm唯一head，并要求缺口跨一个完整recovery调度周期后才发一次repair。
6. current-head pre-SACK保护继续保留；其余最多4095条shadow仍可O(1) eviction。
7. 已经重传过的candidate仍使用原RACK时间门；RTO、3s horizon、fresh/5 credit、128KiB burst、渐进repeat cost均不变。

这更接近有限TCP-like duplicate/SACK-progress evidence：不要求所有后继可重传payload仍驻留，但也不会把一个重复ACK无限累计成repair资格。

## 回归

定向测试更新为：
- current head在满窗压力下保留；三次逐步扩展的SACK进展才能arm；重复第三个SACK不增加evidence；
- 同batch相同发送时间的persistent loss也必须由三次novel SACK progress建立arm并经持久化周期repair；
- transient reorder在arm后若累计ACK补洞仍必须取消，不发repair；
- strong RACK、repeat repair、credit、RTO、fresh no-HOL、eviction max scan<=1等既有测试全部继续。

## 当前状态

V4提交创建时 correctness/race/performance 均NOT_RUN。历史689dea19 final18/P6、V1/V2/V3所有独立样本及只读artifact永久保留。Actions全绿后，新SHA重新独立跑Normal lossless与5205；只有20% stress恢复有限非零repair且fresh/drop/五分类不退化，才继续5305。
