# 已确认lane先发回程，去除兄弟建连屏障

## 本轮目标和阶段

开始HEAD9408b126，部署660不变。最近两份S19约1.64%下行损失均只在120/240s醒来后首秒增加，每次约370kB，稳态不继续扩大。源码RoutePacket用groupReadyLocked要求全部desired已确认才回程，导致已有健康lane也返回ErrTunnelNotQualified；这是一项明确可缩小的跨lane屏障。不是宣称它解释所有损失，真正没有连接时的自然恢复损失仍须区分。

## 修改与原因

datapath GameOutboundOnLanes复用既有PacketID/FEC/一次分片，只在选择lane IDs时加四位mask；排除项在任何编码前筛掉，无新队列/计时/线程/复制。linuxserver按lease demux后将资格mask传给Game，Normal必须lane1。runtimeentry在WithOutbound generation fence内选择已确认且非retiring lane；零个仍拒绝，其他缺失不阻止已确认兄弟。TunnelQualified原全部就绪资格保持。source anti-spoof、generation fence、stable lease、认证建连、4096/FEC期限和首次交付不改。

新增masked Game端到端测试：零/非法mask不消耗PacketID；单lane可交付，其他lane不编码，随后全lane同PacketID竞速与去重保持。入口测试明确partial server publication仍只发送一份，未齐全时full qualification仍false。新资格候选仅Actions待验，不部署。

## 复用来源

当前GameOutbound、SharedTUNRouter、Runtime.WithOutbound，未导入old或新恢复算法。

## Actions证据

9408 helper analysis37274592567/predelivery37274592631/targeted37274592667/GUI37274592552 PASS；foundation37274592672 Windows共享账号自动租约测试旧间歇错误复现：Dormant Wake lane1 handshake failed。2e相同测试曾失败，此处没有产品传输修改；不能宣布runner原因。追加server.Run错误上下文，不重试测试、不延长期限、不改正确预期。

产品660邻居dynamic37274802852/permanent37274806220独立诊断正在跑，profile OFF；新产品候选core/race/性能 NOT_RUN。最大UDPseed1397 SSH过期夹具无负载，记INCOMPLETE不计样本；恢复连接后seed1398正在保留精确missing IDs与target实际echo，单原生工况运行。

## 问题、排查与风险

必须验证部分可发不会让未确认lane发送、跨代资格错用或破坏正常全lane复制；正常路径只额外有界四位筛选，CPU与p99仍在每Action一条的真实负载下测，不能凭代码断言省多少。原p99和最大UDP失败保持。

## 下一项原子任务

正确性/race通过后独立Normal/Game lossless/5205与Game5305精确seed配对；门通过再打同源包native S19复验。邻居诊断若指向ARP模型，明确与真实产品隔离，不能删除旧失败。根据新增server退出上下文再修共享server错误隔离。
