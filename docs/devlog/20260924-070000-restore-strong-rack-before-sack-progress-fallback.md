# 20260924-070000 恢复strong RACK优先，SACK-progress仅作fallback

## V4 correctness FAIL

V4 SOURCE_SHA `60c5f0ab7ee5ebe77371985be87be2e0c893eea9` 没有启动任何性能样本。

`next-runtimeowner-recovery` run `35936037992` 在unit阶段FAIL。主要失败均指向同一个回归：

- 原有 selective-ACK strong fast repair 不再发出；
- repair-budget测试无法进入repair/defer路径；
- deep-hole 1023 SACKed records不再立即fast repair；
- sequence-wrap strong SACK测试不再repair；
- transmit-time RACK测试在后续发送时间证据达到门槛后仍未repair。

原因是V4把 `firstRepairEvidence >= 3` 放在strong-RACK判断之前。于是即使一个ACK已经同时证明>=3条后继shadow成功到达，并且transmit-time separation已经超过现有RACK reordering window，也会因为“只收到1次SACK-progress事件”而拒绝repair。这错误改变了原有可靠strong evidence路径。

该FAIL永久保留，不通过改测试掩盖，也不为V4发性能样本。

## V5修正

首次repair重新分成两个有明确优先级的证据路径：

### 1. Strong RACK immediate（原语义）

若：
- candidate是当前累计head；
- candidate从未重传；
- 当前pending中仍有>=3条SACKed后继shadow；
- newest delivered/SACKed transmit time相对head达到现有RACK reordering window；

则立即沿原路径fast repair。

这保留此前为lossless reordering验证过的transmit-time RACK语义。

### 2. SACK-progress fallback（新增）

只有strong RACK不成立时，才看V4新增的bounded scoreboard evidence：

- 同一cumulative ACK；
- 三次ACK分别带来真正novel的SACK range进展；
- 重复相同SACK不计；
- ACK前进清零；
- 达到3后只arm唯一head；
- 必须继续跨一个完整recovery调度周期才发一次first repair。

这个fallback专门解决高吞吐约一个RTT内后继payload shadow已经被淘汰、`sackedOutstanding`无法同时达到3的问题。

## 其它约束不变

- current cumulative head pre-SACK保护：1条/lane
- 4096 shadow总上限
- O(1) ordinary eviction / max scan目标1
- fresh永不等repair；必要时FreshWindowBypass
- 1s InitialRTO / 3s absolute horizon
- fresh/5 repair credit / 128KiB burst / progressive repeat cost
- repeated repair继续用原RACK/RTO
- FEC/wire/loss threshold不变

两个fallback专项测试也改成真正没有strong transmit-time separation的场景，并要求三次novel SACK progress；full-window armed测试同样使用三次progress。

## 当前状态

V5提交创建时 correctness/race/performance NOT_RUN。V4 FAIL、V3/V2/V1诊断样本、历史689dea19 final18/P6全部保留。V5 Actions全绿后才能重新开始lossless/5205独立对比。
