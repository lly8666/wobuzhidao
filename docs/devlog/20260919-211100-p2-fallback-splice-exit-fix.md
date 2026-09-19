# 20260919-211100 P2 Fallback Splice退出修复

## 失败证据

SOURCE_SHA：b0d6f780a7bbd16e5d722be5183e826dbb145287

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35444744192

repository-contract PASS。

Windows/Linux均进入真实unit运行并出现相同两个失败：

1. TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay
   - 约5秒后失败
   - server fallback返回：io: read/write on closed pipe
   - 与FallbackConfig.SessionTimeout=5s一致

2. TestFallbackSNIMismatchDoesNotDialTarget
   - 1秒后失败
   - client handshake did not stop after fallback SNI rejection

race/fuzz/reference因unit失败未运行。

## 根因

### splice退出

旧mirror逻辑假设两侧通常是TCP，可用CloseWrite做半关闭。

新P2 fallback的一侧是FakeTCP BootstrapConn。BootstrapStream不实现CloseWrite。当前closeWrite在不支持half-close时是no-op：

- decoy TLS正常关闭；
- target -> client copy结束；
- 对BootstrapConn的closeWrite什么也不做；
- client -> target copy仍阻塞在BootstrapStream.Read；
- 直到SessionTimeout触发deadline，splice才退出，并暴露closed-pipe错误。

这是产品路径退出语义问题，不只是测试问题。

### 测试peer关闭

associationPeerConn.Close过去只设置closed bool。若Read已经阻塞在emitted channel上，它不会被唤醒，因此SNI拒绝后的client handshake goroutine不能及时退出。

## 修复

internal/realityfront/fallback.go：
- 新fallbackCloseWrite。
- 支持CloseWrite时仍只半关闭，保持真实TCP语义。
- 不支持CloseWrite时退化为Close；对BootstrapConn这是唯一可用的结束信号。
- 这样任一splice方向结束都会唤醒另一方向，不依赖SessionTimeout兜底。

internal/realityfront/tls_test.go：
- associationPeerConn增加done channel + sync.Once。
- Read同时监听emitted/done/deadline。
- Close关闭done，保证已阻塞Read立即返回EOF。
- 仅修改测试transport，不改变产品FakeTCP实现。

识别、route marker、raw ClientHello byte-exact replay、recognized admission、exporter、prepare/detach全部不改。

## 退出条件

新SHA必须通过repository-contract、Windows/Linux unit/build、Linux race及既有fuzz/reference。
