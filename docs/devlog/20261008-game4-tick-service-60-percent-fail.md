# E0 Game4实流原始FAIL：tick的60.1%落在平台service维护（2026-10-08）

## 原始Actions与身份
目标branch `next/performance-efficiency-20261008`，本轮父HEAD `e6db9bc965d3a4d6492d7b0253b2db832d6ccf25`，仪表产品SOURCE `fa97f6ad916b35f0ecb157a8d97fac3ef082b1c9`，助手SOURCE `e6db9bc965d3a4d6492d7b0253b2db832d6ccf25`。先前[基础Actions 37779734501](https://github.com/lly8666/wobuzhidao/actions/runs/37779734501) core/race、Windows编译/单测、Linux TPROXY/TUN所有执行门success；[独立Game4 profile-ON run 37780170850](https://github.com/lly8666/wobuzhidao/actions/runs/37780170850)，job113320978683、artifact11552790293、仅1个300s双向逻辑3Mbps Game4 mixed、0%loss/300ms单向、3s drain、FEC20:20、pad-off、outer1400、auto record cap0、real server TUN1273、seed1819，正式Linux client TPROXY/加密raw/server共享TUN。原workflow **FAIL**；generator、analyzer、ledger步骤运行成功，但原analyzer issues三个：`LOSSLESS_UDP_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_s2c`。**不能改为PASS，也不能把运行阶段success说成总体success**；profile ON本身有计时开销，CPU不与profile OFF比较。

## 完整性和丢包
C2S逻辑发2.97615648Mbps、bulk交付2.93895563Mbps，UDP missing**2105** (96B1131/256B387/512B138/1000B63/1372B245/4068B141)，探针1500发1468回、**32missing**、returned-only p99=811.016ms，最长主动零接收窗口270ms。S2C发2.97614368Mbps、bulk2.97231648Mbps，UDP missing0，但探针1495发1465回、**30missing**，returned-only p99=811.843ms。TCP304/304两方向完整hash无坏，HTTP(S)20/20、HTTPS证书/正文10个验证成功，不覆盖UDP/probe FAIL。实际Game两端active/physical均4，client逻辑出134495、Game lane copies537980；server逻辑出134522、copies538088、交付129057、duplicates381180；不是CLI假装四lane。

## 新仪表关键实证：准确选择下一优化边界
server pipeline raw reads3068638 vs handler samples2753328，**ready队列overflow315310**外层segments，与两者差完全一致；4096槽ready峰4098、queue_age最大259.406ms，内核丢包不能说明该段无丢。handler总wall73.297s/2753328次、tick总wall31.113s/3217次，单tick最大210.945ms、379次超过10ms。真正tick阶段分解如下：

|stage|sum wall秒|单次max毫秒|over10ms次数|占tick wall|
|---|---:|---:|---:|---:|
|faketcp EmitRetransmitDue|0.013452|1.117|0|0.043%|
|faketcp Sweep|0.002389|0.055|0|0.008%|
|owner runtime.Tick 含FEC/repair|12.373388|15.976|27|39.769%|
|**platformflow service.Tick**|**18.711386**|**209.803**|**278**|**60.139%**|
|replacement/idle bookkeeping|0.004220|0.681|0|0.014%|

各阶段指标来自server-diag.jsonl最新快照，不是分线程CPU-s，且group循环count3174略少于总tick3217，不能拿sum精确对等wall，也不能仅凭上述占比给损失包作一对一因果映射。这已经让原先“重传扫描或握手Sweeper吞掉大部分tick”假设缺乏支持。AMD EPYC7763 4vCPU、hostbusy peak81.59、PSI peak38.55、steal0、quota未知，client/server产品CPU194.31/169.20s/300s；profile-on不能作CPU改善证明，也无足够依据重标CAPACITY_LIMITED。server CPU top syscall6 flat53.15s、FEC xorMul29.39s（解释其他热区，但无单项因果证明）。

源码核对 `internal/platformflow/service.go:149-155` 中service.Tick实际先`s.udp.Tick(now)`再`s.tcp.Tick(now)`，前者扫UDP idle状态，后者对live TCP flows调用`RetransmitDue`并可能同一server接收循环内同步`flow.tunnel.Send`。目前尚不知209.8ms尾部主要在哪个子调用；**下一轮只拆UDP/TCP service子阶段**，其余收发顺序/有界4096/100ms tick/主动fresh/密文和FEC都不改变。若已确认TCP同步修复占用才设计无跨业务HOL的有界处理预算，而不是先扩socket/队列或取消Game竞速。Game4旧[off 37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445)、[on 37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504)仍FAIL。单lane profile ON run37777409229是scoped PASS但不能与本run不同mode/宿主做CPU AB。

本提交仅归档[数值/原始SHA机器证据](../evidence/performance-efficiency-e0-game4-tick-phase-fail-37780170850.json)与STATUS，不修改产品或触发任何新的性能run；后续E1候选必须core/race并四条独立Normal lossless/5205和Game4 lossless/5205各一Actions，不能把某条Game4 FAIL直接隐身。E6/P6/物理NOT_RUN，80秒单向故障E7 OPEN。
