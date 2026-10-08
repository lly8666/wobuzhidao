# E0冻结测量源码：单条Game4 tick五阶段profile-ON诊断（2026-10-08）

## 精确SOURCE资格与防提前跑
只操作 `next/performance-efficiency-20261008`，本提交父HEAD `fa97f6ad916b35f0ecb157a8d97fac3ef082b1c9`。E0新增的server tick诊断仅`ObserveTiming`分项，并无真实产品优化、FEC/握手/4096/自动MTU逻辑变更。其精确源码 `fa97f6ad916b35f0ecb157a8d97fac3ef082b1c9`的 [next-foundation run 37779734501](https://github.com/lly8666/wobuzhidao/actions/runs/37779734501) 所有实际执行的Linux/Windows单元/编译、race、特权TUN/TPROXY与repository-contract job均success。新的性能helper由**本轮新提交精确GITHUB_SHA**冻结，绝不偷用旧bf11产品，也不继承旧性能资格。

## 唯一严格性能样本
`.github/workflows/next-efficiency-e0-single.yml`唯一产品SOURCE env由旧 `bf11fbf...`换成 `fa97f6ad916b35f0ecb157a8d97fac3ef082b1c9`；与`.github/efficiency-e0-sample.json`完全一致；只配置1个case：Game4/4 negotiated active lanes、双向逻辑3Mbps（不是4倍业务预算）、real Linux client TPROXY→加密raw→server TUN、TCP+UDP+HTTP(S) mixed、0%外层loss、300ms单向、300s有效业务+3s真实netem drain、seed1819、FEC20:20、padding off、outer1400、record limit0 auto、100ms正式default tick、profile ON，并保留一条job/一claim，没有AB/matrix。高丢业务和80s故障不借这条诊断改判。

分析产物记录五个`server_pipeline.tick_retransmit/tick_sweep/tick_runtime/tick_service/tick_bookkeeping`的计数、总wall ns、最大和超阈值次数，与原`tick/handler/queue_age/overflow_drops`、真正Game four lanes/复制/去重、FEC decode/修复、raw收发batch、CPU/RSS/PSI/host quota、丢失UDP/probe及HTTP/TCP hash合并观察。之前Game off [37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445)原始FAIL，on [37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504)原始FAIL，诊断最长tick226.85ms、累计29.94s、server ready overflow319041，不能以本条结果抹去原fail，不能将profile-ON CPU比profile-OFF或不同AMD/Intel宿主判改进。

本轮commit**尚无任何新性能结果**，Actions触发后仍要查run/job/artifact、原分析器issues、五阶段结果；如果继续FAIL保留原始FAIL。如果明确runtime E1到期维护抑或service/repair耗时主导，就挑一处单独最小修改并先走core/race，再Normal1 lossless/5205和Game4 lossless/5205四项独立Actions保护。E1–E6/P6/physical仍未完成，E7 80s下行OPEN。机器证据：[本条精确配置](../evidence/performance-efficiency-e0-game4-tick-phase-sample-candidate-20261008.json)。
