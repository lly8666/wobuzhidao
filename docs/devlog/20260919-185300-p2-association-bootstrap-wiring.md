# 20260919-185300 P2 Same-Association Bootstrap Wiring

## 本轮目标

继续P2，只提取FakeTCP三次握手、IPv4/TCP packet/persona和server association最小闭包，并把已经通过Actions的BootstrapStream、Sender、StageTransition挂到同一四元组和同一TCP序列空间。明确不搬steady-state Receiver/SACK/RACK/FEC。

## 定向读取

只定向读取：
- old/internal/faketcp/packet.go + packet_test.go
- old/internal/faketcp/tcp_persona.go、tcp_persona_default_{other,windows}.go + tcp_persona_test.go
- old/internal/faketcp/server_mux.go + server_mux_test.go

旧server_mux.go的steady-state HandleSegment直接依赖Receiver、AckSelective、SACK/RACK和recovery mode；这些属于后续steady-state数据面，因此本轮只保留握手和association所有权。

## 提取边界

### packet/persona

新增/提取internal/faketcp/packet.go：
- IPv4/TCP基本build/parse。
- SYN固定MSS=1360、SACK-permitted、WS=8与WBD握手识别。
- DF、IPv4 checksum、TCP pseudo-header checksum。
- legacy和windows11可观察presentation。
- payload offset与flags/seq/ack/window。

故意不迁移SACK block编码/解析、MaxHeaderSize steady-state预算等依赖。

新增tcp_persona.go及平台default文件；persona仅影响presentation，不参与sequence/reliability。

### ServerAssociation

新增internal/faketcp/association.go：
- ServerFlow固定client/server IPv4+port四元组。
- SYN只接受已有WBD SYN option profile，不增加明文私有marker。
- SYN-ACK继续同四元组，server send sequence从serverISN+1开始。
- final ACK建立association；若final ACK携带首批TLS payload，同一次HandleSegment直接Feed BootstrapStream，避免首包丢失。
- BootstrapStream.Write经association.sendBootstrap进入同一个Sender；Sender.WaitAck由同association收到的累计ACK解除。
- EmitRetransmitDue继续使用Pending原Seq与复制payload。
- PrepareTransition冻结BootstrapStream.NextSeq作为显式ownership boundary。
- prepare后boundary及之后提前record只进StageTransition有界队列，不Feed TLS。
- pure ACK始终FakeTCP处理。
- DetachTransition关闭TLS bootstrap payload ownership并移交提前record。
- association table保持多client同public port的四元组隔离、duplicate与cap约束。

新增SegmentEmitter作为raw I/O边界，本轮不提取平台socket backend。

## 测试

packet_test.go覆盖：
- bootstrap payload build/parse和IPv4/TCP checksum。
- SYN option与WBD fingerprint。
- ordinary kernel-like SYN不误识别。
- Windows11 TTL/DF/option ordering及checksum。
- explicit legacy persona对base builder逐字节不变。

association_test.go覆盖：
- 非WBD SYN拒绝、cross-flow拒绝。
- data-bearing final ACK首批TLS字节可从BootstrapConn读取。
- BootstrapConn.Write实际发出serverISN+1序列的同flow payload，并在incoming ACK前阻塞、ACK后解除。
- prepare精确使用已读bootstrap结束Seq。
- 提前record排队，detach后移交；detach后record不再进入TLS。
- transition期间pure ACK仍只在FakeTCP路径。
- bounded association table duplicate/cap/slot reuse。

## 复用台账

REUSE_LEDGER新增packet、persona平台default与server_mux -> association条目；所有source均为单一真实old路径，避免重复此前Missing reuse source错误。

## Actions

本日志创建时新代码尚未提交/执行Actions。最近已验证SOURCE_SHA仍为525175a19165d392b825a96e95594b0507aa2878。提交后只认新精确SOURCE_SHA的repository-contract、Windows/Linux unit/build与Linux race。

## 下一项

若本轮Actions通过，下一原子任务转向old/internal/realityfront：定向提取真实TLS/uTLS persona、真实exporter与最小握手闭包，让tls.Conn真实跑在ServerAssociation.BootstrapConn上；先做本地双端真实TLS/exporter/prepare-detach测试，不先搬账户/lease全栈。
