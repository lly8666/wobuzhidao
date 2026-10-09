# WBD NEXT 当前开发方案

实时任务来自STATUS.json。本轮CPU与真实业务效率优化的完整实现顺序和测试细节在[PERFORMANCE_EFFICIENCY_PLAN](PERFORMANCE_EFFICIENCY_PLAN.md)，必须按E0..E7逐步执行。本文只保留长期架构及当前已决定的行为；历程和旧任务见[AGENT_CONTINUITY](AGENT_CONTINUITY.md)及只读历史快照，不重复叠加“当前任务”。

## 1. 形态和模块边界

单进程client/server，唯一TLS-like数据面。建立阶段复用真实TLS/FakeTCP association、识别、fallback、认证、protected admission及可靠bootstrap移交；一个lane从SYN到稳态同四元组与序列空间。具体协议、ticket/移交规则以WIRE_SPEC与现有核心证据为准，不另造第二握手或把稳态放回普通内核TCP。

模块：tlsrecord独立密文记录；pathmtu唯一预算；linkdata/FEC逐数据报及lane-local恢复；datapath/logicaltunnel保持租约与首次交付；runtimeowner有限外层修复；runtimeentry建立/生命周期；faketcp平台IO；Windows Wintun/Npcap、Linux共享TUN/raw、OpenWrt TPROXY保持已有边界。复用登记看MODULE_MAP/REUSE_LEDGER。

## 2. 无HOL与有限恢复

完整记录独立解密、后到首次交付，不等expected序号或累计ACK。FEC systematic立即发送，独立块和lane不相互等待。内层TCP有流内顺序，单个IP/LINK大数据报等待自己的片是允许语义，不能扩散为跨业务HOL。

4096为可放弃shadow备份，fresh不门控；ACK/SACK增量维护，repair有预算、年龄和有界选择。启动RTO1s，可信未重传RTT后SRTT+4*RTTVAR、最小200ms；3s绝对repair horizon和FEC期限保留。查不到/淘汰旧密文结束repair，同Seq相同wire，不能重新seal。不盲目改这些策略凑零loss。

## 3. MTU、封装与大包

本分支继承68cd1a4/c480cce的自动record cap与配置派生IPv4 TUN MTU。outer1300/1400、普通20+20头、FEC20:20时名义inner1173/1273；实际按统一预算、双向record limit、peer MSS和启用封装计算。没有LINK封装的合法IPv4包不额外扣20B fragment header，内层IP/TCP/UDP头不能重复扣。

TUN基于稳定配置，不能随某一lane MSS频繁改变；更小的实际peer/path约束仍可让LINK分片。IPv4最低576处也不承诺一包一record。这是配置预算，不是PMTU自动探测。

合法超inner UDP按OS DF语义分片或拒绝；产品已有逐数据报LINK兼容仍保留9000单IPv4包上限及固定wire，普通IPv4 UDP65507可由多IP片兼容，65508应API拒绝。高丢包最大UDP不要求全部恢复，但受控无损不允许无故损坏/丢失，且不能阻塞独立业务或堆无限重组状态。异常TUN超限仅丢弃计数，无主动ICMP生成能力不得伪称有反馈；TCP MSS/PMTU/ICMP黑洞另验。outer budget/checksum和同Seq不可变仍硬门。

## 4. FEC和Game

保留off、20:4/8/10/12/16/20固定集合与现有FEC v1。实际partial parity=min(k,R)，不把R/20当所有业务的实际冗余。20:20已有256/512/最大source三长度组和有效k/r计算，source立即出；首源8ms是到期条件，正式100ms tick并不保证8ms发出。E1修到期执行，不改wire/档位，不凑包等待。

Normal1、Game2..4，PacketID首有效竞速去重，FEC不跨lane。最多4权威/10物理incarnation，A→A+B→B验证后切换，candidate失败保留A，所有回调带generation。不能按复制字节虚增逻辑goodput。

## 5. 生命周期、平台与产品功能

业务idle与health/ACK/repair分开。保活缺失UNKNOWN不是空闲；server等待当前权威lane客户端FIN，Dormant保留Tunnel/lease/TUN状态。默认idle0、rotate0/0，15s保活和90s判死沿用。完全Dormant没有独立server反向唤醒通道，持续推送使用idle0。不在性能优化中盲改判死。

保留GUI portable、全部CLI/JSON参数与配置优先级、tls-startup-padding默认off、DNS双备份、LAN/CN分流、IPv6捕获丢弃、owned退出恢复、Linux服务化、多客户端认证和7天内存地址。Windows/Npcap真实驱动和ARM原生能力与hosted编译区分，文档不得继承历史包资格。

## 6. 效率和观测

已有批量IO、owned记录、增量退役、异步ACK和方向锁先审实际效果，不重复实现。允许有界工作区/pool/索引预分配换CPU，不扩大等待队列、socket排队或全flight repair库存。无每包goroutine、凑包延时、假流量、无证据密码/codec替换。CPU与吞吐收益由普通off真实进程样本验证，profile仅诊断。

每性能Action一条，所有开发构建/测试在Actions。runner型号/配额/steal/PSI/助手成本分层，多独立样本验证，不能以较好宿主解释为代码收益；容量探索按上限停止。每一步devlog+STATUS+evidence同提交，SOURCE/helper/config/seed和原始失败留存。

## 7. 当前未解决项与后续

b4原生五条留下Normal残余10包、约80秒下行中断、恢复后65507B 1.23s late；NoHOL为用户接受的b4 Actions专项PASS。MTU分支分层功能不代表集成产品真实压力PASS。80秒按用户指令延至E6优化收口后E7，证据不删除，不把它归因FEC或互联网。

当前源码完整70/strict18/1800s/P6/物理等资格按STATUS记录，未跑就是未跑。整理前详细方案及全部旧任务在history/20261008-before-efficiency-development_plan.md，仅供解释历史，不作为指令。


## 2026-10-09 新增FEC开关成本/恢复实验（PLANNED_NOT_RUN）

用户最新授权本项一个Action单job顺序跑不同业务/off与20:20；本项覆盖旧单样本限制，禁止并行负载，原正式资格规则和门槛保持。详见[FEC_POLICY_EXPERIMENT](FEC_POLICY_EXPERIMENT.md)、[夹具功能与使用](REALPATH_TEST_FIXTURE_GUIDE.md)、[接手模板](templates/FEC_POLICY_AGENT_PROMPT.md)。实际夹具当前300s/300ms/不含1%等约束必须先适配并验真，不直接声称已有serial支持。实验分支独立，产品恢复政策/默认不改，首批结果与CPU收益均NOT_RUN，现有STATUS和历史失败保留。
