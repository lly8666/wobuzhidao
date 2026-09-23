# 20260924-030000 有界 head repair 跟进候选

## 背景

历史固定 SOURCE_SHA `689dea19e2e1dfdccbab9c8f42210d62c4a12223` 的 loss-tolerant-v1 final18 已完成 18/18 PASS，并在同一SHA完成P6重新打包；这些证据全部继续保留。

复核 final18 与当前 runtimeowner 后发现一个独立于旧 O(N) scan/HOL 的行为问题：高吞吐、高RTT与持续loss下，真实 shadow retransmission 几乎消失。4096 shadow metadata大约只覆盖一个RTT，RTO最低1s；首次fast repair又需要较强发送时间RACK evidence。高loss时大量optional repair state先按fresh-first规则淘汰，于是 `Abandoned` 很高、`RTORepairs` 近零、真实repair极少。

本轮不回退fresh-first，不追求可靠TCP，不扩4096，不改FEC、wire、loss门槛、1s InitialRTO或3s repair horizon。

## 设计

只增加一个每lane最多一条的 armed cumulative-head repair：

1. 首次repair仍必须是当前累计ACK head，且至少有3条后继SACK证据。
2. 若已有现行的强 transmit-time RACK evidence，仍立即fast repair。
3. 若SACK证据足够但强RACK时间证据不足，则只把这一条head标为armed，不立即发送。
4. armed head从普通optional fresh-eviction链移出；expiry和3s horizon仍然生效。
5. armed head必须持续跨过一个完整recovery调度周期才允许一次fast repair。若期间累计ACK补洞，立即取消armed状态。
6. 只保护一个head candidate；其它repair state继续走现有O(1) retired/evict/expiry索引。窗口满仍优先fresh，极端保护态继续 `FreshWindowBypass`。
7. repair credit不变：fresh/5、128KiB burst、重复repair渐进虚拟成本、credit不足继续defer；重复repair继续使用当前保守RACK/RTO。

目标是恢复少量、有限、能实际发生的TCP-like shadow repair，而不是提高可靠性门槛。高loss允许业务损失的产品原则不变。

## 新增观测

`TransportStats` 增加 `FastRepairArmed`、`FastRepairArmFired`、`FastRepairArmCanceled`。结合现有 `FastRepairs`、`RTORepairs`、`RepairEvicted`、`Abandoned`、`ForgivenGaps`、`FreshBlocked`、`FreshWindowBypass` 与 timing/scan 指标，可判断repair机会是否恢复且没有拖累fresh。

## 新增回归

- SACK head在强RACK证据不足时先armed，跨过完整recovery周期后只发一次原密文fast repair。
- transient reorder若累计ACK先补洞，armed必须取消且后续tick不发repair。
- 4096满窗下armed head继续保留，fresh O(1)淘汰其它optional state，`FreshBlocked=0` 且 `RepairEvictionMaxScan<=1`。

已有strong-RACK、repair credit、RTO、full-window、Emit/ACK/Close并发、all-protected bypass和gap retirement测试全部保留。

## 资格状态

本日志所在提交创建时：
- compile/unit/race：NOT_RUN，只允许GitHub Actions
- performance：NOT_RUN
- 历史 `689dea19...` final18/P6：保留，不覆盖本候选
- P4/P5：重新打开
- P6：对当前HEAD stale
- P7：NOT_RUN

正确性全绿后，性能对比继续严格执行 `ONE_WORKFLOW_RUN_ONE_SAMPLE`，先独立Normal lossless、5205、5305；不通过改测试参数、降门槛或覆盖失败结果取得结论。
