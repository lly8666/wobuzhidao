# 20260919-214500 P2 Hosted Wire-Format Qualification

## 前一资格结果

SOURCE_SHA：35e5b0d3e81a4295ae283322094aeb580e848d67

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35445413191

顶层结果：completed / success。

通过：
- repository-contract
- Windows unit/build
- Linux unit/build
- Linux race
- tlsrecord directed fuzz
- independent reference vector generator + artifact

该SHA新增的三个跨层资格均通过：
1. 最后bootstrap payload整包丢失：同association、同Seq、同payload重传，收到累计ACK后原Write解除。
2. 最后bootstrap ACK丢失：peer已交付一次后同Seq重传不重复交付。
3. 首条新record整包丢失：第二条直接RouteRecord，tlsrecord decoder在缺PN=0时立即接受PN=1，无HOL。

## P2 ACCEPTANCE当前覆盖矩阵

| 要求 | 当前证据 |
|---|---|
| 真实TLS/persona | f26b8842 Actions PASS；真实uTLS Firefox120 + crypto/tls TLS1.3 |
| fallback | f9bb007f Actions PASS；单ClientHello分类、byte-exact replay、真实decoy TLS/splice |
| 认证/版本拒绝 | d4acf7ae Actions PASS；TLS内认证、record_version=1，未知版本拒绝 |
| 同lane单SYN lineage/无第二公开连接 | d1a2501 + realityfront同association集成测试 PASS |
| 最后bootstrap payload loss/retransmit | 35e5b0d3 PASS |
| 最后bootstrap ACK loss | 35e5b0d3 PASS |
| 首条新record提前到达 | StageTransition + real TLS prepare/detach集成测试 PASS |
| 首条新record丢失/no-HOL | 35e5b0d3 PASS |
| reader ownership/prefetch隔离 | server在最终admission reply前Prepare；Prepared后payload只入transition queue，TLS不再拥有该边界后的字节 |
| 退出候选/清buffer | transition conflict/overflow/overlap -> Aborted并QueueUsage=0；auth失败不Prepare |
| transition上限 | 64 records + 64*negotiated wire bytes测试 PASS |
| pure ACK ownership | association/transition测试固定ACK只留FakeTCP |
| wrap boundary | StageTransition与BootstrapStream wrap测试 PASS |
| 抓包格式门槛 | 尚缺一个把真实TLS/admission产生的Segment序列通过正式packet serializer转成IPv4/TCP字节后的hosted wire验证 |
| 真实raw/Npcap capture | 当前未实现，不能由SegmentEmitter测试冒充；fullstack capture属于ROADMAP P5，物理Npcap属于P7 |

## 本原子任务

只加测试，不改协议实现。

### 测试peer观察点

在现有associationPeerConn增加可选onClientPayload hook，与已有onServerPayload对称。默认nil，对既有测试无行为变化。

### 新 hosted wire 测试

TestP2HostedWireQualificationRealTLSAdmission：

1. 在真实FakeTCP association上运行Firefox120 uTLS client + crypto/tls server + protected admission。
2. 同时捕获client->server和server->client所有data-bearing FakeTCP Segment。
3. 每个Segment用生产MarshalSegment生成IPv4/TCP bytes。
4. 用独立测试实现验证IPv4和TCP checksum。
5. 检查IPv4 fragment field只有DF，offset=0。
6. 数据段TCP data offset必须5，即无自定义明文TCP options。
7. 每段payload <= DefaultBootstrapChunk 且 <= advertised DefaultMSS；IPv4 total length与实际一致。
8. ParseIPv4TCP后四元组/Seq/Ack/Flags/payload必须逐项一致。
9. 每方向按Seq连接payload，必须从首字节到末字节完整解析为TLS records，不能出现gap或尾部残片；至少有TLS application-data record。
10. admission magic、username、password、16-byte TunnelID不得以明文出现在双向TLS-shaped payload。
11. SYN通过正式serializer后仍只有既有MSS/SACK-permitted/window-scale标准profile。

## 证据边界

这不是raw socket/Npcap实抓，也没有capture loss统计。它证明的是：
- P2核心生成的Segment经过正式packet serializer后具备目标IPv4/TCP wire格式；
- 真实TLS bootstrap byte stream在无loss hosted adapter下连续可解析；
- 当前P2外观没有额外明文私有TCP header/options。

真实平台raw I/O、Npcap与pcap文件级证据仍明确DEFERRED，不以此测试替代；按ROADMAP在P5 fullstack capture和P7物理资格完成。

## 退出条件

新精确SOURCE_SHA必须通过repository-contract、Windows/Linux unit/build、Linux race及既有fuzz/reference。通过后再写最终P2覆盖矩阵并判定是否可进入P3。
