# E4 Game4 无损同源种子1840：业务全交付但 AF_PACKET socket drops，原判 FAIL（2026-10-09）

限制工作分支 `next/performance-efficiency-20261008`，基于父HEAD `2b51c960b9959f98e546e7fabca2e116e5ac8f45`。不触碰规范主线 `next/tlslike-dataplane`、物理设备、产品SOURCE或性能实现；不重复派发以挑选干净宿主。此提交仅核验结束的 Actions 原始artifact并原子回填本仓既有 STATUS/devlog/evidence。

## 真实样本与原判定

[Actions 37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784)，job113583590325，artifact11583983972，正式一个测量job。产品SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，helper `2b51c960b9959f98e546e7fabca2e116e5ac8f45`，seed1840，Game4配置/ordinary mixed/0% loss/每方向逻辑3Mbps/300ms单向/300s+3s/FEC20:20/padding OFF/profile OFF/auto record cap0。真实biz socket→正式OpenWrt TPROXY client→raw→router netem→正式server共享TUN→目标socket，**实际server TUN MTU1273**。原始sample步success、profile-OFF校验success；analysis原始 **FAIL, issues=[RESOURCE_AUDIT_WARNINGS, LOCAL_SOCKET_DROP]**；ledger执行success，最终run **FAIL**。绝非 PASS。

- netem c2s尝试2,850,155个外层包、s2c 2,836,372，双向人工netem drop=0，真实所有接口链路drop增量0。
- 双向每方向UDP **103062/103062**，尺寸96/256/512/1000/1372/4068B全部逐尺寸正确；源端无发送错，接收无损坏。独立probe C2S **1500/1500**、S2C **1495/1495**，缺失0、超1s/3s 0，p99 **613.791008/616.014422ms**，各向最大643.576/669.177ms；TCP各304流hash完整、连接失败0，HTTP(S)20/20 HTTPS验证10/10。业务每向goodput2.97231648Mbps。活动接收10ms统计最大空桶C2S40/S2C60ms，只是诊断不代表无HOL。
- **原始资源审计严格FAIL**：客户端AF_PACKET `ss_packet` 累计 `d33`，服务器 `d86`，各socket`rb=1048576`；采样中最大`r`客户端203776B、服务端120064B。1秒资源采样表明server d0→d86在约**98..99s**的相邻样本之间，client d0→d33在**154..155s**之间；不是packet本身精确drop时间。采样点`r0`不能排除采样间瞬时bursts/handler停顿。不允许用平均占用低断言socket未满，更不能说互联网或FEC致因。工具原判`capacity_limited_evidenced=false`。其它socket/drop0，资源样本300，进程采样无缺。
- Hosted EPYC **9V45/4vCPU**，quota未知，CPU client83.34+server83.63 = **166.97 CPU-s**，804.07 CPU-s/有效GiB，CPU PSI some max28.07、host busy max50.13%、softirq max12.86%、steal max0；RSS峰值90.17/86.36MiB。总malloc/alloc **NOT_COLLECTED**。**不能**与上一Game4真5205的EPYC9V74/265.02 CPU-s横比，既宿主不同又loss场景不同，没有CPU收益证明。
- artifact内 SHA256：manifest `05e01b64bf21239383698b72d19c01a59f769ad915ff8f16a90929494e483a61`；summary `959cbe6db8218787432c5e29ad12767ed334a566619d4c334b9c903a8226d76f`；efficiency-ledger `990cdf9b4086c7c81d744c39e2b672607db645115ab25c948dd76428a23bbdbb`。raw pcap按原工作流删除，保留数值artifact及 [有界evidence](../evidence/performance-efficiency-e4-game4-lossless-off-run37857040784.json)。

## 结论、保留失败与下一保护门

虽然业务完全交付，但额外AF_PACKET skmem drop是用户硬保护门；单样本 **FAIL**，不能因UDP缺失0改判 PASS，不能补大等待队列掩盖，不得无限抽取新host求绿。另一条Game4 staged5205 run37853468730仍仅 **单个PASS_SCOPED_ACTIONS**，本条不会抹掉它。原Game4旧source [37817466498](https://github.com/lly8666/wobuzhidao/actions/runs/37817466498) 中段S2C UDP446未达、[37851028041](https://github.com/lly8666/wobuzhidao/actions/runs/37851028041) socket drop140原FAIL、TCP-only 37762364098 hash/注入、8937/65507B jumbo待验、E7约80s下行中断OPEN_DEFERRED，E6/P6/物理NOT_RUN均不变。

**暂停新的普通/游戏性能保护调度和新候选。** 下一步需在固定SOURCE ba8ed1下针对AF_PACKET在98..99s和154..155s的突发drop进行因果定位，可选**单独**Game诊断ON的一run一场景验证lane数、固定4 ingress工作器分桶队列年龄/drop、handler时长/TCP后台维护、100ms FEC partial 到期及公平性；diagnostic ON绝不与OFF CPU比较。只有原始业务和资源保护门同时过，才进入后续Normal1、Game2 lossless/真5205与优化。已遵守当前失败就停止扩大实现改动的限制。
