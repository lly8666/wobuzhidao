# E0：缩小服务端service.Tick的UDP/TCP责任范围（2026-10-08）

上一真实Game4固定产品SOURCE `fa97f6ad916b35f0ecb157a8d97fac3ef082b1c9`，[run 37780170850](https://github.com/lly8666/wobuzhidao/actions/runs/37780170850) 原始FAIL且server ready overflow315310段、C2S UDP2105缺失、两向probe缺32/30。server tick服务阶段本身累计18.7114s/3217 tick里每group 3174次、max209.803ms、278次over10ms，runtime tick12.3734s，retransmit-table/sweep合计不足0.016s。详情与原始SHA： [上一轮fail evidence](../evidence/performance-efficiency-e0-game4-tick-phase-fail-37780170850.json)，旧 [off 37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445)和[on 37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504)也保持原始FAIL，不重标。

## 最窄的产品代码只诊断补丁
`internal/platformflow/service.go` 原`Server.Tick`逻辑完全保留，仍先`udp.Tick`再`tcp.Tick`。新增`TickTimed`只供已有`ObserveTiming` ON 使用，在**相同顺序、相同调用次数、无新增队列/等待/worker/timer**的条件下量两段wall duration。`internal/runtimeentry/lifecycle.go`在ObserveTiming=false时继续直接执行原`group.service.Tick(now)`；打开真实server诊断JSONL时才调用`TickTimed`并观测。另在`internal/runtimeentry/perf_diag.go`增加`server_pipeline.tick_service_udp`和`tick_service_tcp`两个有界原子计数器，扩展unit测试的计数值和JSON字段完整性。不会影响FEC wire、IPv4记录自动MTU、game generation/竞速、4096有限资源、TCP重传政策和first-arrival数据交付语义，且不是一次正式产品性能优化；ON的额外time.Now开销不得与OFF比较CPU。

静态代码阅读：`UDPServer.Tick`只是持锁扫描idle UDP flow并清理；`TCPServer.Tick`遍历live TCP flows，调用`tx.RetransmitDue`，并可能在维护循环同步执行`flow.tunnel.Send`，对4 lane复制有潜在阻塞。**这是候选因果，不是已确认根因**；不在知道UDP/TCP具体耗时前盲目关闭repair、缩100ms tick、扩大ready队列/内核socket或设额外goroutine。

## 自证流程与后续
本提交工作分支 `next/performance-efficiency-20261008`，父HEAD `525a27f1899be1d99b2f0e8100e4f079da17b537`；新产品SOURCE待GitHub本提交SHA产生。先等下一次`next-foundation` core/Go race/Windows与privileged TUN/TPROXY全部PASS；如FAIL先修自身，不派性能。通过后单独原子提交唯一`.github/efficiency-e0-sample.json`为Game4、mixed、3Mbps/向、300s+3s真实drain、0%netem/300ms单向、20:20、4lane、profile ON、seed1820，并把perf workflow的PRODUCT_SOURCE固定成此源码commit；本样本自身helper SHA由新配置commit冻结，仍1 Action/1样本不AB。读取server UDP/TCP两阶段、ready溢出、FEC/repair/原始业务缺失、host压力以择最小瓶颈修复，四保护后才可称E1有效。当前无优化CPU改善，不声称E1-E6或E7/physical资格，80s下行旧FAIL仍OPEN。
机器证据：[本轮诊断候选JSON](../evidence/performance-efficiency-e0-service-udp-tcp-probe-20261008.json)。
