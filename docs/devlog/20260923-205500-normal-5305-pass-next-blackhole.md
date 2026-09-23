# 20260923-205500 Normal 5305 PASS，下一步共享短黑洞

## Normal 5305 有效资格

SOURCE_SHA `bcf6cb8c799392c12eed4330d2ac48ea7d668b77`，`next-strict-weaknet` run `35850699034` / job `107147426180`：

- Normal / 10Mbps each direction / 5→30→5 / seed601 / 1lane / FEC20:20；
- CORRECTNESS / INPUT_VALIDITY / CAPTURE / ENVIRONMENT / PERFORMANCE：全部 PASS；
- stress实际qdisc loss：c2s 30.0813%，s2c 29.9524%；
- stress业务最终packet loss：c2s 0.1081%，s2c 0.1723%；
- stress byte loss：0.1317% / 0.2008%；
- stress delay-aligned wall：9.98617 / 9.97928 Mbps；
- post5 offset=2s；
- probe timeout=0；
- socket/link drop=0；
- drain 10s late bytes=0；
- stress sender abandonment约117k/方向、receiver forgiveness约81k/方向，stress repair_segments=0，FEC recovered source约58k/方向；没有形成fresh HOL或本机overflow。

artifact：
- compact summary `10745103190`，digest `sha256:4a6e49ffb09647d7d6eb5af5f2c7d6931df6fece645750d7ea3a802561926c3e`；
- full `10745183084`，703356499 bytes，digest `sha256:036a2092f87adf7daf10b9436b1ad3b2cd7bd6651dd4fcc2814ff62198b2578a`。

## 当前阶段

恢复阶段已经有以下独立有效PASS：
- Normal lossless；
- Game4 lossless；
- Normal 5205独立复核（首个环境CAPACITY_LIMITED保留）；
- Normal 5305。

这些证据足以继续专项，但不能关闭最终性能资格。规范第3节/第4节仍要求最终 Normal/Game × 三场景 × 三seed 共18主测，并在最后每配置做>=30min目标速率长测。

## 共享黑洞专项入口

仓库目前没有合规的burst/blackhole性能入口；现有strict workflow只支持lossless/5205/5305。规范明确的专项是Game四lane共同100ms/500ms短黑洞，并要求正确性、连续性、有界恢复和资源为硬门，不套主随机loss的数值门槛。

下一原子任务新增one-run-one-sample专项：
- Game4 / 3Mbps each direction / FEC20:20 / 300ms one-way；
- 100ms和500ms分别独立run；
- 四lane共享同一underlay blackout；
- blackout期间允许业务失败/损失；恢复后fresh不得等待积压repair，必须在固定有界窗口恢复；
- event时刻、背景loss、恢复观察窗等新harness参数会显式写入manifest/devlog，作为本轮测试定义，不伪称历史规范已有。
