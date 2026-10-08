# E3 Stage B：恢复到最新诊断原始FAIL，按四元组保序分片、总4096有界接收（2026-10-08）

工作范围仅 `next/performance-efficiency-20261008`，父HEAD `bb28b07c305fa11f53bcd854460e2851d19acd1b`。没有修改main、物理网卡或运行器。先完整读取中断前的[Game4诊断Actions37802779025](https://github.com/lly8666/wobuzhidao/actions/runs/37802779025) job113399041023、artifact11561433657，正式产品 `19e5b19245bed8691bbb5e7053ad27a0268c893d`，helper `bb28b07c...`。原始workflow failure、analyzer FAIL，issues为C2S UDP missing / C2S probe missing / S2C probe missing；不能只根据工具步骤success写PASS。

四条Game lane双方均真实active/physical=4。场景0% netem、300ms单向、混合真实TCP/UDP/HTTPS、每向3Mbps逻辑预算、正式Linux客户端TPROXY/加密outer raw/服务器真实TUN MTU1273、300s+3s、20:20、record自动0、profile ON、seed1826。C2S普通UDP**319缺失**：96B176/256B38/512B32/1000B10/1372B39/4068B24；S2C UDP0缺；probe C2S **1/1500**、S2C **2/1495** 未回，仅返回者p99=783.70/771.48ms。TCP双向304条完整、HTTP(S)20/20、10 HTTPS证书/body验证成功。无qdisc/socket/interface drop。上一条Stage A独立[profileOFF run37801415227](https://github.com/lly8666/wobuzhidao/actions/runs/37801415227) C2S UDP37 missing、探针0/0 missing也仍FAIL；**不能将ON和OFF当成严格同资源/同负载性能对照**。

新的直接证据是服务端raw接收2,919,960而handle2,846,374，用户态read queue精确overflow**73,586外层segments**、queue max233.66ms、等待超过10ms有1,025,258次，且服务器UDP业务下游队列overflow0。StageA异步TCP维护scheduled3106/coalesced71，主循环`group.rt.Tick`仍累计13.043wall秒(max23.82ms)。已经存在的Game共享`PacketID`池在authenticated owner之后，不能挽救在raw队列就丢掉的帧。旧profileON sync emit18.363s/ready overflow317739，StageA对队头阻塞有合理改善方向，但本诊断是**新的明确未解决瓶颈**。

## 本次源码变更是尚未验证的候选

新增`internal/runtimeentry/game_ingress_shards.go`、`game_ingress_shards_test.go`，修改`lifecycle.go`。在raw `Read()`之后、入旧main readCh之前，将**每一个FakeTCP源四元组固定地**按源端口映射到2–4个顺序worker中的一个。从首次SYN、握手、TLS/FEC解码、后续data和FIN直至退休，映射恒定，避免已发布lane切换队列时越过尚未处理的早期数据。此处仅按端口决定CPU调度，并不是鉴权或提早PacketID去重：每个worker照旧调用既有`handleSegment`，所有associations/faketcp、TLS authenticated、lane generation、server租约源检查、Game共享首次有效PacketID去重与TUN交付不变。未知连接也只在原`handleSegment`中有权进入握手/鉴权。末端read错误仍入control主循环。

**没有扩容队列**：Normal desired1仍原4096主循环；Game desired2/3/4采用256控制槽+desired × (3840/desired) FIFO，和原4096容量相等。任一slow shard只占自己的原预算，继续`offerLatestBounded`固定容量旧包丢弃，readyCurrent/readyDone和overflowDrop仍聚合计数，不隐藏局部丢包。固定2–4个worker/进程而非每包goroutine或新等待凑批；共享owner跨lane已具备并发加锁和first-arrival完整校验。路由hash碰撞会损失并行收益，但**不能**改变同流保序。terminal worker错误转到主循环致命错误通道，Run退出先取消再join workers后关闭owner/table，保护FIN、替换及generation清理。

## 尚未通过的并发、功能与效率门

此改动让服务端handshake/admission和主循环tick真正多线程运行，存在锁等待、源端口碰撞、控制握手错误、close join、worker慢lane局部队列溢出等新风险。测试覆盖Normal原4096保持、Game2/3/4总容量精确4096、同流flags/seq变化不转worker、off-port不入shard、单lane FIFO和另lane受保护。**本提交没有执行新产品Actions，不能记编译/race/PASS**。下一步需要同SHA的Linux/Windows unit、Go race、TUN/TPROXY privileged、lifecycle + FIN/replacement压力全绿，FAIL不隐藏。成功才另一个提交精确固定新产品SHA、每run一条Game4 profileOFF无损300s真实mixed单样本，之后Game2/Normal1/真正5→20→5阶段5205独立保护，并单独profileON记真实overflow。GameMode必须实现UDP和独立probe都0 missing，不能把改善95%算PASS。CPU/alloc节省需至少3份可比ProfileOFF同资源层样本，不能拿profileON差异算收益。旧E2 CPU获益尚未证实，E7 80秒S2C OPEN，E6/P6和physical NOT_RUN。

详细架构决议：[GAME_MODE_INGRESS_ARCHITECTURE_20261008.md](../GAME_MODE_INGRESS_ARCHITECTURE_20261008.md)。本轮独立[机器证据JSON](../evidence/performance-efficiency-e3-game4-profile-on-run37802779025-and-ingress-shards-20261008.json)附原始artifact SHA256。
