# 2026-09-20 P2 TLS ticket / ALPN / ownership收口

## 基线与已有证据

基线 `3f8e921a59d987172b58302eb71ba5d2418bc752`，Actions run 35457317441 PASS：repository-contract、Windows unit/build、Linux unit/build/race、tlsrecord fuzz/reference均成功；faketcp小peer-window短段与实际receive-window通告已通过。P3 LINK/FEC/MTU代码与证据不改。

## 问题证据

最新 `handshakeServerRecognizedConn` 强制 `SessionTicketsDisabled=true`，但没有形成完整票据决策；调用者的 `NextProtos` 也会直接进入recognized WBD TLS server，可能协商h2/http1.1而后续实际承载的是WBD admission。admission成功后返回结构仍暴露 `*tls.Conn/*utls.UConn`，切换后调用Close/Write会有机会继续写旧TLS record。

Go 1.23.12源码 `src/crypto/tls/handshake_server_tls13.go` 显示：非QUIC TLS1.3在server Finished后调用 `sendSessionTickets`；`shouldSendSessionTickets` 由 `SessionTicketsDisabled` 和客户端PSK-DHE能力决定；标准路径 `sendSessionTickets` 调用一次 `sendSessionTicket`。该动作发生在 `Handshake` 返回前，不需要也不允许固定sleep等ticket。

## 修改与明确策略

- recognized WBD：只允许TLS1.3、RenegotiateNever；证书链仍来自配置的Certificates/GetCertificate。
- ALPN强制为空，不宣称h2/http1.1；普通fallback仍byte-exact转发ClientHello，由真实decoy协商ALPN/证书/ServerHello。
- 开启真实crypto/tls TLS1.3 ticket生成；显式 `UnwrapSession -> nil,nil` 拒绝恢复，客户端也没有session cache。每连接继续完整TLS + admission + 新exporter/context/nonce；不新增0-RTT。
- 清除 `GetConfigForClient` 防止动态配置绕过上述recognized策略；不影响GetCertificate。
- admission成功后的返回session只保留Hello/Keys/Negotiated，不再保留tls/uTLS writer。顺序固定为握手内ticket -> admission -> prepare -> final reply ACK -> detach。
- 新测试检查TLS策略clone不污染调用者、无ALPN、恢复拒绝；用crypto/tls `WrapSession` 观察标准库确实在握手内生成恰一张ticket；admission测试断言detach后旧TLS writer不可达。

## Actions / SOURCE_SHA

本日志随实现提交创建，当前 `NOT_RUN`。资格只绑定承载本日志的新SOURCE_SHA；本地未运行编译、测试、race、fuzz或网络实验。

## 剩余风险与下一步

Firefox120只是固定客户端persona，不等于当前浏览器或目标网站完整指纹；recognized本地Go TLS ServerHello/cipher/curve/扩展/分段与目标网站仍可能不同，P2最终报告必须保留此差异。下一步是P2硬门：最小Linux raw FakeTCP网络适配 + 普通内核TCP/TLS客户端 + 受控decoy证书验证/HTTP/FIN + 连续pcap。未通过前P2保持OPEN。
