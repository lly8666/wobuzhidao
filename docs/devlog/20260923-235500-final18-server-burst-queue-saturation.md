# 20260923-235500 final18暂停：server 256槽burst queue在两个5305样本中触边

## final18结果

固定SOURCE_SHA `148b2b0cca472d69b1da6b8e6075f229a31b3e11` 的18份独立 `workflow_dispatch` strict样本已经全部完成。逐份读取 `loss-tolerant-v1` compact summary 后：16份五分类全PASS，2份为 `ENVIRONMENT=FAIL` + `PERFORMANCE=CAPACITY_LIMITED`。CAPACITY_LIMITED不是PASS，因此该final18停止并永久保留，不择优覆盖、不重跑同一workflow run。

- Normal / 5305 / seed202：run `35877070378`，job `107235645879`，full artifact `10759231301`。CORRECTNESS/INPUT_VALIDITY/CAPTURE PASS；server `ss_packet` drops=1346，另有 `ss_udp` drops=25；client/link/qdisc为0；performance errors为空，post5通过。
- Game4 / 5305 / seed303：run `35877100569`，job `107235753927`，full artifact `10757968883`。CORRECTNESS/INPUT_VALIDITY/CAPTURE PASS；server `ss_packet` drops=204；client/link/qdisc为0；performance errors为空，post5通过。

## Normal5305/202只读诊断

main上的只读artifact readers不是性能样本。对应clientdrop/context/pipeline/ssraw runs为 `35882176326` / `35882176116` / `35882176151` / `35882176244`。

- AF_PACKET drop全部在stress：约83.048s一次增加1258，下一秒再增加88；第一点server packet socket已到 `1037376/1048576`。
- server pipeline在drop前已出现 `ready_current=257`、`ready_peak=258`，说明256槽handoff达到边界并让raw reader反压。
- 随后同一事件窗口内累计最大值跳到：handler `281.241374ms`、handoff_block `248.437983ms`、queue_age `294.722948ms`；downstream max仅 `5.768441ms`。
- nearest transport仍为 `FreshBlocked=0`、`FreshEmitFailures=0`、`FreshWindowBypass=0`、`RepairEvictionMaxScan=1`，不是旧repair全窗扫描/HOL复发。

该样本server全程最高相邻diagnostic读速约 `12330.999 reads/s`。按观测最大queue age 294.723ms计算，覆盖同量级停顿需要约3634槽；局部drop窗口约7.9k reads/s，对应约2326槽。

## Game5305/303独立确认

只读artifact readers runs为 `35882370399` / `35882370372` / `35882370404` / `35882371816`。

- drop发生在pre：drop前1秒server packet socket `640064/1048576`，下一采样累计204 drops并已排空。
- pipeline同样达到 `ready_peak=258`；drop附近 handoff_block max `10.164816ms`、queue_age max `48.433018ms`，handler max `7.234804ms`。
- 四lane仍全部 `FreshBlocked=0`、`FreshEmitFailures=0`、`RepairEvictionMaxScan=1`。
- 该样本全程最高相邻diagnostic读速约 `15269.005 reads/s`；48.433ms对应约740槽。

两份相互独立的5305结果共同证明：client SegmentMux不是当前问题，server 256槽bounded handoff在真实runner短停顿/突发下仍可触边。Normal样本还出现约281ms长调度/handler停顿；当前证据不足以安全重构handler并发模型，因此不做worker pool、每包goroutine或协议改造。

## 第二个最小有界修复

`serverReadQueueDepth` 从256调整为4096，Server与LifecycleServer继续共用同一固定队列。4096是由当前最坏观测 `12.33k reads/s × 294.723ms ≈ 3634` 向上取一个有界余量，不是盲目扩kernel buffer。队列满时仍阻塞reader，保持显式背压；不静默丢包。

同时qualification-only `ServerPipelineDiagnostic` 增加 `ready_capacity`，便于后续artifact直接把peak与固定边界对照。测试固定验证depth=4096、容量不可越界、释放一个slot后继续前进。

明确不变：kernel `SO_RCVBUF`、wire/protocol、FEC20:20、repair horizon/4096 shadow metadata、loss门槛、fresh优先、SegmentMux 256/route均不改。

## 验证顺序

1. 本提交先由Actions跑repository/targeted/foundation/lifecycle/fullstack及race相关回归；本地不编译不测试。
2. 回归全绿后，在本次产品精确SHA上分别跑且只跑一条 Normal/5305/seed202 和 Game4/5305/seed303 strict canary；每run一条性能样本。
3. 要求五分类全PASS，client/server AF_PACKET与link/qdisc drop均0，并确认server `ready_peak` 明显低于 `ready_capacity=4096`。
4. 两条canary稳定后，再把状态文档与final18 relay一次性落到新的固定final HEAD，并从头跑18份；`148b2b0` 的18份历史样本全部保留。
