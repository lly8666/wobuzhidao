# 20260919-211600 P2 Fallback close_notify ACK修复

## 失败证据

SOURCE_SHA：2abd15206c028e3c5eaba90fff939a5195d5b573

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35444882035

repository-contract PASS。

Windows/Linux unit都只剩同一失败：
- TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay
- 约5秒后失败
- server fallback返回 io: read/write on closed pipe

上一轮的SNI mismatch阻塞已经消失，说明associationPeerConn done唤醒修复有效。

## 进一步根因

client已经收到pong后调用clientTLS.Close。真实TCP语义下，即使应用Close，内核仍会接收并ACK对端随后到达的TLS close_notify/TCP数据。

测试FakeTCP peer不同：
- server->client FakeTCP ACK是在associationPeerConn.Read时生成；
- clientTLS.Close后测试立即peer.Close，不再Read；
- decoy tls.Server.Close产生close_notify；
- fallback target->BootstrapConn copy写出该TLS record后等待FakeTCP ACK；
- 没有reader生成ACK，因此一直等到FallbackConfig.SessionTimeout=5s；
- deadline/连接竞态最终表现为closed pipe。

这不是raw replay或fallback路由错误，而是测试transport没有模拟应用关闭后的内核ACK行为。

## 修复

fallback_test.go：
- clientTLS.Close后不立即关闭peer；
- 直接从底层peer再Read一次最终TLS record，从而按test peer现有机制生成累计ACK；
- serverDone必须在2秒内完成，明确证明不再依赖5秒SessionTimeout；
- server完成后再peer.Close。

fallback.go：
- benignFallbackCopyError增加io.ErrClosedPipe和faketcp.ErrBootstrapClosed；
- 这两类都属于splice关闭竞态的正常结束，不应把已成功完成的fallback会话判为fatal。

上一轮增加的fallbackCloseWrite保留：支持CloseWrite时半关闭，不支持时full Close。

## 不变项

- 单ClientHello分类
- recognized绝不拨decoy
- unrecognized raw ClientHello逐字节replay
- 同一FakeTCP association
- protected admission/exporter/prepare/detach
- 固定SNI/target约束

## 退出条件

新SHA必须通过repository-contract、Windows/Linux unit/build、Linux race以及既有fuzz/reference。
