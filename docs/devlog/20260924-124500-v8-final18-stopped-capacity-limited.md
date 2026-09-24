# 20260924-124500 V8 final18 stopped: 16 PASS / 2 CAPACITY_LIMITED

## 结论

产品 SOURCE_SHA 仍为 `27e4bb34ae35e4c48fc4ebaa04ba3e3d161b4089`（`runtimeowner: retire reserve only under pressure`）。本轮没有修改产品代码、FEC、wire、RTO、repair credit、active4096、reserve1024、kernel socket buffer或任何性能阈值。

专用 fixed ref `perf-fixed/27e4bb34ae35e4c48fc4ebaa04ba3e3d161b4089-r2` 已由 coordinator run `35955278581` 派发完整 18 条**全新** strict `workflow_dispatch`；每条 attempt=1、一个run一个样本，诊断/canary均未复用，也没有rerun任何strict性能run。

结果必须记录为：

- PASS = 16
- CAPACITY_LIMITED = 2
- FAIL = 0
- INVALID = 0

因此 **V8未取得final18资格，P6 repackage不得启动**。两个CAPACITY_LIMITED样本不能被后续“更幸运”的样本覆盖。

## 两条CAPACITY_LIMITED

### Normal / 5305 / seed202

run `35955301841`，job `107492255131`。

- CAPTURE PASS
- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- ENVIRONMENT FAIL
- PERFORMANCE CAPACITY_LIMITED
- client `ss_packet` drops = 3121
- client `ss_udp` drops = 2531
- server drops = 0
- link/qdisc drops = 0

summary `10790317139` / `sha256:7233e2b2273234aeb06ff035a816855d7de35a1e142b226637810ced8d887204`

full `10790177289` / `sha256:88ec2b2ca64554e04d123558762efca7e982012e2d3eae9fbac7ab8163fa057f`

只读诊断显示 stress 尾部 client AF_PACKET 从低占用瞬时顶到约1MiB接收上限并drop。同期唯一Normal SegmentMux route：

- capacity = 4096
- peak = 4097
- full_waits = 231
- handoff_block max ≈ 784ms
- queue_age max ≈ 1.319s

transport仍为 FreshBlocked=0、FreshWindowBypass=0、FreshEmitFailures=0、RepairEvictionMaxScan=1。不是旧repair O(N)/fresh HOL复发。

### Game4 / 5305 / seed303

run `35955313644`，job `107492302493`。

- CAPTURE PASS
- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- ENVIRONMENT FAIL
- PERFORMANCE CAPACITY_LIMITED
- server `ss_packet` drops = 10460
- server `ss_udp` drops = 1042
- client drops = 0
- link/qdisc drops = 0

summary `10790312175` / `sha256:fd665eb1bd0825aae5c86a3ddd84634c1838caf78ce4c25d0cfb971a5bbaef11`

full `10790675643` / `sha256:781795bd66d39d4005a3e426dc8c6dd4acfabc335815703379749fc5592dbe7b`

只读pipeline显示：

- server ready capacity = 4096
- peak = 4097
- handoff_block max ≈ 1.895s
- handler max ≈ 2.333s
- queue_age max ≈ 2.338s

同一时段server AF_PACKET连续两秒处于约1MiB满槽并累计drop。四条Game SegmentMux route都远低于4096且full_waits=0；transport fresh三项全0、MaxScan=1。

## 通过样本的对照

同一final18 generation中的Normal/5305/seed101 run `35955293464`：

- client SegmentMux peak 465/4096，full_waits=0
- handoff max≈6.51ms，queue age max≈39.64ms
- server ready peak 784/4096
- handler max≈12.14ms

Game4/5305/seed101 run `35955286814`：

- client四route最大peak 68/4096，full_waits全0
- server ready peak 407/4096
- handler max≈7.02ms
- server raw read_gap虽曾≈6.43s，但并未造成handler/queue饱和或drop

所以当前证据更符合“有限bounded receive cushion在个别秒级消费停顿/宿主调度异常下被真实吃满”，而不是repair行为让fresh HOL。这里仍不能把本机drop解释成netem预期loss。

## 为什么不直接继续扩大容量

历史256→4096的server/client receive burst调整有明确约0.29s队列年龄与约12–15k handoff/s的容量证据。当前失败若仅按最坏秒级停顿线性扩容，会需要远大于4096的槽数，并显著增加内存与backlog latency；这与fresh、低延迟、资源有界优先级冲突。

因此本轮**不**：
- 扩kernel socket buffer；
- 盲目扩大4096 userspace receive queues；
- 修改FEC/wire/loss阈值；
- 降低analyzer门槛；
- 用新final18抽样覆盖两条CAPACITY_LIMITED。

## docs-only HEAD race

docs-only HEAD `4cfac18e1d812796533149d10c1c784d9f52e808` 的 targeted run `35955148262` PASS；foundation run `35955148248` 的Ubuntu race在attempt1与非性能job rerun attempt2都失败于同一测试：

`TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease`

均为 `lifecycle_test.go:177 timed out waiting for lifecycle state`，约3.04s。普通unit、runtimeowner race、Windows与privileged jobs仍PASS。该失败永久保留，不把docs-only HEAD描述成全绿。

## 下一步

当前资格路径停止。后续只有在新证据支持一个不牺牲fresh/低延迟/资源边界的真正产品修正时，才进入下一版本；修正后先Actions correctness/race，再跑少量独立诊断样本验证receive path与repair不退化，最后才可能建立新的完整资格矩阵。

P6当前为 NOT_RUN。
