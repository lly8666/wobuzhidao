# 20260923-104100 接收容量有界观测

## 本轮目标和阶段

对应 `STATUS.workstreams.PERFORMANCE_RECOVERY` 与 `docs/WEAKNET_QUALIFICATION.md` 第9节。开始分支 `next/tlslike-dataplane`，HEAD `cc75dd71e209a3490445727c37d6e7cee7d2cdfd`。本轮只增加资格诊断观测和单样本 Actions 入口，不改变 FEC 档位、Game 副本、4096 recovery、生命周期或队列容量。

## 修改与原因

- 审计确认服务端稳态路径为 AF_PACKET `ReadSegment` → 容量1 `readCh` → `HandleServerSegmentQualified` → runtime owner → Lane record/FEC/LINK → SharedTUNRouter。
- `runtimeentry` 在现有 `diagnostic-jsonl` 开启时累计 reader 相邻成功读取间隔、readCh 交接阻塞、ready 数量/字节峰值、队列年龄、handler 总耗时和下游交付耗时。全为有界累计/峰值，无逐包日志、无新队列。
- `runtimeowner` 增加 transport 锁等待/常用 payload 临界区持锁、owner decode 路径和最终 delivery 的累计/峰值时序；`datapath.Lane` 增加 record/FEC/LINK decode 与 Expire 的累计/峰值时序。诊断关闭时不调用 `time.Now` 热路径，只保留轻量开关读取。
- 新增 `next-performance-recovery`：先相关 core/race，再在单独 runner 上跑 Normal 1 lane、FEC20:20、每方向10Mbps、lossless 一份正式路径样本。validator 可继续把性能标为 CAPACITY_LIMITED；workflow 不把分类失败包装成性能通过。
- 代码审计同时发现两个待证据确认的热路径浪费：服务端每包 qualification 前后 `TransportStats` 会扫描 pending/received；Linux `ReadSegment` 每次调用分配约64KiB并保留/复制 packet backing。此提交不先修，避免无样本归因。

## 复用来源

无旧项目代码复用；未读取或迁入 `old/` 实现。仅在当前权威代码上增加观测。

## Actions证据

SOURCE_SHA：本日志所在观测提交（提交后回填精确 SHA）。`next-performance-recovery` 将由 push 触发；当前写入时 run/job/artifact 尚未产生，状态为 NOT_RUN。该 run 将先执行 targeted core/race，再采集 Normal10 lossless 原始 artifact。另有 `next-foundation` 按分支 push 规则运行，但不把其绿色结果替代性能门槛。

## 问题、排查与风险

基线仍为 `0b206a07f91513133a80a147656b637c286ce3e2` 的 18/18 `CAPACITY_LIMITED`。AF_PACKET drops 是已知边界而非根因结论。新增观测本身会有少量原子计数与计时开销，必须在后续修复前后 A/B 单列；没有扩大 socket/TUN/readCh 缓冲，也没有更改注入速率、FEC、Game、HOL 或 lifecycle 语义。

## 下一项原子任务

读取本次 Normal10 lossless 的 server pipeline、transport/Lane timing、GC、每核 CPU/softirq/steal、AF_PACKET skmem drops 与 PPS 时间线，确定最早阻塞阶段。只对证据支持的实现浪费做一个最小修复；无损仍崩则继续缩小诊断，不跑18份长矩阵。
