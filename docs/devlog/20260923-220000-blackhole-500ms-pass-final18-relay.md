# 20260923-220000 500ms shared blackhole PASS；建立同SHA最终18份中继

## 500ms共享黑洞

SOURCE_SHA `e5e12558fe6b2b32c1dbb07060450be811a099eb`，run `35856226845` / job `107165229100`：

- Game4，logical 3Mbps/方向，FEC20:20，300ms单向；
- 双向共享100% loss黑洞实际 `500.005742ms`；
- `shared-blackhole-v1` 五类 classification 全PASS，errors全空；
- expected-resume wall point之后 c2s/s2c first data delay均 `8.232573ms`；
- 首个完整1s >=99%目标wall-goodput窗口：c2s `3.00608Mbps`，s2c `3.04Mbps`；
- near-blackhole最长零交付双向 `510ms`；
- recovery bound `3000ms`；
- socket/link drop=0，10s drain late bytes=0；
- compact artifact `10748245980`，digest `sha256:494d7d5ed7c95b205c384cd09379d7056e89d3ecf107807aaf7f058700e451ef`；
- full artifact `10747731837`，929453744 bytes，digest `sha256:2ad0fe5af1f5bf4f79748c45cd516ea65e3b0fbe792652c95674df90ea86a094`。

100ms与500ms共享短黑洞专项均通过。

## 最终18份为什么换dispatch方式

规范要求最终同一SOURCE_SHA完成Normal/Game × lossless/5205/5305 × 3个seed，共18个独立Action run。此前单request relay每改一次请求都会产生新commit SHA，不适合最终闭环。

## 新资格中继

新增 `.github/workflows/next-performance-final-qualification-relay.yml`：

- 本身不执行measurement；
- push attempt1只校验固定18份计划并ARM；
- 每次connector rerun同一relay job时，确认branch HEAD仍等于它的 `GITHUB_SHA`；
- 查询同SHA已有strict workflow_dispatch runs；
- 按固定计划只dispatch第一条缺失样本，然后退出；
- `next-strict-weaknet` 新增稳定run-name（mode/scenario/seed/rate/lanes），供同SHA精确去重；
- 没有matrix measurement，没有同run多样本。

固定seed沿用历史同拓扑资格的 `101 / 202 / 303`。

## 执行策略

先跑未覆盖的两个Game弱网canary：
1. Game4 / 5205 / seed101；
2. Game4 / 5305 / seed101。

二者通过后补齐剩余16份。18份全部结束前不推进branch HEAD。

18份满足后，再实现并执行两配置各一次 >=30分钟目标速率长测（持续5% loss、周期20%阶段、至少一次rotation）；旧低频HTTPS soak不能代替。
