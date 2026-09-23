# 20260923-111000 服务端资格计数热路径修复

## 本轮目标和阶段

对应 `STATUS.workstreams.PERFORMANCE_RECOVERY` 与 `docs/WEAKNET_QUALIFICATION.md` 第9.2节。输入为观测提交 `7ac4f2236c1fa0efbdb8032e7613bbef1a510cf5`；本轮只修一个已被定向样本证据支持的同步 handler 热路径浪费，不改生命周期、队列所有权、FEC、Game、MTU、4096 边界或 tls-startup-padding。

## 定向 Actions 事实

- `next-performance-recovery` run **35812290504** overall SUCCESS。
- core/race job **107026270927** PASS。
- Normal1 lane、FEC20:20、每方向10Mbps、lossless job **107026857488** 完成原始采集；artifact **10730701490**，digest `sha256:e57f6a7bbff044cba66338123cc9cfdbb2369969d4d699688c246a4cb1f4292d`。
- validator 仍为 `CORRECTNESS=PASS`、`CAPTURE=PASS`、`INPUT_VALIDITY=PASS`、`ENVIRONMENT=FAIL`、`PERFORMANCE=CAPACITY_LIMITED`，没有把采集成功包装成性能通过。
- lossless C2S goodput pre/stress/post 约 **0.335/0.232/0.238 Mbps**；S2C约 **4.146/3.750/3.719 Mbps**。server AF_PACKET `ss_packet` max drops **677365**，rmem/rb 约 **1.0021**；server/client UDP socket drops 约 **180704/129238**。
- 本样本 outer IP/app raw input 放大为 C2S **2.80065x**、S2C **1.94015x**，与历史代表样本5.08x不同，继续证明放大必须逐样本/逐方向记账，不能预设为某一类错误。

## 时间线与最早阻塞点

最终 server diagnostic 共读到 **61162** 个包：
- handler total **116.130660743s**，均值约 **1.899ms/包**，max **18.47ms**；
- reader->readCh handoff block total **118.616262500s**，均值约 **1.939ms/包**；
- queue age total **216.154205835s**，均值约 **3.534ms/包**；
- downstream delivery total **14.100404668s** / 15689 batches，约 **0.899ms/batch**；
- transport lock wait **6.710915008s**、常用payload临界区持锁 **4.427016675s**、owner decode **5.093269103s**、transport delivery **14.121057039s**；
- Lane record/FEC/LINK inbound decode **1.281958035s** / 45103 payloads，约 **0.028ms/payload**。

把已细分阶段从 handler 总耗时扣除后仍有约 **85.78s** 未解释。代码审计确认 `HandleServerSegmentQualified` 在 detached/RouteRecord 两条稳态路径上，每个包都在 `HandleSegment` 前后调用完整 `TransportStats`；`statsSnapshotAt` 会在 transport mutex 下扫描 `pending` 与 `received`。本样本 `PeakOutstanding=4096`，因此这个资格判断把本应 O(1) 的“AuthenticatedRecords 是否增加”变成每包两次 O(4096) 扫描，位置和未解释 handler 时间完全吻合。

runner 同时确实繁忙：120s窗口4核总 busy约 **99.08%**、softirq约 **2.39%**、steal **0%**；server进程约 **111.05 CPU-s**（约0.93核），client约130.2 CPU-s。服务端最终 `total_alloc≈7.73GB`、`NumGC=846`、GC pause total约350ms；`raw_linux.ReadSegment` 的64KiB临时分配仍是下一候选，但本轮不同时修改第二原因。

## 修改

- 在 `runtimeowner.Runtime` 增加只读取当前 transport `stats.AuthenticatedRecords` 的 O(1) 计数 accessor；仍使用已有 transport mutex，保持所有权和竞态边界。
- `HandleServerSegmentQualified` 两条资格路径改用该 accessor 前后比较，不再为了一个计数构造完整 `TransportStats` 快照。
- 不改变 qualification 语义：只有实际 authenticated record 计数增加才返回 true；ACK-only、坏record、重复/无新认证记录仍不触发 qualified。
- 保留 7ac4 的 bounded timing 诊断用于修复后同口径验证。

## 验证计划

本提交 push 后先由 `next-performance-recovery` 跑相关 core/race，再跑同一独占 Normal10 lossless。比较 7ac4 baseline 的 handler/readCh/queue age、AF_PACKET drops、goodput、server CPU与单位有效MiB CPU；性能仍崩则不跑18份长矩阵，继续缩小到 raw-read allocation/GC 或下一明确热点。只有 Normal10 明显改善后才进入 Game4逻辑3Mbps和线上字节账本。

生命周期/队列所有权本轮未改变，因此不触发36份功能验收；已有36/36 COMPLETE语义保持，尤其客户端主导休眠、server等待所有当前权威lane PeerFIN、黑洞恢复和稳定lease均不改。
