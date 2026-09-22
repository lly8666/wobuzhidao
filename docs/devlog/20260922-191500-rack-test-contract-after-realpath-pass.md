# 20260922-191500 realpath PASS 后的 RACK 测试契约校正

## 当前通过的真实路径证据

SOURCE_SHA:
`466145a059967aac4649146b669e27e5c1309f53`

Harness blob SHA:
`4863a6a6715d40b3dc90c17507635b7978bea011`

Analyzer blob SHA:
`4989ec1e6c392799ac8599352196a49b782182e8`

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35723954047

Artifact:
- ID 10692079954
- zip sha256 `bbe47475e84925ad4ccb479eb863c301ab41175cac80554cf602e3a4e361ef67`

严格无损校准结果：
- C2S 987/987 packet，500080/500080B unique delivered。
- S2C 987/987 packet，500080/500080B unique delivered。
- send_failures=0，corrupt=0，duplicate=0。
- probe 8/8，RTT p50=600.630ms，p95/p99=601.318ms。
- qdisc delay p50=300.029ms/向，p95=300.107/300.106ms。
- 四点capture drop均0。
- outer repair payload C2S=0、S2C=0。
- outer IP/app input=5.222461406175012。该比例只是当前低速无损校准成本，不能冒充后续目标速率资格成本。
- harness=success，validator=success。

因此前两轮真实问题已闭合：shared-TUN IPv6上线早退、Linux raw EINTR早退、以及高RTT微重排触发的spurious fast repair均不再阻断无损真实闭环。

## 同SHA基础回归为何红

Targeted:
https://github.com/lly8666/wobuzhidao/actions/runs/35723953925

Foundation:
https://github.com/lly8666/wobuzhidao/actions/runs/35723954064

平台privileged、lifecycle、P2均PASS；Linux/Windows Go测试只失败以下3个runtimeowner测试：

1. `TestSteadyRepairBudgetDefersRepairButNeverFresh`
   - 仍把5条record全部标成同一发送时刻t0；
   - t0+20ms收到SACK后期待RepairDeferred；
   - 新实现正确地看到 transmit-time evidence=0，因此根本不选择fast repair，也就不会进入repair budget defer。

2. `TestSteadyIncrementalSACKHandlesSequenceWrapAndFourBlockCache`
   - 同样把洞包和后续SACKed记录全部设成同一发送时刻；
   - t0+50ms只是ACK墙钟延迟，不构成RACK发送时间证据。

3. `TestSteadyFreshFastRepairUsesTransmissionTimeReorderingEvidence`
   - 第一阶段正确证明：后续记录只比洞包晚3ms，即使SACK在600ms后到达也不得误repair；
   - 但SACK同时建立约596ms SRTT，现有 `rackReorderingWindowLocked` 因此约149ms；
   - 测试第二阶段只给12ms发送差，仍不足以触发fast repair。实现没有错，是测试错误地继续按10ms最低floor断言。

## 本轮改动

只修改测试，不改产品源码：

- repair-budget测试：首记录t0，后续4条t0+12ms；20ms时收到SACK，SRTT约8ms，window仍为10ms，发送时间证据12ms，进入repair选择后才由不足的credit正确触发RepairDeferred；fresh仍不受阻。
- sequence-wrap测试：首记录t0，后续4条t0+20ms；50ms SACK给出20ms发送时间证据，跨越约10ms动态window，继续覆盖wrap+fast-repair。
- 600ms RTT测试：第一阶段保持3ms微重排不修；首个SACK后动态window约149ms。随后在t0+700ms发送新record，t0+1300ms SACK它，形成明确大于window的发送时间证据，应产生一次fast repair。

不修改10ms最低window、SRTT/4、InitialRTO、RepairHorizon、repair credit、SACK数量、validator或任何生产配置。

## 下一步

该测试修复提交后只看：
1. next-realpath-calibration；
2. next-p4-steady-targeted；
3. fast next-foundation。

三者同SHA通过后，才正式进入18个120秒严格样本。旧31分钟低负载soak保持显式触发，不参与当前定位；最终仍需目标速率30分钟长测。
