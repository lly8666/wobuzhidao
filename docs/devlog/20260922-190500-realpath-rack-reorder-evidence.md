# 20260922-190500 realpath RACK 重排证据修复

## 当前 exact-SHA 证据

SOURCE_SHA / HARNESS_SHA:
`751bf4cf1eb3eb356b3531958e5d3bc3ff44c697`

Actions:
https://github.com/lly8666/wobuzhidao/actions/runs/35721326780

artifact:
- ID 10692130019
- zip sha256 `c76dd2cfa90522ddca6d24cd6142def7a8eb4e8d5e6d7ed7380bfadbaf02e223`

校准业务结果已经完整：
- C2S: 987/987 packet，500080/500080B unique delivered，corrupt=0，duplicate=0。
- S2C: 987/987 packet，500080/500080B unique delivered，corrupt=0，duplicate=0。
- probe: 8/8，RTT p50 600.388ms，p95/p99 600.929ms。
- netem: C2S/S2C qdisc delay p50均约300.011ms，p95约300.030/300.027ms。
- 四个capture点 dropped=0；qdisc drops=0、backlog=0。
- 正式client/server在完整drain后均alive，harness step SUCCESS。
- validator唯一FAIL原因：0% loss下 outer repair C2S=7500B、S2C=2137B。

因此真实路径已经打通，但“无损仍付repair成本”不能被workflow绿色或完整交付掩盖。

## pcap时间线：首个C2S误修复

以 C2S seq=2315460976 len=1250 为例，使用artifact的 c2s-pre 与 s2c-post：
- 洞包首发：177?（pcap绝对epoch为 1790076365.638576s）。
- 约600.285ms后，发送端收到 cumulative ACK=2315460976，并携带 SACK [2315462226, 2315465976)。
- 该SACK覆盖的最新已发送record首次发送仅比洞包晚 **24us**；也就是说发送时间维度只存在几十微秒重排证据。
- 当前实现却用 `now.Sub(candidate.lastSent)`，看到的是约600ms墙钟年龄，远大于10ms reordering window，于是立刻fast repair。
- repair抓包时间比上述SACK晚约38us；原洞包随后使累计ACK推进，覆盖ACK已在repair抓包前约17us出现。该repair没有独立业务收益，是微重排上的冗余线上成本。

其他repair同样集中在约600ms RTT返回边缘，且小于1s InitialRTO，因此不是RTO repair。

## 根因

`internal/runtimeowner/recovery.go::selectFastRepairLocked` 将RACK重排判断写成：
`now - candidate.lastSent >= reorderingWindow`。

但RACK类发送侧时间证据应比较“最新已交付/SACK的较新发送时间”和候选洞包的发送时间。高RTT本身不应把几微秒的发送序重排放大成几百毫秒的丢包证据。

当前代码已经维护 `rackLatestTx`，因此无需新增线程/队列/缓存，只需使用已有证据。

## 最小修复

- 新增 `rackEvidenceAgeLocked`：仅当 `rackLatestTx > candidate.lastSent` 时返回二者发送时间差。
- fresh fast repair 仍要求 `sackedOutstanding >= 3`，并额外要求发送时间证据差达到现有 `rackReorderingWindowLocked()`。
- 已重试候选仍保留原有RACK路径，但同样用发送时间差，不用墙钟年龄。
- 10ms最小window、SRTT/4、1s InitialRTO、3s repair horizon、repair budget、SACK数量均不改。

## 测试契约调整

不降低已有fast-repair覆盖：
- selective ACK fast repair：首包t0，后续包t0+16ms，仍应fast repair。
- deep-hole 1024索引：首包t0，后续t0+20ms，仍应fast repair并验证有界索引。
- sequence wrap：首包t0，后续t0+20ms，仍应跨wrap fast repair。
- 重写reordering regression：洞包t0，四个后续仅t0+3ms；即使SACK在600ms后返回也不得fast repair。再发送t0+12ms的新record并SACK后，发送时间证据超过10ms，应产生一次fast repair。

这直接覆盖本次realpath失败形态。

## CI与验收顺序

不在本地运行。提交后只看exact-SHA：
1. next-realpath-calibration；
2. next-p4-steady-targeted；
3. fast next-foundation。

旧31m低负载soak继续显式-only。无损校准严格PASS前不启动18主测；无损PASS后才进入18×120s，再在最后跑真正目标速率30min长测。
