# 20260922-203000 strict generation 2 启动

## 固定窗口发生器 fast 验证

父 SOURCE_SHA：
`9192be4c28582092f6e6fe01536635d5e1b0d078`

Actions：
- realpath: https://github.com/lly8666/wobuzhidao/actions/runs/35729424399 — PASS
- targeted: https://github.com/lly8666/wobuzhidao/actions/runs/35729424568 — PASS
- fast foundation: https://github.com/lly8666/wobuzhidao/actions/runs/35729424630 — PASS

realpath artifact：
- ID 10694817621
- zip sha256 `12a4b7a267e6d5eb9377c4f56ee58af9e7d7be20f275927ae27178d8a9e50d62`

realpath发生器原始输出确认两方向：
- sent 987/987 packets，500080B/500080B；
- skipped_slots=0，skipped_bytes=0；
- send_failures=0；
- send_lag_p99_ns=2472 / 2630；
- unique delivery 987/987；
- outer repair=0；
- harness=success，validator=success。

因此“>10ms slot显式skip且不追赶”不会破坏正常低速校准路径。

## generation 1 永久作废

首次strict run：
https://github.com/lly8666/wobuzhidao/actions/runs/35728934986

状态固定为 **HARNESS_INVALID**。旧发生器允许积压catch-up并可能延长120秒窗口；该run无论最终job显示success/failure，其业务数字都不得用于主资格，也不得和generation2拼接选优。

## 本提交

恢复 `next-strict-weaknet.yml` 的严格paths限定push触发，并将
`docs/qualification/STRICT_WEAKNET_TRIGGER` 从 generation 1 提升为 generation 2。

本提交不改产品数据路径、不改FEC/repair/padding/MTU/速率/门槛；其唯一目的为在fast验证通过后准确启动一轮新的18样本。

generation2必须满足：
- 18个独立job/runner；
- Normal1 10Mbps/向、Game4 3Mbps/向；
- FEC20:20、padding off、MTU1400、300ms/向；
- lossless/5-20-5/5-30-5 × seeds 101/202/303；
- 固定120s 30/60/30 + 10s drain；
- >10ms迟到slot只记skipped，不允许catch-up；
- 一个job一个负载。

## 下一步

新strict run出现后，先检查：
1. matrix确为18个独立job；
2. 首批lossless/损伤样本的负载步骤在固定时限内结束；
3. artifact含manifest/stage-events/resources/client/server diag/四点pcap；
4. validator分开输出CORRECTNESS/INPUT_VALIDITY/PERFORMANCE/ENVIRONMENT/CAPTURE；
5. 任何INPUT_INVALID/CAPACITY_LIMITED/FAIL均保留原结果，不降低标准。

18主测即使aggregate PASS，也仍不关闭补丁A/B、专项损伤、带宽受限专项、目标速率30min长测、最终模块回归/打包和物理P7。
