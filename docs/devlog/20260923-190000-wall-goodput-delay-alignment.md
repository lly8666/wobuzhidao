# 20260923-190000 wall-goodput 固定路径延迟对齐

## 新证据

零采样 artifact-reader run `35844026766` / job `107125699186` 成功读取原始artifact `10742800172`。小型抽取artifact为 `10742238061`。

有效 Normal/lossless/seed601 run `35843250922` 的 `loss-tolerant-v1` 只有两个 performance errors：

- c2s/pre wall goodput = `9.899067733333332 Mbps`，门槛 `9.9 Mbps`；等价30秒窗口约少 `3496 B`；
- s2c/pre wall goodput = `9.899456 Mbps`，门槛 `9.9 Mbps`；等价30秒窗口约少 `2040 B`。

其它关键事实：

- 两方向每阶段 actual send 与 eventual unique goodput 均约10Mbps；
- packet loss=0，byte loss=0；
- lossless probe timeout=0；post drain 10s后late bytes=0；
- socket/link/capture环境均无drop；
- repair、abandon、gap-forgive、FreshBlocked、FreshWindowBypass、FreshEmitFailures全部0；
- PeakOutstanding约4062～4063，但最终Outstanding=0；
- GapIndexSteps全程仅1～2，未见接收端索引扫描放大。

因此当前证据不支持继续改bounded-recovery产品热路径。

## 统计定义问题

资格规范同时固定：

- 300ms单向路径延迟；
- pre/stress/post为30/60/30秒；
- lossless wall goodput >= 目标99%。

旧analyzer把接收wall窗口直接取 `[0,30s)`，而发送也从t=0才开始。即使产品零处理耗时，300ms纯传播也会让pre窗口最多只看到29.7秒的数据，恰好等于99%；任何几毫秒正常处理/调度都会数学上落到9.9Mbps以下。本样本的缺口正是几KB量级。

这不是降低门槛的理由，而是wall窗口坐标系错误。

## 修复

本提交保持10Mbps、99%、FEC20:20、300ms、30/60/30全部不变：

1. `realpath_udp_duplex.py` 额外记录10ms接收wall buckets，保留原1秒wall统计；
2. analyzer从manifest读取 `one_way_delay_ms=300`，保留原始 `wall_delivered_mbps` 作为报告值，同时增加 `wall_delivered_mbps_path_delay_aligned`；
3. gate仍是原来的 `target × (1-p) × 99%`，但接收窗口按固定路径延迟平移300ms，避免把确定性传播fill/drain算成产品吞吐损失；
4. analyzer仍标 `loss-tolerant-v1`，新增 `analysis_revision=path-delay-aligned-wall-v2`，并记录新Git blob `ee768e95c986203f9380a1bd95aa7d83baea803c`；
5. strict workflow新增小型summary artifact，后续无需下载数百MB主artifact即可读取分类；
6. 新增纯unit Actions工作流验证10ms bucket与delay-aligned窗口；不包含性能measurement。

旧run `35843250922` 的FAIL不追溯改绿。新定义必须在新的独立Normal lossless Action sample上验证。

## 历史放大对照

本次c2s outer/app约5.09199×、FEC parity约481.28MB；规范已记录旧Normal/lossless代表样本约5.07892×、parity 481.01MB。因此5×放大量级不是本轮bounded-recovery修改新引入，且本次正式路径已从历史大规模AF_PACKET drops改善为0 drop。线上放大仍需按既有9.3继续解释，但不是当前唯一FAIL的触发条件。
