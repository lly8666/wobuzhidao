# 20260919-215500 P2 Actions PASS / P3 Handoff

## P2最后证据

SOURCE_SHA：f3df77e8b9e3170e1d9c52a501985a5b384e0fba

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35446331156

顶层：completed / success。

通过：
- repository-contract
- Windows go list / go test ./... / go build ./...
- Linux go list / go test ./... / go build ./...
- Linux race
- tlsrecord directed fuzz
- independent P1 reference vector generator
- artifact upload

新增hosted wire-format测试实际证明：
- 真实uTLS/crypto-tls/admission产生的双向FakeTCP Segment可由正式MarshalSegment转为IPv4/TCP wire；
- 独立测试checksum实现验证IPv4/TCP checksum为真；
- IPv4 fragment field只有DF，offset=0；
- data-bearing TCP header无额外options；
- 每个bootstrap payload不超过DefaultBootstrapChunk=1200和DefaultMSS=1360；
- 双向payload按Seq连续重组后，从首字节到末字节完整解析为TLS records；
- protected admission magic、username、password、16-byte TunnelID不以明文出现在TLS-shaped wire payload；
- SYN继续只有标准MSS/SACK-permitted/window-scale profile。

这不是raw socket/Npcap实抓，不能冒充P5/P7真实抓包或capture-loss证据。

## P2 ACCEPTANCE覆盖结论

ACTIONS_PASS：
- 真实TLS/persona/exporter
- 真实fallback
- TLS内认证与unsupported-version明确拒绝
- 同lane同四元组/SYN lineage，无第二公开连接
- 最后bootstrap payload loss/retransmit
- 最后bootstrap ACK loss且不重复交付
- 首条新record提前到达的transition queue
- 首条新record整包丢失后后续PN立即交付，无HOL
- reader ownership：final TLS reply前prepare；boundary之后字节只进transition
- exit candidate：conflict/overflow/overlap abort并清queue；auth fail不prepare
- transition 64条和64*wire bytes上限
- pure ACK仍属FakeTCP
- wrap-aware边界
- hosted wire-format checksum/DF/MSS/options/TLS-record连续性

明确DEFERRED：
- Linux raw socket / Windows Npcap真实平台I/O
- pcap文件和capture-loss统计
- fullstack弱网抓包

DEFERRED项并非P2核心协议失败：ROADMAP将fullstack capture放在P5，物理Npcap放在P7。后续阶段必须实现并补证据，不能把本轮hosted adapter当成替代。

## 阶段判定

P2满足ROADMAP退出条件：
“真实握手、fallback、同流、切换竞态、record no-HOL核心资格通过”。

因此STATUS milestone切换到P3；product_state仍IN_PROGRESS。

## P3第一原子任务

只提取old/internal/linkdata最小闭包：
- 单业务数据报fragment wire
- PacketID/fragment identity
- 乱序重组
- 重复分片
- 最大长度/片数约束
- 有界状态与超时退役

不在同提交引入FEC、MTU调参、平台I/O或Game/Tunnel。
