# 20260924-050000 SACK返回前保留唯一 cumulative-head shadow

## V2实测

V2 SOURCE_SHA `ab29a618fc32aba7aa6cd1a4d714bfb6649d15c1`：

- next-runtimeowner-recovery `35907662097` PASS
- next-p4-steady-targeted `35907662131` PASS
- next-foundation `35907661956` PASS
- next-lifecycle `35907662071` PASS
- Normal/lossless/seed101 `35908065363`：五分类PASS，socket/link drop=0，FastRepairs=0
- Normal/5205/seed101 `35931570578`：五分类PASS，socket/link drop=0

5205 compact artifact `10781551178` / full artifact `10781516462`。相较历史 `689dea19...` 和V1 `18a8b044...`，5% pre/post repair略有增加，但20% stress两方向 `FastRepairs=0`，目标仍未达到。

## V2只读repair lifecycle证据

main只读reader run `35932048026` / artifact `10781940276`，不属于性能样本。

20% stress：

- client: FastRepairArmed=0, Fired=0, Canceled=0, RepairEvicted=77264, SACKed=317544, FreshBlocked=0
- server: FastRepairArmed=0, Fired=0, Canceled=0, RepairEvicted=78045, SACKed=316814, FreshBlocked=0
- repair credit在阶段末仍有额度；RepairDeferred=0；不存在arm后因credit不足或tick未运行而未发的问题
- RecoveryTicks约600/60s，调度持续存在

所以V2失败点发生在arm之前：高吞吐+约600ms RTT下，4096 shadow window持续满，当前累计head在SACK反馈返回前就作为普通optional repair被O(1) fresh eviction淘汰。之后即使收到大量SACK，`pendingAtHeadLocked()` 已不再对应 `lastAck`，无法建立first-repair candidate。

## V3最小修正

不扩大4096，不改变fresh/5 credit、128KiB burst、1s InitialRTO、3s horizon、FEC、wire或loss门槛。

唯一新增规则：

- 当前累计ACK head（`seq == lastAck`）从进入shadow窗口起就不加入普通optional eviction链。
- 每lane最多保护这一条；后面的最多4095条业务shadow仍按现有intrusive list O(1) eviction。
- cumulative ACK前进后，立即把新的pending head从evict链摘出；旧head已由ACK正常移除。
- armed head规则继续存在：>=3 SACK可arm，强transmit-time RACK仍可立即repair，否则缺口跨完整recovery周期后只发一次first repair。
- 重复repair、RTO、credit、expiry完全不放宽。
- fresh仍不等待；如果没有普通可淘汰项，继续走既有 `FreshWindowBypass`。

这不是把4096重新变成发送窗口：只牺牲一个optional shadow槽，换取当前TCP-like cumulative hole在一个RTT反馈时间内仍有一份可修复密文。

## 回归新增/强化

- 新增pre-SACK full-window压力测试：4096满后继续发送1024 fresh，无ACK/SACK期间current head必须仍存在且不在evict链；`FreshBlocked=0`、`RepairEvictionMaxScan<=1`。随后最近4条SACK返回，必须能arm并经持久化周期发一次repair。
- 原full-window constant-work测试增加current head保留断言；原O(1) eviction调用/scan约束不放宽。
- transient reorder cancel、strong RACK、repair credit、RTO、in-flight repair并发和all-protected bypass测试继续保留。

## 当前状态

本V3提交创建时correctness/race/performance均NOT_RUN。V2及历史所有FAIL/PASS/CAPACITY_LIMITED artifacts永久保留。Actions全绿后重新从V3 exact SHA独立跑lossless、5205；5205确认stress产生有限非零repair且fresh/drop/五分类不退化后，才继续5305。
