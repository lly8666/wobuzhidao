# 20260919-212100 P2 Fallback TLS Shutdown顺序修复

## 失败证据

SOURCE_SHA：44b13410c4c641bd601699b69227b4822bfecceb

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35445029536

repository-contract PASS。

Linux unit：
- TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay 0.00s失败
- failed to drain/ACK fallback close_notify: n=0 err=EOF

Windows为同一逻辑失败。

这与此前固定5秒失败不同：fallback不再阻塞到SessionTimeout，上一轮产品splice退出修复已经起效。

## 根因

测试先调用clientTLS.Close，再尝试直接peer.Read最终close_notify。

uTLS Close会关闭底层net.Conn；associationPeerConn.Close立即关闭done。因此后续peer.Read必然直接EOF，不可能再模拟TCP内核ACK。

## 修复

测试改成真实TLS关闭顺序：
1. client读到pong。
2. decoy端随后主动tls.Close并发送close_notify。
3. client不先Close，而是再调用一次clientTLS.Read。
4. 该Read消费close_notify并返回EOF；底层associationPeerConn.Read同时生成FakeTCP累计ACK。
5. server fallback必须在2秒内完成。
6. 最后再clientTLS.Close/peer.Close。

不修改fallback产品路由、raw replay、dial、splice、认证或exporter代码。

## 退出条件

新SHA需完整通过repository-contract、Windows/Linux unit/build、Linux race和既有fuzz/reference。
