# 20260919-202700 P2 Protected Admission

## 原子目标

在已经通过Actions的真实FakeTCP -> uTLS/crypto-tls闭包上，只增加TLS内受保护admission参数与最小username/password认证。不得迁移ticket、lease、installation ID、CLI或steady-state FEC。

## 定向复用

读取：
- old/internal/realityfront/simple_auth.go
- old/internal/realityfront/simple_auth_v2.go
- 对应直接测试
- old/internal/logicaltunnel/logicaltunnel.go中的TunnelID规范

实际复用仅登记simple_auth.go -> internal/realityfront/admission.go：
- username/password只在TLS内发送一次
- 明确长度上限
- server使用constant-time compare认证

没有迁移ticket文件、一次性ticket、lease provider、installation ID和产品CLI。

既有TunnelID规范确认：
- TunnelIDBytes = 16
- archived V2在线路上使用16-byte raw TunnelID

因此新admission严格固定TunnelID为16字节，不接受任意长度身份。

## 新受保护帧

请求固定头，全部network byte order：

- magic "WBAD"
- record_version u16，当前只能是1
- client_limit u16
- TunnelID length u16，必须为16
- username length u16
- password length u16
- 16-byte TunnelID
- username
- password

成功应答：

- status=0
- accepted record_version u16
- server-generated incarnation nonce 16 bytes
- echoed client_limit u16
- server_limit u16
- TunnelID length u16，必须为16
- echoed 16-byte TunnelID

失败只返回一个受保护status byte：
- auth fail
- unsupported version
- invalid params

client拒绝version/client_limit/TunnelID echo不一致，避免双方在不同exporter context下继续。

## TLS/exporter阶段顺序

为了满足DEVELOPMENT_PLAN的切换边界：

Client：
1. 在同一FakeTCP连接上真实uTLS 1.3 handshake。
2. TLS内发送admission请求。
3. 完整读取最终admission应答。
4. 使用应答中的server nonce/server_limit和回显参数，从原始uTLS ConnectionState调用exporter。
5. 返回后才允许上层发送首条新record。

Server：
1. ReadHello并在同一association上crypto/tls replay handshake。
2. TLS内读取并认证请求。
3. server生成16-byte incarnation nonce。
4. 用协商参数从原始crypto/tls ConnectionState派生keys。
5. 在写最终TLS admission应答之前调用ServerAssociation.PrepareTransition(server_limit)。
6. 写应答；BootstrapStream.Write继续等待FakeTCP ACK。
7. ACK完成后DetachTransition，返回期间竞速到达的首批新record。

这样最终应答的到达时刻已经处于TransitionPrepared，不会让客户端读完应答后立刻发送的新record重新进入TLS reader。

任何prepare后的应答写失败会AbortTransition并清理bootstrap候选。

## API兼容

internal/realityfront/tls.go把真实TLS handshake提取成：
- handshakeClientConn
- handshakeServerRecognizedConn

原有HandshakeClient / HandshakeServerRecognized继续保留，供“exporter参数已知”的测试/调用使用。新admission路径先握手、后协商、再从同一原TLS对象导出，不使用占位nonce，也不复制ConnectionState。

## 新测试

internal/realityfront/admission_test.go固定：
- 真实FakeTCP association上的受保护admission成功闭环
- server确定性16-byte nonce
- client/server协商参数一致
- 双方exporter-derived KeyPair相同
- 最终server TLS reply到达时transition已经Prepared
- 最终detach成功且边界等于client下一Seq
- wrong password两端明确auth failure且不prepare
- record_version != 1明确拒绝
- reply TunnelID/context echo mismatch拒绝
- record limit和tlsrecord构造器最小/最大界一致

tls_test.go测试peer只增加一个server-payload观察hook，用于证明prepare-before-final-reply顺序；不改变产品行为。

## 退出条件

新精确SOURCE_SHA必须同时通过：
- repository-contract
- Windows go list/go test/go build
- Linux go list/go test/go build
- Linux race
- 既有tlsrecord directed fuzz/reference

通过前不把admission列为completed。
