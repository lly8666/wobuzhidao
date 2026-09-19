# 20260919-185700 P2 Association Bootstrap Actions回执

## 精确证据

SOURCE_SHA：d1a2501edf772b999cf20ded707ae36b31603740

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35438696722

顶层结果：completed / success。

## 实际通过项

- repository-contract：PASS
- Windows 2022：go list / go test ./... / go build ./... PASS
- Ubuntu 24.04：go list / go test ./... / go build ./... PASS
- Ubuntu race：PASS
- 既有internal/tlsrecord directed parser fuzz：PASS
- independent P1 reference vector generator：PASS
- reference artifact upload：PASS

因此本轮新增的packet/persona/association接线测试实际在Windows/Linux unit与Linux race中执行。

## 已固定的P2行为

- WBD SYN仍用既有MSS/SACK-permitted/WS profile，不新增明文私有marker。
- legacy和Windows11 packet persona保持presentation-only；Windows profile重写后checksums正确。
- ServerAssociation固定同四元组，SYN-ACK与bootstrap payload使用同一server sequence lineage。
- data-bearing final ACK可同时完成三次握手并Feed首批TLS bootstrap字节。
- BootstrapConn.Write经同一Sender排队，并等待同association累计ACK才返回。
- prepare使用BootstrapStream.NextSeq作为显式ownership boundary。
- 提前record只进入有界StageTransition队列；pure ACK仍属于FakeTCP；detach后新record不再喂TLS。
- association table保持多client四元组隔离与容量/重复约束。

## 仍未证明

这不是P2完成。尚未接真实TLS/uTLS、真实exporter、认证/fallback、真实raw socket/Npcap，也没有steady-state Receiver/SACK/RACK/FEC或可运行client/server。

## 下一原子任务

定向读取old/internal/realityfront及其直接imports/tests，只提取真实TLS/uTLS persona、证书/识别与真实exporter最小闭包。目标是让真实TLS连接跑在ServerAssociation.BootstrapConn上，并把真实connection state产生的exporter用于tlsrecord方向keys；先做内存SegmentEmitter双端握手/prepare-detach测试，不先搬账户/lease/CLI全栈。
