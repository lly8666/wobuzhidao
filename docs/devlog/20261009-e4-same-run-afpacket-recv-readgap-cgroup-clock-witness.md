# E4 AF_PACKET 100ms掉包与 raw recv / 服务端 read-gap / Cgroup 1秒时间窗关联（2026-10-09）

唯一分支 `next/performance-efficiency-20261008`、精确父 `e809218eee1ae4a8b6dc8ec7777ec815c2b88540`、产品SOURCE保持 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，不改产品、接收队列、buffer、MTU、协议、主线或物理。前一文档提交 [Foundation37868844579](https://github.com/lly8666/wobuzhidao/actions/runs/37868844579) 已独立核查全部 SUCCESS；其证明工具存在但不能关闭历史 socket-drop。

## 为什么需要新的归因防误判门

对已存正式 artifact 的结构核对发现 product ON 的 `client-diag.jsonl` 有逐秒累计 `product.raw_io.receive_calls / receive_messages`，server 另有 `product.tunnel.server_pipeline.read_gap.{over_10ms,max_ns}`；严格资源 `resources.jsonl` 在每行有成对的 `unix_ns` / `monotonic_ns` 和对应 `processes.{client,server}.cgroup.cpu.stat`。而100ms packet socket probe 只有 monotonic timestamps；**直接将 Unix 时间与单调时间比较会产生错误归因**。这些累计计数能判断包含 socket drop 事件的~1s窗口发生多少业务接收、read-gap高水位是否刷新、cgroup节流是否递增，但它们**绝不是**确切的recv syscall开始/结束时间或Go接收goroutine调度轨迹。

新 `tools/afpacket_drop_witness.py` 只读关联器用资源中同一行wall/mono计数求中位数校准(偏移漂移>20ms直接FAIL)，在唯一 business_start/end 下消化两个100ms num trace；先用已有 `afpacket_probe_report.summarize()` 严格要求缺失不为0，packet socket d增加的保守bracket扩展至上次ss开始~本次ss结束，再匹配与该范围重叠的正式逐秒diag和cgroup 1秒资源窗口，记录 raw receive calls/messages delta、server累计 >10ms read-gap 次数delta及max新纪录、cgroup nr_throttled delta。结果只称 `TEMPORAL_COINCIDENCE_ONLY` 或 `INSUFFICIENT_TIME_CONTEXT`，`causal_root=NOT_ESTABLISHED`；如果新run无drop标 `NO_REPRODUCED_SOCKET_DROP_NOT_ROOT_CLOSED`，如果原run 3150全NULL标 `NO_VALID_SOCKET_TRACE` 而非0。所有分析只读、最大512条正式/资源行、4,096 socket行与64个drop跃迁，全程不接触真实数据包和秘密地址。

合成 `tools/test_afpacket_drop_witness.py` 含7个无网络回归：两时钟偏移、双端drop归因但不宣称原因、3150 NULL拒绝、no drop 不升级、clock drift拒绝、diagnostic counter倒退fail-closed、缺少时间覆盖时返回未知。新增至 Foundation；**不**触发新300s Actions、不把 profile ON CPU算作优化收益。

旧9V45 profileOFF [run37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client skmem d33/server d86原始FAIL保持。9V74 [run37865738583](https://github.com/lly8666/wobuzhidao/actions/runs/37865738583) 原业务scoped PASS但整体workflow FAIL、100ms轨迹每端3150个NULL，不能补回。现有汇总器只能在未来真正具有同一运行中三种有效窗口的单条诊断实验上产生相关性，没有直接根因或CPU收益；不能为修 parser 就再抽幸运runner。E7约80s下行OPEN_DEFERRED、Normal/Game其余protector/E6/P6/物理不放行。

下一步先看 Foundation 真结果，必要时再界定一个有量化触发条件的接收/内核调度观测方案，而不是改大队列或盲改产品。
