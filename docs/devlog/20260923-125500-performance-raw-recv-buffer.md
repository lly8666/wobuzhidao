# 20260923-125500 AF_PACKET receive scratch 复用

## 本轮目标和阶段

对应 `STATUS.workstreams.PERFORMANCE_RECOVERY` 与 `docs/WEAKNET_QUALIFICATION.md` 第9.2节。上一原子修复已证明同步 qualification 全量统计扫描是实质瓶颈，但 Normal10 仍未达目标；本轮只修下一项确定的 allocation/GC 浪费，不改变队列容量、socket buffer、FEC档位、Game副本、注入速率、生命周期或 wire。

## 上一修复最终 Actions 证据

- SOURCE_SHA `7550844af73ca07481a932d17d6793ccb23ae05a`。
- `next-performance-recovery` run **35815797772** overall SUCCESS。
- core/race job **107036930642** PASS。
- Normal1 lane、FEC20:20、每方向10Mbps、lossless job **107037161702** 采集成功；artifact **10731592515**，digest `sha256:3d1891a9874d6fe1002e13b8f58a7168cbcc014b6c262e12a20454174bfb2754`。
- validator仍为 `CORRECTNESS=PASS`、`CAPTURE=PASS`、`INPUT_VALIDITY=PASS`、`ENVIRONMENT=FAIL`、`PERFORMANCE=CAPACITY_LIMITED`；没有把workflow成功等同性能通过。

同seed与观测基线 `7ac4f223` 比较：server reads **61162 -> 269830**（4.41x），handler平均 **1.899ms -> 0.359ms**，readCh handoff平均 **1.939ms -> 0.382ms**，queue age平均 **3.534ms -> 0.687ms**。C2S goodput pre/stress/post从约 **0.335/0.232/0.238 Mbps** 提升到 **1.794/1.611/1.672 Mbps**；S2C从 **4.146/3.750/3.719 Mbps** 提升到 **6.000/5.953/5.901 Mbps**。server AF_PACKET drops **677365 -> 631423**，仍是明确失败边界。

该修复让更多包进入产品进程后，server `TotalAlloc` 增至约 **25.7GB**、`NumGC=2721`、GC pause total约1.26s；这不是把runner直接归因成瓶颈，而是暴露下一处热路径分配。

## 代码与所有权审计

`internal/faketcp/raw_linux.go:ReadSegment` 在每一次调用开头执行 `make([]byte, 65536+64)`，然后 `ParseIPv4TCP` 把 `Segment.Payload` 作为输入packet的borrowed view返回；函数同时再 `append` 一份实际IPv4 packet作为第二返回值。Linux正式client/server均写成 `seg, _, err := raw.ReadSegment()`，第二raw返回值不被消费。

因此不能简单把同一64KiB buffer循环复用后直接返回原Segment，否则下一次 `Recvfrom` 会覆盖仍在handler中使用的 `Segment.Payload`。安全的最小所有权修复是：
- endpoint持有一个 `recvBuf` mutable scratch，并由 `recvMu` 串行拥有；正式路径本来只有一个reader，因此不新增并行瓶颈。
- AF_PACKET先读入scratch并完成frame/IP/TCP过滤；只有真正匹配localIP的包才复制到实际IP packet长度的owned backing。
- 从owned backing重新解析Segment，使返回的 `Segment.Payload` alias owned，而绝不alias可复用scratch。
- 保留ReadSegment原API及第二个owned packet返回值；官方入口即便忽略第二返回值，Segment.Payload仍引用该owned backing，因此没有生命周期悬空。
- 新增unit测试：构造合法IPv4/TCP packet，经owned-transfer后破坏原scratch字节，验证返回payload保持不变。

按755样本269830次reads粗算，单独取消每次约64KiB scratch allocation可避免约 **17.7GB** 的累计临时分配压力；这是理论分配项，不预先等同为同等RSS/CPU收益，实际收益以Actions TotalAlloc/GC/throughput为准。

## 中间提交说明

实现主体先落在 `6c4daa15fef91c74f70d88e4d1a142a6bcb324f8`，仅用于接续编辑；本日志、ownership unit test和正式状态将在其后同一最终候选提交收口。中间SHA触发的Actions不作为最终资格证据。

## 验证与下一步

最终候选push后由 `next-performance-recovery` 自动先跑相关core/race，再跑独占Normal10 lossless。重点对比755的reads、handler/readCh、AF_PACKET drops、C2S/S2C goodput、server CPU、TotalAlloc、NumGC与GC pause。若Normal10仍明显低于10Mbps，保持 `CAPACITY_LIMITED`，不运行18份长矩阵；依据新时间线继续下一单一根因。只有Normal无损容量继续实质改善后再进入Game4逻辑每方向合计3Mbps和逐方向source/parity/fragment/Game/repair/ACK/padding字节账本。

生命周期与队列所有权未变，因此本轮不重跑36份功能验收；客户端主导休眠、server等待全部当前权威lane PeerFIN、保活不等于业务活跃、黑洞恢复与稳定lease语义均未触碰。
