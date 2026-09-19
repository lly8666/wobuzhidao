# 20260919-190300 P2 Realityfront真实TLS与Exporter

## 本轮目标

继续P2，只提取真实TLS/uTLS persona、ClientHello route marker/classification、byte-exact replay与真实TLS exporter闭包。认证/ticket/lease和真实fallback splice继续后置。

## 关键纠偏

旧single_flow.go为了返回统一类型，把utls.ConnectionState手工复制成tls.ConnectionState。WIRE_SPEC明确禁止从这种不完整copy派生exporter，因为未导出的exporter闭包不会被复制。

本轮不复用该copy：
- client保留原始*utls.UConn；握手后直接取得utls.ConnectionState并调用ExportKeyingMaterial。
- server保留原始*tls.Conn；握手后直接取得tls.ConnectionState并调用ExportKeyingMaterial。
- exporter label固定tlsrecord.ExporterLabel，即EXPORTER-WBD-TLSLIKE-V1。
- context使用tlsrecord.ExporterContextHash(version, incarnation nonce, TunnelID, client_limit, server_limit)。
- 32-byte master立即进入tlsrecord.DeriveKeys；不记录master/keys。

## ClientHello/persona

从old/internal/realityfront/front.go提取：
- HMAC-SHA256 route marker，仍只占TLS 1.3 compatibility SessionID。
- ClientHello random/SessionID identity parser。
- byte-exact replayConn。

从old/internal/realitymirror/mirror.go提取：
- bounded ReadClientHello。
- SNI/ALPN parser。
- raw hello byte保留。

从old/internal/realityfront/single_flow.go提取：
- pinned uTLS HelloFirefox_120。
- BuildHandshakeState后只替换SessionId并重marshal。
- same provided net.Conn，不dial第二公开连接。

uTLS固定源项目版本v1.6.5；根x/crypto继续v0.38.0。

## Server TLS

识别与takeover分两步：
1. ReadHello只读取ClientHello并返回Info/Raw/Recognized；未识别时raw仍可用于后续fallback。
2. HandshakeServerRecognized只对recognized hello执行crypto/tls takeover，并replay原始hello。

server强制TLS 1.3并禁用session tickets，避免prepare之后残留post-handshake NewSessionTicket writer。后续如恢复ticket能力必须先补专项transition测试。

## FakeTCP真实集成测试

tls_test.go不只用net.Pipe做握手。它构造：
- 一个真实faketcp.ServerAssociation。
- 一个测试client net.Conn；其Write把uTLS字节切成FakeTCP payload Segment送入association.HandleSegment。
- association的SegmentEmitter把server BootstrapStream.Write发出的payload交给client Read。
- client Read收到server payload后发纯ACK回同association，真实解除Sender.WaitAck。

在此链路上执行：
- uTLS Firefox120 client。
- crypto/tls TLS1.3 server。
- ClientHello marker分类/replay。
- 双方真实exporter。
- tlsrecord KeyPair逐字节相等。
- 改TunnelID后从原uTLS state重新export，keys必须变化。
- server握手后PrepareTransition，client提前发送首个TLS-like record；DetachTransition必须拿到原Seq/原payload，证明未被TLS reader预取。

另有测试固定Firefox120 preset字段只改SessionID，并证明wrong route key时ReadHello返回Recognized=false且保留raw/SNI供未来fallback。

## 依赖

go.mod新增：
- github.com/refraction-networking/utls v1.6.5
- 其最小直接传递依赖brotil/circl/compress/x-net/x-text
- 继续使用golang.org/x/crypto v0.38.0与x/sys v0.33.0

go.sum使用uTLS v1.6.5上游/旧项目已有校验和，不盲升版本。

## Actions

本日志创建时新代码尚未提交/验证。最近产品测试SHA仍为d1a2501edf772b999cf20ded707ae36b31603740。提交后只认新精确SOURCE_SHA的repository-contract、go module consistency、Windows/Linux unit/build与Linux race。

## 下一项

若Actions通过，下一原子任务提取受保护admission参数与最小认证：record_version=1、server生成incarnation nonce、TunnelID、双向record limits必须在TLS内交换并参与exporter context，然后由该协商结果驱动prepare/detach。lease和steady-state FEC继续后置。
