# E4候选：仅把Linux即时recvmmsg接收批量8→16，发送锁批量8完全保留（2026-10-09）

仅`next/performance-efficiency-20261008`，父HEAD`8f646743f3134d280d9c7b18c1a7b3aca16323ad`。当前StageB+SACK栈scratch新产品 `ea7248974b52fcb1688679be54607e881797a743` 已通过foundation/lifecycle和一条AMD EPYC7763真Game4/5→20→5% staged5205 scoped PASS [37851931918](https://github.com/lly8666/wobuzhidao/actions/runs/37851931918)（20%阶段双向51503/51503全交付，post C2S仍6缺，probes0缺，socket0drop，383.92 CPU-s），但**不是全0缺、不是已证实CPU收益**。后续独立Game2 true5205 seed1839 [Actions37852789098](https://github.com/lly8666/wobuzhidao/actions/runs/37852789098)冻结旧产品ea724897、helper`8f646743f3134d280d9c7b18c1a7b3aca16323ad`，其结果不可归到本提交新候选。

另一个原始[Game4/9V45 5205 OFF run37851028041](https://github.com/lly8666/wobuzhidao/actions/runs/37851028041)旧产品真实FAIL：服务器AF_PACKET `ss_packet`累计drop140，即使业务UDP和probe均0缺也绝不把资源失败绿化。CPU profileON run37821931789中服务端/客户端平坦系统调用约30.53/30.32%总样本，两端真实recv约315万段/约139万calls，说明真实recv syscall成本和burst压力值得独立验证。此证据**不能证明**提高批量一定能消除socket drop；也不能用当下rmem不满去否认瞬时溢出。当前外层已用`recvmmsg(MSG_WAITFORONE)`，首次包可立即返回，不应新增等待凑满批次和无限队列。

本轮E4仅修改`internal/faketcp/raw_batch_linux.go`新增`rawReceiveBatchSize=16`，将复用rawReceiveBatch内的frames/iovec/linklayer sockaddr/mmsghdr静态数组由8扩到16，`recvmmsg`批次上限同步16，其`MSG_WAITFORONE`标志保持，因此无额外“等到16包”延迟。原`rawBatchSize=8`用于**同步发送端**`WriteSegments`的锁窗口/mmsg分段完全不变，避免延长ACK和Game先到优先级的临界区。每套Linux raw endpoint会多8条预分配暂存帧（不是增大4096 ready接收队列和repair shadow），客户端Windows Npcap原实现不变；wire/TLS/FEC20:20、ACK/RTO500ms、自动产品record MTU、虚拟TUN和物理网络均不改变。

新增Linux socketpair端到端`TestRawReceiveSixteenPrequeuedPacketsOwnedAndOrdered`，要求16条已排队IPv4/TCP段一次recvmmsg读完且逐条保序/独占最终wire，下一轮ring复用不能污染先前packet；`TestRawReceiveSixteenSparseOneDoesNotWaitToFill`保障只收到一包时立即读出；同时assert发送batch仍是8。原有raw batch/truncation/Sendto fallback/owned buffer测试仍运行，Linux Go-race和privileged TUN/TPROXY必须真实PASS；**当前提交代码/测试尚未跑Actions，不得宣布E4已通过**。即便源码资格全绿，Game2旧SHA业务结果不能继承；还要独立新E4产品Game4/true5205/profileOFF一run一case及Normal/game2低时延测试、主机同类型至少三轮资源对比，才能主张CPU或稳定性提升。当前E3跨资源层尚未合格，E7约80s下行OPEN，E6/P6/physical NOT_RUN，main未改。

[本轮精确改动与风险证据](../evidence/performance-efficiency-e4-raw-receive16-with-send8-candidate-20261009.json)。
